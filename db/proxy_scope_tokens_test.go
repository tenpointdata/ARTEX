package db

import (
	"strings"
	"testing"
)

func TestProxyScopeTokens(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })

	a, err := d.CreateTask("proxy scope token task a", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupExec(t, d, `DELETE FROM tasks WHERE id = $1`, a.ID) })
	b, err := d.CreateTask("proxy scope token task b", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupExec(t, d, `DELETE FROM tasks WHERE id = $1`, b.ID) })

	const tokA, tokA2, tokB = "scope-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "scope-a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2", "scope-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	for _, c := range []struct {
		task int64
		tok  string
		node string
	}{{a.ID, tokA, "w1"}, {a.ID, tokA2, "w2"}, {b.ID, tokB, "w1"}} {
		if err := d.RegisterProxyScopeToken(c.task, c.tok, c.node); err != nil {
			t.Fatalf("register %s: %v", c.tok, err)
		}
	}
	// Idempotent for the same task; refused for another.
	if err := d.RegisterProxyScopeToken(a.ID, tokA, "w1"); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if err := d.RegisterProxyScopeToken(b.ID, tokA, "w1"); err == nil {
		t.Fatal("a token registered for task a must not be re-registered for task b")
	}
	if err := d.RegisterProxyScopeToken(a.ID, "", "w1"); err == nil {
		t.Fatal("an empty token must be refused")
	}

	// Only the hash is stored.
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM proxy_scope_tokens WHERE token_sha256 LIKE 'scope-%' OR token_sha256 = $1`, tokA).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("a raw token was stored")
	}
	var stored string
	if err := d.QueryRow(`SELECT token_sha256 FROM proxy_scope_tokens WHERE task_id = $1 AND node_id = 'w1'`, a.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 64 || strings.Contains(stored, "scope-") {
		t.Fatalf("stored value %q is not a sha256 hex", stored)
	}

	lookup := func(tok string, want int64, wantOK bool) {
		t.Helper()
		got, ok, err := d.LookupProxyScopeToken(tok)
		if err != nil {
			t.Fatal(err)
		}
		if ok != wantOK || got != want {
			t.Fatalf("lookup %q = (%d, %v), want (%d, %v)", tok, got, ok, want, wantOK)
		}
	}
	lookup(tokA, a.ID, true)
	lookup(tokB, b.ID, true)
	lookup("scope-unknown", 0, false)
	lookup("", 0, false)

	if err := d.ForgetNodeProxyScopeTokens(a.ID, "w2"); err != nil {
		t.Fatal(err)
	}
	lookup(tokA2, 0, false)
	lookup(tokA, a.ID, true)

	if err := d.ForgetProxyScopeTokens(a.ID); err != nil {
		t.Fatal(err)
	}
	lookup(tokA, 0, false)
	lookup(tokB, b.ID, true)

	// Deleting the task drops its tokens (ON DELETE CASCADE).
	cleanupExec(t, d, `DELETE FROM tasks WHERE id = $1`, b.ID)
	lookup(tokB, 0, false)
}
