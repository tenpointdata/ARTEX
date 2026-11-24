package db

import "time"

// AuthorizationSummary is the part of a task's newest artifact version that a
// task list needs: which version, and whether it ended (revoked, or its window
// is over). Its presence alone means the target gate is required
// (ErrTargetGateRequired); Ended means the task cannot resume until a new
// version is stored.
type AuthorizationSummary struct {
	Version int
	Revoked bool
	Ended   bool
}

// SummarizeAuthorization reads a summary from one version; nil stays nil.
func SummarizeAuthorization(a *EngagementAuthorization, now time.Time) *AuthorizationSummary {
	if a == nil {
		return nil
	}
	return &AuthorizationSummary{Version: a.Version, Revoked: a.Revoked, Ended: a.Ended(now)}
}

// AuthorizationSummaries returns the summary of the newest version of every
// live task that has one, keyed by task id, in one query. Ended follows
// EngagementAuthorization.Ended.
func (d *DB) AuthorizationSummaries(now time.Time) (map[int64]AuthorizationSummary, error) {
	rows, err := d.Query(`SELECT DISTINCT ON (ea.task_id) ea.task_id, ea.version, ea.revoked, ea.not_after <= $1
FROM engagement_authorizations ea JOIN tasks t ON t.id = ea.task_id
WHERE t.deleted_at IS NULL
ORDER BY ea.task_id, ea.version DESC`, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]AuthorizationSummary{}
	for rows.Next() {
		var id int64
		var s AuthorizationSummary
		var over bool
		if err := rows.Scan(&id, &s.Version, &s.Revoked, &over); err != nil {
			return nil, err
		}
		s.Ended = s.Revoked || over
		out[id] = s
	}
	return out, rows.Err()
}
