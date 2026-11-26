package traffic

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// MaxBulkIDs caps how many exchange ids one by-id delete or export may name.
// It keeps the IN (...) list well under SQLite's bound-parameter limit and a
// single request's work bounded; the UI's selection never gets near it.
const MaxBulkIDs = 5000

// ErrTooManyIDs is returned when a by-id operation names more than MaxBulkIDs.
var ErrTooManyIDs = fmt.Errorf("too many exchange ids (max %d)", MaxBulkIDs)

// NormalizeIDs trims ids and drops blanks and duplicates, keeping first-seen
// order. Callers validate the count against MaxBulkIDs after normalizing.
func NormalizeIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// idsCondition is the SQL condition (with its bound args) matching exactly ids.
// ids must be non-empty.
func idsCondition(ids []string) (string, []any) {
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	return "id IN (" + strings.Join(ph, ",") + ")", args
}

// DeleteIDs removes exactly the exchanges whose ids are given — the per-exchange
// counterpart of DeleteHostsExact, for the list's row selection. It is one
// transaction across the index, bodies, full-text index, stream messages,
// rewrite trace and blob references; a legacy row's own on-disk directory is
// staged before the commit so a filesystem failure aborts the whole deletion,
// and shared blobs are garbage-collected afterwards. Unknown ids are ignored.
// Returns rows deleted.
func (t *Traffic) DeleteIDs(ids []string) (int64, error) {
	ids = NormalizeIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > MaxBulkIDs {
		return 0, ErrTooManyIDs
	}
	where, args := idsCondition(ids)
	t.wmu.Lock()
	defer t.wmu.Unlock()
	legacy, err := t.exchangeTrees(where, args...)
	if err != nil {
		return 0, err
	}
	tx, err := t.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed
	n, err := t.deleteWhere(tx, where, args...)
	if err != nil {
		return 0, err
	}
	stageDir, moves, err := t.stageTrees(legacy)
	if err != nil {
		return 0, errors.Join(err, restoreTrees(stageDir, moves))
	}
	if err := tx.Commit(); err != nil {
		return 0, errors.Join(fmt.Errorf("commit traffic index deletion: %w", err), restoreTrees(stageDir, moves))
	}
	t.reapStage(stageDir)
	if n > 0 {
		t.reclaim()
		if err := t.gcBlobs(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// exchangeTrees returns the on-disk directory of each matching legacy exchange
// (rows recorded since bodies moved into SQLite have an empty path). Only a
// path that stays inside the store, below a host directory, and outside the
// store's own underscore-prefixed directories is returned, so a corrupt index
// row can never stage a host's whole tree or anything outside the store.
func (t *Traffic) exchangeTrees(where string, args ...any) ([]string, error) {
	rows, err := t.db.Query(`SELECT path FROM exchanges WHERE path<>'' AND `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dirs []string
	for rows.Next() {
		var rel string
		if err := rows.Scan(&rel); err != nil {
			return nil, err
		}
		if dir, ok := t.legacyExchangeDir(rel); ok {
			dirs = append(dirs, dir)
		}
	}
	return dirs, rows.Err()
}

func (t *Traffic) legacyExchangeDir(rel string) (string, bool) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if !filepath.IsLocal(clean) {
		return "", false
	}
	parts := strings.Split(clean, string(filepath.Separator))
	if len(parts) < 2 || strings.HasPrefix(parts[0], "_") {
		return "", false
	}
	return filepath.Join(t.dir, clean), true
}
