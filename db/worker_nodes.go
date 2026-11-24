package db

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

// Distributed intent workers (E7, docs/DISTRIBUTED.md). Several processes share
// one database: one control node (API, planners) and any number of worker nodes
// that claim intents. The claim lease is on exploration_nodes; this file holds
// the node registry, the per-task dispatch fence, the cross-node work signals
// and the task control state a worker node reads before every claim.

// NodeTTL is how long a node row stays live without a heartbeat.
const NodeTTL = 45 * time.Second

// NodeHeartbeatInterval is how often a node refreshes its registry row.
const NodeHeartbeatInterval = 10 * time.Second

// Dispatch fence reasons (tasks.dispatch_closed). Empty = open.
const (
	DispatchSettling = "settling" // timeout or budget wrap-up: running intents settle, none start
	DispatchDeleting = "deleting" // delete in progress: running intents stop, none start
)

// WorkerNode is one row of the node registry.
type WorkerNode struct {
	NodeID     string    `json:"node_id"`
	Role       string    `json:"role"`
	Host       string    `json:"host"`
	Version    string    `json:"version"`
	Workers    int       `json:"workers"`
	StartedAt  time.Time `json:"started_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Live       bool      `json:"live"`
}

// HeartbeatNode upserts the caller's registry row. started_at is kept from the
// first insert of this process (a restart with the same id resets it).
func (d *DB) HeartbeatNode(n WorkerNode, restarted bool) error {
	_, err := d.Exec(`INSERT INTO worker_nodes(node_id, role, host, version, workers, started_at, last_seen_at)
VALUES ($1,$2,$3,$4,$5,now(),now())
ON CONFLICT (node_id) DO UPDATE SET role=EXCLUDED.role, host=EXCLUDED.host, version=EXCLUDED.version,
  workers=EXCLUDED.workers, last_seen_at=now(),
  started_at=CASE WHEN $6 THEN now() ELSE worker_nodes.started_at END`,
		n.NodeID, n.Role, n.Host, n.Version, n.Workers, restarted)
	return err
}

// RemoveNode deletes the caller's registry row on a clean shutdown.
func (d *DB) RemoveNode(nodeID string) error {
	_, err := d.Exec(`DELETE FROM worker_nodes WHERE node_id=$1`, nodeID)
	return err
}

// ListNodes returns every registry row, live ones first; Live is computed on
// the database clock against NodeTTL.
func (d *DB) ListNodes() ([]WorkerNode, error) {
	rows, err := d.Query(`SELECT node_id, role, host, version, workers, started_at, last_seen_at,
  last_seen_at >= now() - $1 * INTERVAL '1 second'
FROM worker_nodes ORDER BY 8 DESC, role, node_id`, NodeTTL.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkerNode
	for rows.Next() {
		var n WorkerNode
		if err := rows.Scan(&n.NodeID, &n.Role, &n.Host, &n.Version, &n.Workers, &n.StartedAt, &n.LastSeenAt, &n.Live); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// LiveNodes counts live nodes. Every node paces target traffic (a control
// node with no workers still paces its capture proxy), so the noise governor
// divides its limits by it and N nodes stay within one profile. Never below 1.
func (d *DB) LiveNodes() (int, error) {
	var n int
	err := d.QueryRow(`SELECT count(*) FROM worker_nodes
WHERE last_seen_at >= now() - $1 * INTERVAL '1 second'`, NodeTTL.Seconds()).Scan(&n)
	if err != nil {
		return 1, err
	}
	return max(n, 1), nil
}

// TaskControl is the shared control state of a task that a worker node checks
// before each claim and on every heartbeat of a running intent.
type TaskControl struct {
	Exists         bool
	Status         string
	Paused         bool
	Queued         bool
	TargetGate     bool
	DispatchClosed string
	DeadlineAt     int64 // unix seconds, 0 = none
	DeadlinePassed bool  // deadline_at is in the past on the database clock
	FirstRunAt     int64
	TimeoutSeconds int
}

// Claimable reports whether a new intent of the task may be claimed.
func (c TaskControl) Claimable() bool {
	return c.Exists && !c.Paused && !c.Queued && !IsTerminal(c.Status) && c.DispatchClosed == "" && !c.DeadlinePassed
}

// GetTaskControl reads a task's control state. A missing task returns
// Exists=false and no error.
func (d *DB) GetTaskControl(taskID int64) (TaskControl, error) {
	var c TaskControl
	var deadline, firstRun sql.NullTime
	err := d.QueryRow(`SELECT COALESCE(status,''), paused, queued, target_gate, dispatch_closed, deadline_at, first_run_at, timeout_seconds,
  COALESCE(deadline_at < now(), false)
FROM tasks WHERE id=$1 AND deleted_at IS NULL`, taskID).Scan(&c.Status, &c.Paused, &c.Queued, &c.TargetGate, &c.DispatchClosed, &deadline, &firstRun, &c.TimeoutSeconds, &c.DeadlinePassed)
	if err == sql.ErrNoRows {
		return TaskControl{}, nil
	}
	if err != nil {
		return TaskControl{}, err
	}
	c.Exists = true
	if deadline.Valid {
		c.DeadlineAt = deadline.Time.Unix()
	}
	if firstRun.Valid {
		c.FirstRunAt = firstRun.Time.Unix()
	}
	return c, nil
}

// SetTaskDispatchClosed sets (reason != "") or clears (reason == "") the
// task's dispatch fence.
func (d *DB) SetTaskDispatchClosed(taskID int64, reason string) error {
	if reason != "" && reason != DispatchSettling && reason != DispatchDeleting {
		return fmt.Errorf("unknown dispatch fence %q", reason)
	}
	_, err := d.Exec(`UPDATE tasks SET dispatch_closed=$2 WHERE id=$1`, taskID, reason)
	return err
}

// DispatchableTaskIDs lists the tasks a worker node should serve: not
// terminal, not queued, dispatch open. Paused tasks are listed too, so a node
// keeps their loops (idle) and resumes without a rescan.
func (d *DB) DispatchableTaskIDs() ([]string, error) {
	rows, err := d.Query(`SELECT id FROM tasks
WHERE deleted_at IS NULL AND NOT queued AND dispatch_closed='' AND COALESCE(status,'') NOT IN ('done','failed','timeout')
ORDER BY id`)
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

// LiveRemoteRunningIntents counts a task's intents in 'running' whose claim is
// held by a worker node ("<node>/work#N") and whose lease is live. The control
// node's settle drain waits on it, so remote work counts, not only this
// process's in-flight operations.
func (d *DB) LiveRemoteRunningIntents(explorationID int64) (int64, error) {
	var n int64
	err := d.QueryRow(`SELECT count(*) FROM exploration_nodes
WHERE exploration_id=$1 AND kind='intent' AND state='running' AND owner LIKE '%/work#%'
  AND last_heartbeat_at >= now() - $2 * INTERVAL '1 second'`, explorationID, IntentClaimTTL.Seconds()).Scan(&n)
	return n, err
}

// RevokeRemoteIntentClaims fences every running claim of the exploration held
// by a worker node ("<node>/work#N"): the unclaim token is cleared and the
// intent returns to open, so the remote holder's next heartbeat fails and it
// stops (AbortWorkClaimLost) and every fenced write it attempts is rejected.
func (d *DB) RevokeRemoteIntentClaims(explorationID int64) (int64, error) {
	res, err := d.Exec(`UPDATE exploration_nodes SET state='open', unclaim_token=NULL, completed_at=NULL
WHERE exploration_id=$1 AND kind='intent' AND state='running' AND owner LIKE '%/work#%'`, explorationID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// IntentState returns an intent's current state ("" when it does not exist).
func (s *ExplorationStore) IntentState(id int64) (string, error) {
	var st string
	err := s.db.QueryRow(`SELECT state FROM exploration_nodes WHERE id=$1 AND exploration_id=$2 AND kind='intent'`, id, s.expID).Scan(&st)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return st, err
}

// SettledIntent is an intent that reached a terminal state.
type SettledIntent struct {
	ID    int64
	Owner string // the claim owner that settled it
}

// SettledIntentsSince returns the exploration's intents that reached a terminal
// state after since, with the newest completion time seen.
func (s *ExplorationStore) SettledIntentsSince(since time.Time) ([]SettledIntent, time.Time, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(owner,''), completed_at FROM exploration_nodes
WHERE exploration_id=$1 AND kind='intent' AND completed_at > $2 ORDER BY completed_at`, s.expID, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	var ids []SettledIntent
	latest := since
	for rows.Next() {
		var si SettledIntent
		var at time.Time
		if err := rows.Scan(&si.ID, &si.Owner, &at); err != nil {
			return nil, since, err
		}
		ids = append(ids, si)
		if at.After(latest) {
			latest = at
		}
	}
	return ids, latest, rows.Err()
}

