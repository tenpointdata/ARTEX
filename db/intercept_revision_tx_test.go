package db

import "testing"

// Every rule and tool-config write bumps the rules revision in its own
// transaction (writeInterceptRules): a committed change always carries its
// bump, and a bump that fails rolls the change back instead of leaving the
// other processes on the previous rules until their cache's maximum age.
func TestInterceptRuleWritesBumpInTheirTransaction(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() }) // after restoreRevision's cleanup (LIFO)
	restoreRevision(t, d)
	var tools string
	hadTools := d.QueryRow(`SELECT value FROM settings WHERE key=$1`, InterceptEnabledToolsKey).Scan(&tools) == nil
	t.Cleanup(func() {
		if hadTools {
			_, _ = d.Exec(`UPDATE settings SET value=$2 WHERE key=$1`, InterceptEnabledToolsKey, tools)
		} else {
			_, _ = d.Exec(`DELETE FROM settings WHERE key=$1`, InterceptEnabledToolsKey)
		}
	})

	bumped := func(what string, write func() error) {
		t.Helper()
		before, err := d.InterceptRulesRevision()
		if err != nil {
			t.Fatal(err)
		}
		if err := write(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		if after, err := d.InterceptRulesRevision(); err != nil || after != before+1 {
			t.Fatalf("%s: revision %d -> %d (%v); want one bump", what, before, after, err)
		}
	}
	const name = "revision-tx-test-rule"
	var id int64
	bumped("create", func() error {
		r, err := d.CreateInterceptRule(name, "tool_input", "string", "x", "deny", "", 1, true, false, 0, "deny")
		id = r.ID
		return err
	})
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM intercept_rules WHERE name=$1`, name) })
	bumped("update", func() error {
		_, err := d.UpdateInterceptRule(id, name, "tool_input", "string", "y", "deny", "", 1, true, false, 0, "deny")
		return err
	})
	bumped("toggle", func() error { return d.ToggleInterceptRule(id, false) })
	bumped("enabled tools", func() error { return d.SetInterceptEnabledTools(`["Bash"]`) })

	// A bump that fails (a malformed counter makes the cast fail) rolls the
	// write back.
	if _, err := d.Exec(`UPDATE settings SET value='not-a-number' WHERE key=$1`, InterceptRulesRevisionKey); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateInterceptRule(name+"-2", "tool_input", "string", "x", "deny", "", 1, true, false, 0, "deny"); err == nil {
		t.Fatal("create with a failing bump: want an error")
	}
	if err := d.ToggleInterceptRule(id, true); err == nil {
		t.Fatal("toggle with a failing bump: want an error")
	}
	if err := d.DeleteInterceptRule(id); err == nil {
		t.Fatal("delete with a failing bump: want an error")
	}
	if err := d.SetInterceptEnabledTools(`["Other"]`); err == nil {
		t.Fatal("enabled tools with a failing bump: want an error")
	}
	var n int
	var enabled bool
	if err := d.QueryRow(`SELECT count(*) FROM intercept_rules WHERE name=$1`, name+"-2").Scan(&n); err != nil || n != 0 {
		t.Fatalf("rolled-back create left %d rows (%v)", n, err)
	}
	if err := d.QueryRow(`SELECT enabled FROM intercept_rules WHERE id=$1`, id).Scan(&enabled); err != nil || enabled {
		t.Fatalf("rolled-back toggle/delete: rule enabled=%v (%v); want the rule kept, disabled", enabled, err)
	}
	if v, _, err := d.GetSetting(InterceptEnabledToolsKey); err != nil || v != `["Bash"]` {
		t.Fatalf("rolled-back enabled tools = %q (%v); want the previous list", v, err)
	}
	// The counter is put back so the delete can land and bump.
	if _, err := d.Exec(`UPDATE settings SET value='7' WHERE key=$1`, InterceptRulesRevisionKey); err != nil {
		t.Fatal(err)
	}
	bumped("delete", func() error { return d.DeleteInterceptRule(id) })
}
