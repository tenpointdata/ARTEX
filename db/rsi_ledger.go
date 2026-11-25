package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Autumn-27/artex/rsi"
)

// Server-side records the RSI gate checks evidence against (migration 0026,
// card c-0d277f): the per-task arm ledger and suite-run pre-registration.

// RecordTaskRSIArm writes the arm a task's agents were just built with. The
// first build of a task inserts the row. A rebuild while the task is live
// that differs (another prompt body or lesson set) marks the row drifted; a
// rebuild after the task ended changes nothing, so reopening a finished task
// in the console cannot rewrite what it ran.
func (d *DB) RecordTaskRSIArm(ctx context.Context, a rsi.TaskArm) error {
	sha, err := json.Marshal(nonNilSHA(a.PromptSHA))
	if err != nil {
		return err
	}
	_, err = d.ExecContext(ctx, `
		INSERT INTO task_rsi_arm(task_id, prompt_sha, lessons_mode, lesson_ids, lessons_block_sha)
		VALUES($1,$2,$3,$4::bigint[],$5)
		ON CONFLICT (task_id) DO UPDATE SET
			drifted = true,
			drift_reason = 'agents rebuilt with a different arm while the task was live',
			updated_at = now()
		WHERE NOT task_rsi_arm.drifted
		  AND (task_rsi_arm.prompt_sha <> EXCLUDED.prompt_sha
		       OR task_rsi_arm.lesson_ids <> EXCLUDED.lesson_ids
		       OR task_rsi_arm.lessons_block_sha <> EXCLUDED.lessons_block_sha)
		  AND EXISTS (SELECT 1 FROM tasks t WHERE t.id = task_rsi_arm.task_id
		              AND t.status IN ('created','running','paused'))`,
		a.TaskID, sha, a.LessonsMode, nonNilIDs(sortedLessonIDs(a.LessonIDs)), a.LessonsBlockSHA)
	return err
}

// sortedLessonIDs returns ids sorted and de-duplicated, the ledger's canonical order.
func sortedLessonIDs(ids []int64) []int64 {
	return rsi.Edit{LessonIDs: ids}.Normalize().LessonIDs
}

