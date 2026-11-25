package db

import "strconv"

// Delete/archive drain acknowledgements (E7, c-1d3e0a; table task_drain_acks).
// The control node sets a task's 'deleting' fence; every worker node answers it
// once it holds no runs of the task. Since no node starts a run of a fenced
// task, an ack stays true for as long as the fence it answers stands.

// TasksAwaitingDrainAck lists the tasks under the 'deleting' fence that nodeID
// has not acknowledged yet.
func (d *DB) TasksAwaitingDrainAck(nodeID string) ([]string, error) {
	rows, err := d.Query(`SELECT t.id FROM tasks t
WHERE t.dispatch_closed=$1
  AND NOT EXISTS (SELECT 1 FROM task_drain_acks a WHERE a.task_id=t.id AND a.node_id=$2)
ORDER BY t.id`, DispatchDeleting, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, strconv.FormatInt(id, 10))
	}
	return out, rows.Err()
}

// AckTaskDrain records that nodeID holds no runs of the task. It writes only
// while the 'deleting' fence stands, so an ack racing a fence change is
// dropped, never left to answer a later fence. Reports whether it was recorded.
func (d *DB) AckTaskDrain(taskID int64, nodeID string) (bool, error) {
	res, err := d.Exec(`INSERT INTO task_drain_acks(task_id, node_id)
SELECT id, $2 FROM tasks WHERE id=$1 AND dispatch_closed=$3
ON CONFLICT (task_id, node_id) DO UPDATE SET acked_at=now()`, taskID, nodeID, DispatchDeleting)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// UnackedLiveWorkerNodes lists the live worker-role nodes that have not
// acknowledged the task's fence: the nodes a delete or archive still waits for.
// A node whose registry row went stale (NodeTTL) is not waited for.
func (d *DB) UnackedLiveWorkerNodes(taskID int64) ([]string, error) {
	rows, err := d.Query(`SELECT n.node_id FROM worker_nodes n
WHERE n.role='worker' AND n.last_seen_at >= now() - $2 * INTERVAL '1 second'
  AND NOT EXISTS (SELECT 1 FROM task_drain_acks a WHERE a.task_id=$1 AND a.node_id=n.node_id)
ORDER BY n.node_id`, taskID, NodeTTL.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
