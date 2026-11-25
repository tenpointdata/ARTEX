package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Persistence for the RSI benchmark gate (RSI.md; migration 0020). The rules the
// rows encode: a proposal moves proposed → passed|failed only through evidence,
// → adopted only through AdoptRSIProposal (the owner action), and adopted →
// rolled_back only by a pointer change. Every transition writes one row to the
// append-only rsi_adoption_log in the same transaction.

// RSIProposal is one stored proposal.
type RSIProposal struct {
	ID                  int64           `json:"id"`
	Round               int             `json:"round"`
	Surface             string          `json:"surface"`
	Target              string          `json:"target,omitempty"`
	Body                string          `json:"body,omitempty"`
	BodySHA             string          `json:"body_sha,omitempty"`
	LessonIDs           []int64         `json:"lesson_ids"`
	ProposedFrom        json.RawMessage `json:"proposed_from"`
	Rationale           string          `json:"rationale"`
	Status              string          `json:"status"`
	PromptVersionBefore *int            `json:"prompt_version_before,omitempty"`
	PromptVersionAfter  *int            `json:"prompt_version_after,omitempty"`
	CreatedBy           string          `json:"created_by"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// RSIEvidence is one uploaded comparison and the server's verdict over it.
type RSIEvidence struct {
	ID         int64           `json:"id"`
	ProposalID int64           `json:"proposal_id"`
	Passed     bool            `json:"passed"`
	Verdict    json.RawMessage `json:"verdict"`
	CreatedAt  time.Time       `json:"created_at"`
}

// RSILogEntry is one row of the adoption log.
type RSILogEntry struct {
	ID         int64           `json:"id"`
	ProposalID *int64          `json:"proposal_id,omitempty"`
	Action     string          `json:"action"`
	Actor      string          `json:"actor"`
	Detail     json.RawMessage `json:"detail"`
	At         time.Time       `json:"at"`
}

var (
	// ErrRSIConflict is a refused transition (wrong status, budget, stale state).
	ErrRSIConflict = errors.New("rsi conflict")
	// ErrRSINotFound is an unknown proposal or evidence row.
	ErrRSINotFound = errors.New("rsi proposal not found")
)

func rsiConflict(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRSIConflict, fmt.Sprintf(format, args...))
}

const rsiProposalCols = `id, round, surface, target, body, body_sha, to_jsonb(lesson_ids), proposed_from, rationale, status,
	prompt_version_before, prompt_version_after, created_by, created_at, updated_at`

// rsiLockKey serializes proposal creation so the per-round budget check holds.
const rsiLockKey = 0x52534931 // "RSI1"

func scanRSIProposal(row interface{ Scan(...any) error }) (*RSIProposal, error) {
	var p RSIProposal
	var ids []byte
	var before, after sql.NullInt64
	if err := row.Scan(&p.ID, &p.Round, &p.Surface, &p.Target, &p.Body, &p.BodySHA, &ids, &p.ProposedFrom,
		&p.Rationale, &p.Status, &before, &after, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.LessonIDs = []int64{}
	if err := json.Unmarshal(ids, &p.LessonIDs); err != nil {
		return nil, err
	}
	if before.Valid {
		v := int(before.Int64)
		p.PromptVersionBefore = &v
	}
	if after.Valid {
		v := int(after.Int64)
		p.PromptVersionAfter = &v
	}
	return &p, nil
}

func rsiLogTx(ctx context.Context, tx *sql.Tx, proposalID int64, action, actor string, detail any) error {
	js, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO rsi_adoption_log(proposal_id,action,actor,detail) VALUES($1,$2,$3,$4)`,
		proposalID, action, actor, js)
	return err
}

