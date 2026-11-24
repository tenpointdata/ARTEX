package db

import (
	"strconv"
	"time"
)

// Revoked session tokens live in the settings table, one row per token:
// key = prefix + jti, value = the token's expiry in Unix seconds. A row is
// useless once the token has expired, so every write and read prunes those.
// The settings table already holds the password version these checks sit
// beside, so no new table is needed.

// RevokeToken records jti as revoked until exp and drops expired rows under prefix.
func (d *DB) RevokeToken(prefix, jti string, exp time.Time) error {
	if _, err := d.Exec(`
INSERT INTO settings(key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		prefix+jti, strconv.FormatInt(exp.Unix(), 10)); err != nil {
		return err
	}
	return d.pruneRevoked(prefix)
}

// RevokedTokens returns the unexpired revoked jtis under prefix with their expiry.
func (d *DB) RevokedTokens(prefix string) (map[string]time.Time, error) {
	if err := d.pruneRevoked(prefix); err != nil {
		return nil, err
	}
	rows, err := d.Query(`SELECT key, value FROM settings WHERE starts_with(key, $1)`, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			n = time.Now().Add(7 * 24 * time.Hour).Unix() // unreadable expiry: keep it revoked
		}
		out[key[len(prefix):]] = time.Unix(n, 0)
	}
	return out, rows.Err()
}

func (d *DB) pruneRevoked(prefix string) error {
	_, err := d.Exec(`DELETE FROM settings WHERE starts_with(key, $1)
AND CASE WHEN value ~ '^[0-9]{1,18}$' THEN value::bigint < extract(epoch from now()) ELSE false END`, prefix)
	return err
}