func nonNilSHA(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// RSITaskArms returns the ledger rows of the given tasks that exist, are not
// deleted, and were created at or after since, keyed by task id.
func (d *DB) RSITaskArms(ctx context.Context, ids []int64, since time.Time) (map[int64]rsi.TaskArm, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT a.task_id, t.name, a.prompt_sha, a.lessons_mode, to_jsonb(a.lesson_ids), a.lessons_block_sha,
		       a.drifted, a.drift_reason
		FROM task_rsi_arm a JOIN tasks t ON t.id = a.task_id
		WHERE a.task_id = ANY($1::bigint[]) AND t.deleted_at IS NULL AND t.created_at >= $2`, nonNilIDs(ids), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]rsi.TaskArm{}
	for rows.Next() {
		var a rsi.TaskArm
		var sha, lids []byte
		if err := rows.Scan(&a.TaskID, &a.TaskName, &sha, &a.LessonsMode, &lids, &a.LessonsBlockSHA,
			&a.Drifted, &a.DriftReason); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(sha, &a.PromptSHA); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(lids, &a.LessonIDs); err != nil {
			return nil, err
		}
		out[a.TaskID] = a
	}
	return out, rows.Err()
}

const rsiSuiteRunCols = `id, proposal_id, suite_run, suite, suite_version, split, to_jsonb(targets), seed, replicates, evidence_id, created_at`

func scanRSISuiteRun(row interface{ Scan(...any) error }) (*rsi.SuiteRun, time.Time, error) {
	var r rsi.SuiteRun
	var targets []byte
	var ev sql.NullInt64
	var at time.Time
	if err := row.Scan(&r.ID, &r.ProposalID, &r.SuiteRun, &r.Suite, &r.SuiteVersion, &r.Split, &targets,
		&r.Seed, &r.Replicates, &ev, &at); err != nil {
		return nil, at, err
	}
	if err := json.Unmarshal(targets, &r.Targets); err != nil {
		return nil, at, err
	}
	if ev.Valid {
		r.EvidenceID = &ev.Int64
	}
	return &r, at, nil
}

// RegisterRSISuiteRun pre-registers a suite run for an open proposal. A
// proposal holds at most one open (unconsumed) registration, and at most
// maxRuns in all, so every registered run ends up as evidence, passing or
// not, before another can start.
func (d *DB) RegisterRSISuiteRun(ctx context.Context, reg rsi.SuiteRun, maxRuns int, actor string) (*rsi.SuiteRun, error) {
	var out *rsi.SuiteRun
	err := d.rsiTx(ctx, func(tx *sql.Tx) error {
		p, err := lockRSIProposal(ctx, tx, reg.ProposalID)
		if err != nil {
			return err
		}
		switch p.Status {
		case "proposed", "passed", "failed":
		default:
			return rsiConflict("proposal %d is %s and takes no more suite runs", p.ID, p.Status)
		}
		var total, open int
		if err := tx.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE evidence_id IS NULL)
			FROM rsi_suite_runs WHERE proposal_id=$1`, p.ID).Scan(&total, &open); err != nil {
			return err
		}
		if open > 0 {
			return rsiConflict("proposal %d already has a registered suite run with no evidence; upload it first", p.ID)
		}
		if total >= maxRuns {
			return rsiConflict("proposal %d has used its %d suite runs; propose again in a new round", p.ID, maxRuns)
		}
		var taken bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM rsi_suite_runs WHERE suite_run=$1)`, reg.SuiteRun).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return rsiConflict("suite run %q is already registered", reg.SuiteRun)
		}
		out, _, err = scanRSISuiteRun(tx.QueryRowContext(ctx, `
			INSERT INTO rsi_suite_runs(proposal_id,suite_run,suite,suite_version,split,targets,seed,replicates,created_by)
			VALUES($1,$2,$3,$4,$5,$6::text[],$7,$8,$9) RETURNING `+rsiSuiteRunCols,
			p.ID, reg.SuiteRun, reg.Suite, reg.SuiteVersion, reg.Split, reg.Targets, reg.Seed, reg.Replicates, actor))
		if err != nil {
			return err
		}
		return rsiLogTx(ctx, tx, p.ID, "register", actor, map[string]any{"suite_run_id": out.ID, "suite_run": out.SuiteRun,
			"suite": out.Suite, "suite_version": out.SuiteVersion, "targets": out.Targets, "seed": out.Seed, "replicates": out.Replicates})
	})
	return out, err
}

// OpenRSISuiteRun returns the proposal's registered run named suiteRun that
// has no evidence yet, and when it was registered (evidence tasks must be
// newer).
func (d *DB) OpenRSISuiteRun(ctx context.Context, proposalID int64, suiteRun string) (*rsi.SuiteRun, time.Time, error) {
	r, at, err := scanRSISuiteRun(d.QueryRowContext(ctx, `SELECT `+rsiSuiteRunCols+` FROM rsi_suite_runs
		WHERE proposal_id=$1 AND suite_run=$2 AND evidence_id IS NULL`, proposalID, suiteRun))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, at, rsiConflict("suite run %q is not an open registration of proposal %d; register it before running", suiteRun, proposalID)
	}
	return r, at, err
}

// RSISuiteRuns lists a proposal's registrations, oldest first.
func (d *DB) RSISuiteRuns(ctx context.Context, proposalID int64) ([]rsi.SuiteRun, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+rsiSuiteRunCols+` FROM rsi_suite_runs WHERE proposal_id=$1 ORDER BY id`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rsi.SuiteRun{}
	for rows.Next() {
		r, _, err := scanRSISuiteRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// consumeRSISuiteRunTx binds a registration to the evidence that used it.
func consumeRSISuiteRunTx(ctx context.Context, tx *sql.Tx, proposalID, suiteRunID, evidenceID int64) error {
	res, err := tx.ExecContext(ctx, `UPDATE rsi_suite_runs SET evidence_id=$3
		WHERE id=$1 AND proposal_id=$2 AND evidence_id IS NULL`, suiteRunID, proposalID, evidenceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return rsiConflict("suite run %d is not open for proposal %d", suiteRunID, proposalID)
	}
	return nil
}
