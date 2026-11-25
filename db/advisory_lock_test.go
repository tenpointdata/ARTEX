package db

import (
	"context"
	"errors"
	"testing"
)

// PG-backed: the lock is exclusive across sessions and released on return,
// including when fn fails.
func TestWithTryAdvisoryLockExcludesOtherSessions(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	const name = "test_lock:advisory"
	inner := errors.New("unset")
	err := d.WithTryAdvisoryLock(ctx, name, func(ctx context.Context) error {
		// The pool hands fn's nested call another session, as a second
		// request or a second ARTEX instance would have.
		inner = d.WithTryAdvisoryLock(ctx, name, func(context.Context) error { return nil })
		return errors.New("fn failed")
	})
	if err == nil || err.Error() != "fn failed" {
		t.Fatalf("outer: %v", err)
	}
	if !errors.Is(inner, ErrLockBusy) {
		t.Fatalf("nested acquire while held: %v, want ErrLockBusy", inner)
	}
	ran := false
	if err := d.WithTryAdvisoryLock(ctx, name, func(context.Context) error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("not released after a failed fn: %v", err)
	}
	if err := d.WithTryAdvisoryLock(ctx, "test_lock:other", func(ctx context.Context) error {
		return d.WithTryAdvisoryLock(ctx, name, func(context.Context) error { return nil })
	}); err != nil {
		t.Fatalf("distinct names must not exclude each other: %v", err)
	}
}