func (d *DB) rsiTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func lockRSIProposal(ctx context.Context, tx *sql.Tx, id int64) (*RSIProposal, error) {
	p, err := scanRSIProposal(tx.QueryRowContext(ctx, `SELECT `+rsiProposalCols+` FROM rsi_proposals WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRSINotFound
	}
	return p, err
}

func setRSIStatusTx(ctx context.Context, tx *sql.Tx, id int64, status string) error {
	_, err := tx.ExecContext(ctx, `UPDATE rsi_proposals SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	return err
}

// CreateRSIProposal stores a validated proposal if its round still has budget.
// budget maps a round to its edit allowance. Rounds only move forward, one at
// a time: a round older than the newest is closed (revisiting an early, larger
// round cannot reset the annealing) and the next round is at most newest+1.
// At most maxOpen proposals may be open (proposed, passed or failed) at once,
// so stepping through rounds cannot pile up unbounded work.
func (d *DB) CreateRSIProposal(ctx context.Context, p RSIProposal, budget func(round int) int, maxOpen int) (*RSIProposal, error) {
	var out *RSIProposal
	err := d.rsiTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, rsiLockKey); err != nil {
			return err
		}
		var maxRound sql.NullInt64
		var used, open int
		if err := tx.QueryRowContext(ctx, `SELECT max(round), count(*) FILTER (WHERE round=$1),
			count(*) FILTER (WHERE status IN ('proposed','passed','failed')) FROM rsi_proposals`,
			p.Round).Scan(&maxRound, &used, &open); err != nil {
			return err
		}
		if maxRound.Valid && int64(p.Round) < maxRound.Int64 {
			return rsiConflict("round %d is closed; the newest round is %d", p.Round, maxRound.Int64)
		}
		if next := maxRound.Int64 + 1; (maxRound.Valid && int64(p.Round) > next) || (!maxRound.Valid && p.Round != 0) {
			return rsiConflict("round %d skips ahead; the next round is %d", p.Round, map[bool]int64{true: next, false: 0}[maxRound.Valid])
		}
		if open >= maxOpen {
			return rsiConflict("%d proposals are open (cap %d); adopt, reject or close one first", open, maxOpen)
		}
		if allowed := budget(p.Round); used >= allowed {
			return rsiConflict("round %d has used its edit budget (%d of %d)", p.Round, used, allowed)
		}
		if len(p.ProposedFrom) == 0 {
			p.ProposedFrom = json.RawMessage("[]")
		}
		var err error
		out, err = scanRSIProposal(tx.QueryRowContext(ctx, `
			INSERT INTO rsi_proposals(round,surface,target,body,body_sha,lesson_ids,proposed_from,rationale,created_by)
			VALUES($1,$2,$3,$4,$5,$6::bigint[],$7,$8,$9) RETURNING `+rsiProposalCols,
			p.Round, p.Surface, p.Target, p.Body, p.BodySHA, nonNilIDs(p.LessonIDs), []byte(p.ProposedFrom),
			p.Rationale, p.CreatedBy))
		if err != nil {
			return err
		}
		return rsiLogTx(ctx, tx, out.ID, "propose", p.CreatedBy,
			map[string]any{"round": p.Round, "surface": p.Surface, "target": p.Target, "body_sha": p.BodySHA,
				"lesson_ids": out.LessonIDs, "budget": budget(p.Round)})
	})
	return out, err
}

