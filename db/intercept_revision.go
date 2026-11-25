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

// LLMConfigRevisionKey is the settings key of the LLM configuration revision:
// bumped whenever an API change drops the LLM provider caches (an LLM profile
// saved, activated or deleted, the active config replaced, the retry policy or
// the failover switches changed, an agent's profile binding changed), so worker
// nodes drop theirs and re-apply the active profile from the database.
const LLMConfigRevisionKey = "llm_config_revision"

// InterceptRulesRevision returns the current intercept rules revision; 0 when
// it has never been bumped. A stored value that is not a decimal integer is an
// error, never a revision: the caller must then treat its rule cache as unknown.
func (d *DB) InterceptRulesRevision() (int64, error) {
	return d.SettingsRevision(InterceptRulesRevisionKey)
}

// BumpInterceptRulesRevision increments the intercept rules revision.
func (d *DB) BumpInterceptRulesRevision() (int64, error) {
	return d.BumpSettingsRevision(InterceptRulesRevisionKey)
}

// SettingsRevision returns the revision counter stored under key; 0 when it has
// never been bumped. A stored value that is not a decimal integer is an error,
// never a revision.
func (d *DB) SettingsRevision(key string) (int64, error) {
	var raw string
	err := d.QueryRow(`SELECT value FROM settings WHERE key=$1`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is not an integer: %w", key, err)
	}
	return n, nil
}

// BumpSettingsRevision increments the counter under key in one statement and
// returns the new value. Concurrent bumps serialize on the row: each one
// returns a distinct value. A malformed stored value makes the cast fail, so
// the bump errors instead of resetting the counter.
func (d *DB) BumpSettingsRevision(key string) (int64, error) {
	return bumpSettingsRevision(d.DB, key)
}

// rowQuerier is what bumpSettingsRevision needs: a *sql.DB or a *sql.Tx.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func bumpSettingsRevision(q rowQuerier, key string) (int64, error) {
	var raw string
	err := q.QueryRow(`
INSERT INTO settings(key, value) VALUES ($1, '1')
ON CONFLICT (key) DO UPDATE
SET value = (settings.value::bigint + 1)::text, updated_at = now()
RETURNING value`, key).Scan(&raw)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(raw, 10, 64)
}

// writeInterceptRules runs write and the revision bump in one transaction, so
// a rule or tool-config change is never committed without the bump that tells
// the other processes to reload it (intercept/rules_cache.go): either both
// land or neither does. The caller then only drops its own cache.
func (d *DB) writeInterceptRules(write func(tx *sql.Tx) error) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // a no-op after Commit
	if err := write(tx); err != nil {
		return err
	}
	if _, err := bumpSettingsRevision(tx, InterceptRulesRevisionKey); err != nil {
		return fmt.Errorf("bump the intercept rules revision: %w", err)
	}
	return tx.Commit()
}

// SetInterceptEnabledTools stores the intercept_enabled_tools setting (a JSON
// list) and bumps the rules revision in the same transaction.
func (d *DB) SetInterceptEnabledTools(value string) error {
	sealed, err := sealSetting(InterceptEnabledToolsKey, value)
	if err != nil {
		return err
	}
	return d.writeInterceptRules(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO settings(key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, InterceptEnabledToolsKey, sealed)
		return err
	})
}

// InterceptEnabledToolsKey is the settings key of the tools that enter the
// intercept rule system (a JSON list of names).
const InterceptEnabledToolsKey = "intercept_enabled_tools"
