package intercept

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// Two Interceptors on one database stand for two processes (a worker and the
// control node): each has its own waiter map and its own rule cache, so
// nothing reaches the other except through the database.

// startAsk runs a classic ask on ic in the background and returns the id of
// its row, once the waiter is registered, and the channel of its outcome.
func startAsk(t *testing.T, d *db.DB, ic *Interceptor, dec Decision) (int64, <-chan bool) {
	t.Helper()
	scope := taskScope(t, d, ic, time.Minute)
	ctx := WithTaskContext(context.Background(), scope.TaskID, "work#1", nil)
	out := make(chan bool, 1)
	go func() { out <- ic.HandleAsk(ctx, 0, dec, "Bash", []byte(`{"command":"id"}`)) }()
	var id int64
	eventually(t, "the classic ask", func() bool {
		return d.QueryRow(`SELECT id FROM intercept_pending WHERE task_id=$1 AND status='pending' AND target_key IS NULL`, scope.TaskID).Scan(&id) == nil
	})
	eventually(t, "the classic ask to wait", func() bool { return waiters(ic, id) == 1 })
	return id, out
}

func askOutcome(t *testing.T, out <-chan bool) bool {
	t.Helper()
	select {
	case ok := <-out:
		return ok
	case <-time.After(20 * time.Second):
		t.Fatal("the classic ask is still waiting")
		return false
	}
}

