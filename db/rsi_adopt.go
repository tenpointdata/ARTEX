package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// RSIPromptState is an agent's active prompt at adoption time. Stored is false
// when the agent has no stored version and runs its built-in default.
type RSIPromptState struct {
	Version int
	Body    string
	Stored  bool
}

type lockedAgent struct {
	id    int64
	state RSIPromptState
}

// lockAgentPrompt locks the agent row first and reads its prompt in a second
// statement, so the prompt read sees the pointer as it is after the lock (a
// joined FOR UPDATE can re-check the agent row and keep a stale prompt row).
func lockAgentPrompt(ctx context.Context, tx *sql.Tx, key string) (*lockedAgent, error) {
	var a lockedAgent
	var cur sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT id, current_prompt_id FROM agents WHERE key=$1 FOR UPDATE`, key).Scan(&a.id, &cur)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, rsiConflict("agent %q does not exist", key)
	}
	if err != nil {
		return nil, err
	}
	if cur.Valid {
		if err := tx.QueryRowContext(ctx, `SELECT version, template_text FROM agent_prompts WHERE id=$1`, cur.Int64).
			Scan(&a.state.Version, &a.state.Body); err != nil {
			return nil, err
		}
		a.state.Stored = true
	}
	return &a, nil
}

// AdoptRSIProposal is the owner action: it applies a passed proposal whose
// newest evidence is evidenceID. For a prompt body it appends a new
// agent_prompts version and moves the pointer; checkLive receives the prompt
// that is active now and refuses when the evidence's baseline is not that
// prompt (stale evidence). For lessons it moves each one from candidate to
// adopted. The proposal row, the prompt or lesson change and the log row
// commit together.
func (d *DB) AdoptRSIProposal(ctx context.Context, id, evidenceID int64, actor string, checkLive func(RSIPromptState) error,
	checkLessons func([]Lesson) error) (*RSIProposal, error) {
	var out *RSIProposal
	err := d.rsiTx(ctx, func(tx *sql.Tx) error {
		p, err := lockRSIProposal(ctx, tx, id)
		if err != nil {
			return err
		}
		if p.Status != "passed" {
			return rsiConflict("proposal %d is %s; only a proposal whose newest evidence passed the gate can be adopted", id, p.Status)
		}
		var latest int64
		var passed bool
		if err := tx.QueryRowContext(ctx, `SELECT id, passed FROM rsi_proposal_evidence WHERE proposal_id=$1 ORDER BY id DESC LIMIT 1`,
			id).Scan(&latest, &passed); err != nil {
			return err
		}
		if latest != evidenceID || !passed {
			return rsiConflict("evidence %d is not the newest passing evidence of proposal %d (newest is %d)", evidenceID, id, latest)
		}
		detail := map[string]any{"evidence_id": evidenceID}
		switch p.Surface {
		case "prompt_body":
			a, err := lockAgentPrompt(ctx, tx, p.Target)
			if err != nil {
				return err
			}
			if err := checkLive(a.state); err != nil {
				return err
			}
			var ver int
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(version),0)+1 FROM agent_prompts WHERE agent_id=$1`, a.id).Scan(&ver); err != nil {
				return err
			}
			var pid int64
			note := fmt.Sprintf("rsi proposal #%d (evidence #%d)", id, evidenceID)
			if err := tx.QueryRowContext(ctx, `INSERT INTO agent_prompts(agent_id,version,template_text,note,updated_by)
				VALUES($1,$2,$3,$4,$5) RETURNING id`, a.id, ver, p.Body, note, actor).Scan(&pid); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE agents SET current_prompt_id=$1 WHERE id=$2`, pid, a.id); err != nil {
				return err
			}
			var before any
			if a.state.Stored {
				before = a.state.Version
			}
			if _, err := tx.ExecContext(ctx, `UPDATE rsi_proposals SET prompt_version_before=$2, prompt_version_after=$3 WHERE id=$1`,
				id, before, ver); err != nil {
				return err
			}
			detail["agent"], detail["prompt_version_before"], detail["prompt_version_after"] = p.Target, before, ver
		case "lesson":
			// Lock the lessons, then let the caller compare what they render to
			// now with what the benchmark ran.
			if _, err := tx.ExecContext(ctx, `SELECT 1 FROM rsi_lessons WHERE id = ANY($1::bigint[]) FOR UPDATE`, nonNilIDs(p.LessonIDs)); err != nil {
				return err
			}
			lrows, err := tx.QueryContext(ctx, `SELECT `+lessonSelectCols+`
				FROM rsi_lessons l WHERE l.id = ANY($1::bigint[]) AND l.status <> 'retired' ORDER BY l.id`, nonNilIDs(p.LessonIDs))
			if err != nil {
				return err
			}
			current, err := scanLessons(lrows)
			if err != nil {
				return err
			}
			if checkLessons != nil {
				if err := checkLessons(current); err != nil {
					return err
				}
			}
			res, err := tx.ExecContext(ctx, `UPDATE rsi_lessons SET status='adopted', updated_at=now()
				WHERE id = ANY($1::bigint[]) AND status='candidate'`, nonNilIDs(p.LessonIDs))
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); int(n) != len(p.LessonIDs) {
				return rsiConflict("only %d of lessons %v are candidates; each must be a candidate to graduate", n, p.LessonIDs)
			}
			detail["lesson_ids"] = p.LessonIDs
		default:
			return rsiConflict("unknown surface %q", p.Surface)
		}
		if err := setRSIStatusTx(ctx, tx, id, "adopted"); err != nil {
			return err
		}
		if err := rsiLogTx(ctx, tx, id, "adopt", actor, detail); err != nil {
			return err
		}
		out, err = lockRSIProposal(ctx, tx, id)
		return err
	})
	return out, err
}

// RollbackRSIProposal reverses an adoption. A prompt body goes back to the
// version that was active before (a pointer change; no version is deleted),
// and only while the adopted version is still the active one, so a later
// owner edit is never overwritten. Lessons still adopted go back to
// candidate; one retired since stays retired.
func (d *DB) RollbackRSIProposal(ctx context.Context, id int64, actor, reason string) (*RSIProposal, error) {
	var out *RSIProposal
	err := d.rsiTx(ctx, func(tx *sql.Tx) error {
		p, err := lockRSIProposal(ctx, tx, id)
		if err != nil {
			return err
		}
		if p.Status != "adopted" {
			return rsiConflict("proposal %d is %s; only an adopted proposal can be rolled back", id, p.Status)
		}
		detail := map[string]any{"reason": reason}
		switch p.Surface {
		case "prompt_body":
			a, err := lockAgentPrompt(ctx, tx, p.Target)
			if err != nil {
				return err
			}
			if p.PromptVersionAfter == nil || !a.state.Stored || a.state.Version != *p.PromptVersionAfter {
				return rsiConflict("%s now runs prompt version %d, not the adopted version; restore a version from the prompt history instead",
					p.Target, a.state.Version)
			}
			var target any // NULL: back to the built-in default
			if p.PromptVersionBefore != nil {
				var pid int64
				if err := tx.QueryRowContext(ctx, `SELECT id FROM agent_prompts WHERE agent_id=$1 AND version=$2`,
					a.id, *p.PromptVersionBefore).Scan(&pid); err != nil {
					return err
				}
				target = pid
			}
			if _, err := tx.ExecContext(ctx, `UPDATE agents SET current_prompt_id=$1 WHERE id=$2`, target, a.id); err != nil {
				return err
			}
			detail["agent"], detail["from_version"], detail["to_version"] = p.Target, a.state.Version, p.PromptVersionBefore
		case "lesson":
			rows, err := tx.QueryContext(ctx, `UPDATE rsi_lessons SET status='candidate', updated_at=now()
				WHERE id = ANY($1::bigint[]) AND status='adopted' RETURNING id`, nonNilIDs(p.LessonIDs))
			if err != nil {
				return err
			}
			reverted, err := collectLessonIDs(rows)
			if err != nil {
				return err
			}
			detail["lesson_ids"] = nonNilIDs(reverted)
		}
		if err := setRSIStatusTx(ctx, tx, id, "rolled_back"); err != nil {
			return err
		}
		if err := rsiLogTx(ctx, tx, id, "rollback", actor, detail); err != nil {
			return err
		}
		out, err = lockRSIProposal(ctx, tx, id)
		return err
	})
	return out, err
}
