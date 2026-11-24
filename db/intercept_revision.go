package db

import (
	"database/sql"
	"fmt"
	"strconv"
)

// InterceptRulesRevisionKey is the settings key of the intercept rules
// revision: a counter every process that shares the database reads to learn
// that the intercept rules or the intercept_enabled_tools setting changed. It is
// not a secret setting, so it is stored as plain decimal text.
const InterceptRulesRevisionKey = "intercept_rules_revision"

// InterceptRulesRevision returns the current intercept rules revision; 0 when
// it has never been bumped. A stored value that is not a decimal integer is an
// error, never a revision: the caller must then treat its rule cache as unknown.
func (d *DB) InterceptRulesRevision() (int64, error) {
	var raw string
	err := d.QueryRow(`SELECT value FROM settings WHERE key=$1`, InterceptRulesRevisionKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is not an integer: %w", InterceptRulesRevisionKey, err)
	}
	return n, nil
}

// BumpInterceptRulesRevision increments the revision in one statement and
// returns the new value. Concurrent bumps serialize on the row: each one
// returns a distinct value. A malformed stored value makes the cast fail, so
// the bump errors instead of resetting the counter.
func (d *DB) BumpInterceptRulesRevision() (int64, error) {
	var raw string
	err := d.QueryRow(`
INSERT INTO settings(key, value) VALUES ($1, '1')
ON CONFLICT (key) DO UPDATE
SET value = (settings.value::bigint + 1)::text, updated_at = now()
RETURNING value`, InterceptRulesRevisionKey).Scan(&raw)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(raw, 10, 64)
}