// A decision taken by another process wakes the waiter through the row.
func TestAskDecidedByAnotherProcess(t *testing.T) {
	d := targetTestDB(t)
	worker, control := New(d), New(d)

	// Approved on the control node through its Decide: the worker's waiter map
	// never hears of it.
	id, out := startAsk(t, d, worker, Decision{Action: "ask", Message: "cross-process ask"})
	if err := control.Decide(id, true, "ARTEX", "198.51.100.11"); err != nil {
		t.Fatal(err)
	}
	if !askOutcome(t, out) {
		t.Fatal("an ask approved on another process was refused")
	}

	// Denied by a write straight to the row, with a rule timeout that has not
	// elapsed: the decision wins, and the timeout policy (allow) is not applied.
	id, out = startAsk(t, d, worker, Decision{Action: "ask", TimeoutEnabled: true, TimeoutSeconds: 600, TimeoutAction: "allow"})
	if ok, err := d.ResolveInterceptBy(id, "denied", "deny", "user rejected execution", "human", "ARTEX", ""); err != nil || !ok {
		t.Fatalf("resolve = %v, %v", ok, err)
	}
	if askOutcome(t, out) {
		t.Fatal("an ask denied on another process was allowed")
	}
	row, err := d.GetInterceptDetail(id)
	if err != nil || row == nil || row.Status != "denied" || row.DecidedBy != "human" {
		t.Fatalf("row = %+v, %v; want the human denial kept", row, err)
	}

	// A row deleted under the waiter (its conversation was deleted) can never
	// be approved: the wait ends with a refusal.
	id, out = startAsk(t, d, worker, Decision{Action: "ask"})
	if _, err := d.Exec(`DELETE FROM intercept_pending WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if askOutcome(t, out) {
		t.Fatal("an ask whose row is gone was allowed")
	}
}

// The timeout and cancellation paths keep their meaning with the poll.
func TestAskTimeoutAndCancelUnchanged(t *testing.T) {
	d := targetTestDB(t)
	ic := New(d)

	id, out := startAsk(t, d, ic, Decision{Action: "ask", TimeoutEnabled: true, TimeoutSeconds: 1, TimeoutAction: "deny"})
	if askOutcome(t, out) {
		t.Fatal("a timed-out ask with TimeoutAction deny was allowed")
	}
	if row, err := d.GetInterceptDetail(id); err != nil || row == nil || row.Status != "timeout" {
		t.Fatalf("timed-out row = %+v, %v", row, err)
	}

	scope := taskScope(t, d, ic, time.Minute)
	ctx, cancel := context.WithCancel(WithTaskContext(context.Background(), scope.TaskID, "work#1", nil))
	done := make(chan bool, 1)
	go func() { done <- ic.HandleAsk(ctx, 0, Decision{Action: "ask"}, "Bash", []byte(`{"command":"id"}`)) }()
	var cid int64
	eventually(t, "the cancelled ask", func() bool {
		return d.QueryRow(`SELECT id FROM intercept_pending WHERE task_id=$1 AND status='pending'`, scope.TaskID).Scan(&cid) == nil
	})
	eventually(t, "the cancelled ask to wait", func() bool { return waiters(ic, cid) == 1 })
	cancel()
	if askOutcome(t, done) {
		t.Fatal("a cancelled ask was allowed")
	}
	if row, err := d.GetInterceptDetail(cid); err != nil || row == nil || row.Status != "denied" || row.DecidedBy != "system" ||
		row.Audit == nil || row.Audit.DecisionReason != "work cancelled" || row.Audit.ExecutionStatus != "not_executed" {
		t.Fatalf("cancelled row = %+v, %v", row, err)
	}
}

// keepEnabledTools restores the intercept_enabled_tools setting after a test.
func keepEnabledTools(t *testing.T, d *db.DB) {
	t.Helper()
	const key = "intercept_enabled_tools"
	prev, had, err := d.GetSetting(key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = d.SetSetting(key, prev)
		} else {
			_, _ = d.Exec(`DELETE FROM settings WHERE key=$1`, key)
		}
		_, _ = d.BumpInterceptRulesRevision()
	})
}

// A rule added, tightened or removed through another process is enforced here
// on the next call; so is a change of the enabled tools.
func TestRuleChangeReachesAnotherProcess(t *testing.T) {
	d := targetTestDB(t)
	keepEnabledTools(t, d)
	worker, control := New(d), New(d)
	marker := fmt.Sprintf("cross-process-%d", time.Now().UnixNano())
	input := []byte(`{"command":"echo ` + marker + `"}`)

	if dec, ok := worker.Match("Bash", input); ok && strings.Contains(dec.RuleName, marker) {
		t.Fatalf("matched before the rule exists: %+v", dec)
	}
	before, err := d.InterceptRulesRevision()
	if err != nil {
		t.Fatal(err)
	}

	rule, err := d.CreateInterceptRule("deny "+marker, "tool_input", "string", marker, "deny", "", 1000010, true, false, 0, "deny")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteInterceptRule(rule.ID); _, _ = d.BumpInterceptRulesRevision() })
	if err := control.Invalidate(); err != nil {
		t.Fatal(err)
	}
	if after, err := d.InterceptRulesRevision(); err != nil || after <= before {
		t.Fatalf("revision after Invalidate = %d, %v; want above %d", after, err, before)
	}
	if dec, ok := worker.Match("Bash", input); !ok || dec.Action != "deny" || dec.RuleID != rule.ID {
		t.Fatalf("worker after the rule was added = %+v, %v; want a deny by rule %d", dec, ok, rule.ID)
	}

	if err := d.ToggleInterceptRule(rule.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := control.Invalidate(); err != nil {
		t.Fatal(err)
	}
	if dec, ok := worker.Match("Bash", input); ok && dec.RuleID == rule.ID {
		t.Fatalf("worker still enforces the disabled rule: %+v", dec)
	}

	const tool = "cross_process_tool"
	if worker.IsToolEnabled(tool) {
		t.Fatalf("%s is gated before it is enabled", tool)
	}
	if err := control.SetEnabledTools(append(append([]string{}, defaultEnabledTools...), tool)); err != nil {
		t.Fatal(err)
	}
	if !worker.IsToolEnabled(tool) {
		t.Fatalf("worker does not gate %s after another process enabled it", tool)
	}
}

// A revision that cannot be read fails closed even with a loaded cache: every
// tool enters the rule path and every matcher denies. The cache serves again
// once the revision is readable.
func TestRevisionReadErrorFailsClosed(t *testing.T) {
	d := targetTestDB(t)
	ic := New(d)
	input := []byte(`{"command":"ls -la"}`)
	if ic.IsToolEnabled("Read") {
		t.Fatal("Read is gated with the defaults")
	}
	if dec, ok := ic.Match("Bash", input); ok && dec.Action == "deny" && strings.Contains(dec.Message, "could not be loaded") {
		t.Fatalf("a readable store denied: %+v", dec)
	}

	read := ic.revision
	ic.mu.Lock()
	ic.revision = func() (int64, error) { return 0, errors.New("revision unreadable") }
	ic.mu.Unlock()
	check := func(when string) {
		t.Helper()
		if !ic.IsToolEnabled("Read") {
			t.Errorf("%s: IsToolEnabled(Read) = false, want true (fail closed)", when)
		}
		for name, match := range map[string]func(string, []byte) (Decision, bool){
			"Match": ic.Match, "MatchBuiltin": ic.MatchBuiltin, "MatchPersistence": ic.MatchPersistence,
		} {
			tool := "Bash"
			if name == "MatchPersistence" {
				tool = "create_mcp"
			}
			if dec, ok := match(tool, input); !ok || dec.Action != "deny" || !strings.Contains(dec.Message, "could not be loaded") {
				t.Errorf("%s: %s = %+v, %v; want the store-unreadable deny", when, name, dec, ok)
			}
		}
	}
	check("revision read error")

	// Even within the revision window a failed read is not forgotten.
	prevTTL := rulesRevisionTTL
	rulesRevisionTTL = time.Hour
	t.Cleanup(func() { rulesRevisionTTL = prevTTL })
	check("revision read error, long window")

	ic.mu.Lock()
	ic.revision = read
	ic.mu.Unlock()
	if ic.IsToolEnabled("Read") {
		t.Fatal("Read is gated after the revision is readable again")
	}

	// A malformed stored revision is an unreadable one.
	rulesRevisionTTL = 0
	var prev string
	had := d.QueryRow(`SELECT value FROM settings WHERE key=$1`, db.InterceptRulesRevisionKey).Scan(&prev) == nil
	t.Cleanup(func() {
		if had {
			_, _ = d.Exec(`UPDATE settings SET value=$2 WHERE key=$1`, db.InterceptRulesRevisionKey, prev)
		} else {
			_, _ = d.Exec(`DELETE FROM settings WHERE key=$1`, db.InterceptRulesRevisionKey)
		}
	})
	if err := d.SetSetting(db.InterceptRulesRevisionKey, "garbage"); err != nil {
		t.Fatal(err)
	}
	check("malformed stored revision")
}

// Within the window a confirmed revision is served without a store read; once
// the window passes, a change is seen.
func TestRevisionWindow(t *testing.T) {
	d := targetTestDB(t)
	prevTTL := rulesRevisionTTL
	rulesRevisionTTL = time.Hour
	t.Cleanup(func() { rulesRevisionTTL = prevTTL })
	ic := New(d)
	reads := 0
	read := ic.revision
	ic.revision = func() (int64, error) { reads++; return read() }
	for range 5 {
		ic.IsToolEnabled("Bash")
	}
	if reads != 1 {
		t.Fatalf("revision read %d times within the window, want 1", reads)
	}
	rulesRevisionTTL = 0
	ic.IsToolEnabled("Bash")
	if reads != 2 {
		t.Fatalf("revision read %d times after the window, want 2", reads)
	}
}
