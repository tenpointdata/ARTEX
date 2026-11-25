package db

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"
)

// Change notifications. Triggers in schema.sql (migration 0023) NOTIFY
// ChangeChannel once per changed row of findings, intercept_pending and tasks
// (status-like columns only), at commit. A Change is a hint that something the
// console shows has moved: listeners re-read through the normal, access-checked
// queries and never forward a payload as data.

// ChangeChannel is the PostgreSQL NOTIFY channel the change triggers use.
const ChangeChannel = "artex_changes"

// Change kinds, as the triggers name them.
const (
	ChangeFinding  = "finding"  // findings
	ChangeApproval = "approval" // intercept_pending (the approval queue)
	ChangeTask     = "task"     // tasks: insert, delete, status/paused/queued/deleted_at
)

// Change operations.
const (
	ChangeInsert = "insert"
	ChangeUpdate = "update"
	ChangeDelete = "delete"
)

// Change is one row change. Task is the row's task id as text ("" when it has
// none; intercept_pending stores it as text).
type Change struct {
	Kind string `json:"k"`
	Op   string `json:"op"`
	ID   int64  `json:"id"`
	Task string `json:"task"`
}

// ParseChange decodes a NOTIFY payload written by artex_notify_change.
func ParseChange(payload string) (Change, error) {
	var c Change
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		return Change{}, fmt.Errorf("change payload: %w", err)
	}
	if c.Kind == "" || c.Op == "" {
		return Change{}, fmt.Errorf("change payload without kind or op: %q", payload)
	}
	return c, nil
}

// ListenChanges holds one pool connection, LISTENs on ChangeChannel and calls
// fn for every change until ctx ends or the connection fails; it always
// returns a non-nil error (ctx.Err() on cancellation). ready runs once the
// LISTEN is in place: changes committed before that are not delivered, so a
// caller that reconnects should tell its consumers to re-read. The connection
// is discarded, not returned to the pool, so it never carries a LISTEN into
// another query. fn runs on the listening goroutine and must not block.
func (d *DB) ListenChanges(ctx context.Context, ready func(), fn func(Change)) error {
	conn, err := d.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var loopErr error
	rawErr := conn.Raw(func(dc any) error {
		sc, ok := dc.(*stdlib.Conn)
		if !ok {
			loopErr = fmt.Errorf("listen changes: driver connection is %T, not pgx", dc)
			return driver.ErrBadConn
		}
		pc := sc.Conn()
		if _, err := pc.Exec(ctx, "LISTEN "+ChangeChannel); err != nil {
			loopErr = err
			return driver.ErrBadConn
		}
		if ready != nil {
			ready()
		}
		for {
			n, err := pc.WaitForNotification(ctx)
			if err != nil {
				loopErr = err
				return driver.ErrBadConn // discard: the conn is mid-LISTEN or broken
			}
			c, err := ParseChange(n.Payload)
			if err != nil {
				continue // not ours to fail on; the next change still counts
			}
			fn(c)
		}
	})
	if loopErr == nil {
		loopErr = rawErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if loopErr == nil || errors.Is(loopErr, driver.ErrBadConn) {
		return errors.New("listen changes: connection closed")
	}
	return loopErr
}
