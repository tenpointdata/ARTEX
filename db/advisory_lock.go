package db

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
)

// ErrLockBusy is returned by WithTryAdvisoryLock when another session holds the lock.
var ErrLockBusy = errors.New("advisory lock is held by another session")

// WithTryAdvisoryLock runs fn while holding the session-level advisory lock
// named name, or returns ErrLockBusy at once when another session (this
// process or another instance on the same database) holds it. The name is
// hashed to the 64-bit key space with hashtextextended, so callers pick
// readable names ("ticket_sync:7"). The lock is pinned to one checked-out
// connection, because lock and unlock through the pool could land on two
// sessions (see withSchemaMigrationLock); fn itself may use the pool.
func (d *DB) WithTryAdvisoryLock(ctx context.Context, name string, fn func(context.Context) error) (err error) {
	conn, err := d.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var got bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, name).Scan(&got); err != nil {
		return fmt.Errorf("advisory lock %s: %w", name, err)
	}
	if !got {
		return ErrLockBusy
	}
	defer func() {
		// Background: a cancelled request must still release the lock, or the
		// pooled session would keep it until the connection is recycled.
		if _, uerr := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, name); uerr != nil {
			// Drop the session rather than pool it with the lock still held.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			if err == nil {
				err = fmt.Errorf("advisory unlock %s: %w", name, uerr)
			}
		}
	}()
	return fn(ctx)
}
