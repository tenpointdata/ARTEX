package db

import (
	"database/sql"
	"strconv"
	"time"
)

// Settings is a tiny key-value store for global app config the UI toggles at
// runtime (e.g. traffic_capture). Missing keys fall back to caller defaults.

// GetSetting returns the stored value and ok=false when the key is unset. A
// secret setting (secretSettingKeys) is decrypted.
func (d *DB) GetSetting(key string) (value string, ok bool, err error) {
	err = d.QueryRow(`SELECT value FROM settings WHERE key=$1`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if c, secret := secretSetting(key); secret {
		if value, err = openSecret(c, value); err != nil {
			return "", false, err
		}
	}
	return value, true, nil
}

// sealSetting encrypts the value of a secret setting; other values pass as is.
func sealSetting(key, value string) (string, error) {
	if c, secret := secretSetting(key); secret {
		return sealSecret(c, value)
	}
	return value, nil
}

// SetSetting upserts a setting value.
func (d *DB) SetSetting(key, value string) error {
	value, err := sealSetting(key, value)
	if err != nil {
		return err
	}
	_, err = d.Exec(`
INSERT INTO settings(key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, value)
	return err
}

// InsertSettingIfAbsent writes only when the key does not exist yet; an existing key is
// kept as-is and inserted=false is returned.
// For keys like auth.password_hash that "may only be set on first run": the judgment rests
// on the database primary-key constraint. A caller doing GetSetting-then-write is just a
// fast-fail path — on a read error or a concurrent race, the existing value will not be
// silently overwritten by ON CONFLICT DO UPDATE.
func (d *DB) InsertSettingIfAbsent(key, value string) (inserted bool, err error) {
	if value, err = sealSetting(key, value); err != nil {
		return false, err
	}
	res, err := d.Exec(`
INSERT INTO settings(key, value) VALUES ($1, $2)
ON CONFLICT (key) DO NOTHING`, key, value)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// RotatePassword stores hash and increments the monotonic version in one
// transaction. A missing version row starts at the current Unix time in
// milliseconds, not 1, so it stays ahead of tokens signed before the row was
// lost (InsertPassword does the same). A non-numeric row fails the change and
// leaves both rows unchanged. The returned version is
// the value new sessions must carry.
func (d *DB) RotatePassword(hashKey, hash, versionKey string) (int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	sealed, err := sealSetting(hashKey, hash)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`
INSERT INTO settings(key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, hashKey, sealed); err != nil {
		return 0, err
	}
	var raw string
	if err := tx.QueryRow(`
INSERT INTO settings(key, value) VALUES ($1, $2::bigint::text)
ON CONFLICT (key) DO UPDATE SET value = (settings.value::bigint + 1)::text, updated_at = now()
RETURNING value`, versionKey, time.Now().UnixMilli()).Scan(&raw); err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// InsertPassword writes the hash only when that key is absent, and moves the
// version forward in the same transaction. inserted is false when the hash
// already exists; the version row is then left as it was.
//
// The version never goes back: a first-run setup after the hash was removed
// (an operator reset) must not reissue a version an old token carries. The new
// version is the larger of the stored one plus one and the current Unix time
// in milliseconds, so it also stays ahead of tokens from before a deleted
// version row (a counter outruns the clock only with over 1000 password
// changes a second). A stored value that is not a counter counts as 0.
func (d *DB) InsertPassword(hashKey, hash, versionKey string) (version int64, inserted bool, err error) {
	tx, err := d.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	sealed, err := sealSetting(hashKey, hash)
	if err != nil {
		return 0, false, err
	}
	res, err := tx.Exec(`
INSERT INTO settings(key, value) VALUES ($1, $2)
ON CONFLICT (key) DO NOTHING`, hashKey, sealed)
	if err != nil {
		return 0, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if n == 0 {
		return 0, false, nil
	}
	var raw string
	if err := tx.QueryRow(`
INSERT INTO settings(key, value) VALUES ($1, $2::bigint::text)
ON CONFLICT (key) DO UPDATE SET value = GREATEST(
	CASE WHEN settings.value ~ '^[0-9]{1,18}$' THEN settings.value::bigint ELSE 0 END + 1,
	$2::bigint)::text, updated_at = now()
RETURNING value`, versionKey, time.Now().UnixMilli()).Scan(&raw); err != nil {
		return 0, false, err
	}
	version, err = strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return version, true, nil
}

// GetBool returns the boolean setting, or def when unset/unparseable.
func (d *DB) GetBool(key string, def bool) bool {
	v, ok, err := d.GetSetting(key)
	if err != nil || !ok {
		return def
	}
	return v == "true" || v == "1"
}

// SetBool stores a boolean setting as "true"/"false".
func (d *DB) SetBool(key string, val bool) error {
	if val {
		return d.SetSetting(key, "true")
	}
	return d.SetSetting(key, "false")
}
