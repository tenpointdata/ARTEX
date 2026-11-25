package db

// Retired-task lookup for the worker-node file sweep (E7, docs/DISTRIBUTED.md
// "Local files on worker nodes"). Deleting or archiving a task removes its
// files only on the node that runs the delete/archive; every other node asks
// here which of the task and exploration ids it holds files for are retired.

// RetiredTasks returns the subset of taskIDs and explorationIDs whose task is
// retired: deleted, archived, or gone from the database.
//
//   - A task id is retired when no tasks row with that id is live
//     (deleted_at and archived_at both NULL).
//   - An exploration id is retired when no live task uses it, and either a
//     retired task row still names it or the exploration row is gone. An
//     exploration that exists but no task has ever named is kept: it is not a
//     task's, so the sweep has no claim on it.
//
// Ids are only ever returned on a successful read; a caller that gets an error
// must delete nothing.
func (d *DB) RetiredTasks(taskIDs, explorationIDs []int64) (tasks, explorations []int64, err error) {
	if len(taskIDs) > 0 {
		if tasks, err = d.int64Column(`
SELECT id FROM unnest($1::bigint[]) AS c(id)
WHERE NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id=c.id AND t.deleted_at IS NULL AND t.archived_at IS NULL)
ORDER BY id`, taskIDs); err != nil {
			return nil, nil, err
		}
	}
	if len(explorationIDs) > 0 {
		if explorations, err = d.int64Column(`
SELECT id FROM unnest($1::bigint[]) AS c(id)
WHERE NOT EXISTS (SELECT 1 FROM tasks t WHERE t.exploration_id=c.id AND t.deleted_at IS NULL AND t.archived_at IS NULL)
  AND (EXISTS (SELECT 1 FROM tasks t WHERE t.exploration_id=c.id)
       OR NOT EXISTS (SELECT 1 FROM explorations e WHERE e.id=c.id))
ORDER BY id`, explorationIDs); err != nil {
			return nil, nil, err
		}
	}
	return tasks, explorations, nil
}

// int64Column runs a query with one argument and collects its single bigint
// column.
func (d *DB) int64Column(query string, arg any) ([]int64, error) {
	rows, err := d.Query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
