package db

import (
	"cmp"
	"fmt"
	"slices"
)

// Work signals carry kill / pause / cancel / steer requests for a running
// intent to the node that holds its claim (E7, docs/DISTRIBUTED.md).

// Work signal actions.
const (
	WorkSignalKill   = "kill"
	WorkSignalPause  = "pause"
	WorkSignalCancel = "cancel"
	WorkSignalSteer  = "steer"
)

// WorkSignal is one undelivered control request.
type WorkSignal struct {
	ID       int64
	TaskID   int64
	IntentID int64
	Action   string
	Message  string
}

// SendWorkSignal queues a signal for the node running intentID.
func (d *DB) SendWorkSignal(taskID, intentID int64, action, message string) (int64, error) {
	switch action {
	case WorkSignalKill, WorkSignalPause, WorkSignalCancel, WorkSignalSteer:
	default:
		return 0, fmt.Errorf("unknown work signal %q", action)
	}
	var id int64
	err := d.QueryRow(`INSERT INTO work_signals(task_id, intent_id, action, message) VALUES ($1,$2,$3,$4) RETURNING id`,
		taskID, intentID, action, message).Scan(&id)
	return id, err
}

// TakeWorkSignals atomically consumes the open signals for the given intents
// (the ones this node is running) in creation order.
func (d *DB) TakeWorkSignals(intentIDs []int64, consumer string) ([]WorkSignal, error) {
	if len(intentIDs) == 0 {
		return nil, nil
	}
	rows, err := d.Query(`UPDATE work_signals SET consumed_at=now(), consumed_by=$2
WHERE id IN (SELECT id FROM work_signals WHERE consumed_at IS NULL AND intent_id = ANY($1) FOR UPDATE SKIP LOCKED)
RETURNING id, task_id, intent_id, action, message`, intentIDs, consumer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkSignal
	for rows.Next() {
		var s WorkSignal
		if err := rows.Scan(&s.ID, &s.TaskID, &s.IntentID, &s.Action, &s.Message); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b WorkSignal) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// DropWorkSignals discards the open signals of an intent that is no longer
// running anywhere (a late signal must not hit its next run).
func (d *DB) DropWorkSignals(intentID int64) error {
	_, err := d.Exec(`UPDATE work_signals SET consumed_at=now(), consumed_by='dropped' WHERE intent_id=$1 AND consumed_at IS NULL`, intentID)
	return err
}