// GetRSIProposal returns one proposal, its evidence (newest first) and its log.
func (d *DB) GetRSIProposal(ctx context.Context, id int64) (*RSIProposal, []RSIEvidence, []RSILogEntry, error) {
	p, err := scanRSIProposal(d.QueryRowContext(ctx, `SELECT `+rsiProposalCols+` FROM rsi_proposals WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, ErrRSINotFound
	}
	if err != nil {
		return nil, nil, nil, err
	}
	ev := []RSIEvidence{}
	rows, err := d.QueryContext(ctx, `SELECT id, proposal_id, passed, verdict, created_at
		FROM rsi_proposal_evidence WHERE proposal_id=$1 ORDER BY id DESC`, id)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e RSIEvidence
		if err := rows.Scan(&e.ID, &e.ProposalID, &e.Passed, &e.Verdict, &e.CreatedAt); err != nil {
			return nil, nil, nil, err
		}
		ev = append(ev, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	log, err := d.RSILog(ctx, id, 500)
	return p, ev, log, err
}

// ListRSIProposals returns the newest proposals first.
func (d *DB) ListRSIProposals(ctx context.Context, limit int) ([]*RSIProposal, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.QueryContext(ctx, `SELECT `+rsiProposalCols+` FROM rsi_proposals ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*RSIProposal{}
	for rows.Next() {
		p, err := scanRSIProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// LatestRSIEvidence returns the newest evidence row of each given proposal,
// keyed by proposal id. Proposals without evidence are absent.
func (d *DB) LatestRSIEvidence(ctx context.Context, proposalIDs []int64) (map[int64]RSIEvidence, error) {
	out := map[int64]RSIEvidence{}
	if len(proposalIDs) == 0 {
		return out, nil
	}
	ids, err := json.Marshal(proposalIDs)
	if err != nil {
		return nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT DISTINCT ON (proposal_id) id, proposal_id, passed, verdict, created_at
		FROM rsi_proposal_evidence
		WHERE proposal_id IN (SELECT jsonb_array_elements_text($1::jsonb)::bigint)
		ORDER BY proposal_id, id DESC`, string(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e RSIEvidence
		if err := rows.Scan(&e.ID, &e.ProposalID, &e.Passed, &e.Verdict, &e.CreatedAt); err != nil {
			return nil, err
		}
		out[e.ProposalID] = e
	}
	return out, rows.Err()
}

// RSILog returns log rows, oldest first; proposalID 0 means every proposal.
func (d *DB) RSILog(ctx context.Context, proposalID int64, limit int) ([]RSILogEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := d.QueryContext(ctx, `SELECT id, proposal_id, action, actor, detail, at FROM rsi_adoption_log
		WHERE ($1=0 OR proposal_id=$1) ORDER BY id DESC LIMIT $2`, proposalID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RSILogEntry{}
	for rows.Next() {
		var e RSILogEntry
		var pid sql.NullInt64
		if err := rows.Scan(&e.ID, &pid, &e.Action, &e.Actor, &e.Detail, &e.At); err != nil {
			return nil, err
		}
		if pid.Valid {
			e.ProposalID = &pid.Int64
		}
		out = append([]RSILogEntry{e}, out...)
	}
	return out, rows.Err()
}

// RSIEvidenceTasksInvalid returns the task ids among ids that do not exist,
// are deleted, were created before since (evidence must be fresh runs made
// after the proposal, not tasks picked from history), or already back other
// evidence (the ledger).
func (d *DB) RSIEvidenceTasksInvalid(ctx context.Context, ids []int64, since time.Time) ([]int64, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT x FROM unnest($1::bigint[]) AS x
		WHERE NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id=x AND t.deleted_at IS NULL AND t.created_at >= $2)
		   OR EXISTS (SELECT 1 FROM rsi_evidence_tasks l WHERE l.task_id=x)
		ORDER BY x`, nonNilIDs(ids), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AddRSIEvidence stores a comparison and the server's verdict, and moves the
// proposal to passed or failed. Adopted, rejected and rolled-back proposals
// take no more evidence, and a proposal takes at most maxUploads. The run
// tasks enter the ledger, so none of them can back other evidence; passing
// evidence whose tasks are already in the ledger is refused.
func (d *DB) AddRSIEvidence(ctx context.Context, proposalID int64, passed bool, verdict, baseline, candidate []byte,
	taskIDs []int64, maxUploads int, actor string) (*RSIEvidence, error) {
	var out RSIEvidence
	err := d.rsiTx(ctx, func(tx *sql.Tx) error {
		p, err := lockRSIProposal(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		switch p.Status {
		case "proposed", "passed", "failed":
		default:
			return rsiConflict("proposal %d is %s and takes no more evidence", p.ID, p.Status)
		}
		var uploads int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM rsi_proposal_evidence WHERE proposal_id=$1`, proposalID).Scan(&uploads); err != nil {
			return err
		}
		if uploads >= maxUploads {
			return rsiConflict("proposal %d already has %d evidence uploads (cap %d); propose again in a new round", p.ID, uploads, maxUploads)
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO rsi_proposal_evidence(proposal_id,passed,verdict,baseline,candidate)
			VALUES($1,$2,$3,$4,$5) RETURNING id, proposal_id, passed, verdict, created_at`,
			proposalID, passed, verdict, baseline, candidate).
			Scan(&out.ID, &out.ProposalID, &out.Passed, &out.Verdict, &out.CreatedAt); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO rsi_evidence_tasks(task_id, evidence_id)
			SELECT x, $2 FROM unnest($1::bigint[]) AS x ON CONFLICT DO NOTHING`, nonNilIDs(taskIDs), out.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); passed && int(n) != len(taskIDs) {
			return rsiConflict("a run task already backs other evidence")
		}
		status := "failed"
		if passed {
			status = "passed"
		}
		if err := setRSIStatusTx(ctx, tx, proposalID, status); err != nil {
			return err
		}
		return rsiLogTx(ctx, tx, proposalID, "evidence", actor, map[string]any{"evidence_id": out.ID, "passed": passed})
	})
	return &out, err
}

// RejectRSIProposal closes a proposal that was not adopted.
func (d *DB) RejectRSIProposal(ctx context.Context, id int64, actor, reason string) error {
	return d.rsiTx(ctx, func(tx *sql.Tx) error {
		p, err := lockRSIProposal(ctx, tx, id)
		if err != nil {
			return err
		}
		switch p.Status {
		case "proposed", "passed", "failed":
		default:
			return rsiConflict("proposal %d is %s; reject applies only before adoption (roll back an adopted one)", id, p.Status)
		}
		if err := setRSIStatusTx(ctx, tx, id, "rejected"); err != nil {
			return err
		}
		return rsiLogTx(ctx, tx, id, "reject", actor, map[string]any{"reason": reason})
	})
}

// nonNilIDs keeps an empty id list from reaching Postgres as NULL.
func nonNilIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

// RSIEvidenceArms returns the stored baseline and candidate arm files of one
// evidence row.
func (d *DB) RSIEvidenceArms(ctx context.Context, evidenceID int64) (baseline, candidate []byte, err error) {
	err = d.QueryRowContext(ctx, `SELECT baseline, candidate FROM rsi_proposal_evidence WHERE id=$1`, evidenceID).Scan(&baseline, &candidate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrRSINotFound
	}
	return baseline, candidate, err
}