// LiveNodeWithRole reports whether a live node other than self has role.
// A worker node refuses to claim while a live RoleAll node exists: that
// process keeps its control state in memory only.
func (d *DB) LiveNodeWithRole(role, self string) (bool, error) {
	var ok bool
	err := d.QueryRow(`SELECT EXISTS (SELECT 1 FROM worker_nodes WHERE role=$1 AND node_id<>$2
  AND last_seen_at >= now() - $3 * INTERVAL '1 second')`, role, self, NodeTTL.Seconds()).Scan(&ok)
	return ok, err
}

// LatestActivityID is the newest activity id of the exploration (0 = none).
func (s *ExplorationStore) LatestActivityID() (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT COALESCE(max(id),0) FROM activity WHERE exploration_id=$1`, s.expID).Scan(&id)
	return id, err
}

// RunningIntentClaim returns the task and claim owner of a running intent;
// ok is false when the intent is not running.
func (d *DB) RunningIntentClaim(intentID int64) (taskID int64, owner string, ok bool, err error) {
	var o sql.NullString
	err = d.QueryRow(`SELECT t.id, n.owner FROM exploration_nodes n JOIN tasks t ON t.exploration_id = n.exploration_id
WHERE n.id=$1 AND n.kind='intent' AND n.state='running'`, intentID).Scan(&taskID, &o)
	if err == sql.ErrNoRows {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return taskID, o.String, true, nil
}
