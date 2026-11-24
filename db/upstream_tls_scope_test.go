package db

import "testing"

func scopeRule(kind, pattern string) AssetInterceptRule {
	return AssetInterceptRule{Enabled: true, Kind: kind, Pattern: pattern}
}

func TestHostInTaskAllowScope(t *testing.T) {
	tasks := []TaskScopeRules{
		{TaskID: 1, Allow: []AssetInterceptRule{scopeRule("fuzzy_domain", "target.example"), scopeRule("cidr", "10.10.0.0/16")},
			Block: []AssetInterceptRule{scopeRule("exact_domain", "prod.target.example")}},
		{TaskID: 2, Allow: []AssetInterceptRule{scopeRule("exact_ip", "2001:db8::5"), scopeRule("fuzzy_url", "https://app.other.example")}},
	}
	global := []AssetInterceptRule{scopeRule("cidr", "10.10.99.0/24")}
	for host, want := range map[string]bool{
		"target.example":             true,
		"API.target.example.":        true,
		"target.example.attacker.io": false,
		"badtarget.example":          false,
		"prod.target.example":        false, // the task's own block rule
		"10.10.3.4":                  true,
		"10.10.99.4":                 false, // global block rule
		"10.11.0.1":                  false,
		"[2001:db8::5]":              true,
		"::ffff:10.10.3.4":           true,
		"app.other.example":          true,
		"other.example":              false,
		"pypi.org":                   false,
		"":                           false,
	} {
		if got := HostInTaskAllowScope(host, global, tasks); got != want {
			t.Errorf("%q: got %v want %v", host, got, want)
		}
	}
	if HostInTaskAllowScope("target.example", nil, []TaskScopeRules{{TaskID: 3}}) {
		t.Error("a task with no allow rules must not trust anything")
	}
}

// ActiveTaskScopeRules keeps only enabled rules of tasks that can still run.
func TestActiveTaskScopeRulesPG(t *testing.T) {
	d, assets, _ := testSetup(t)
	t.Cleanup(func() { d.Close() })
	active, err := d.CreateTask("tls scope active", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	done, err := d.CreateTask("tls scope done", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupErr(t, "active task", d.DeleteTask(active.ID))
		cleanupErr(t, "done task", d.DeleteTask(done.ID))
	})
	if _, err := d.Exec(`UPDATE tasks SET status='done' WHERE id=$1`, done.ID); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		task          int64
		action, value string
		enabled       bool
	}{
		{active.ID, "allow", "lab.invalid", true},
		{active.ID, "allow", "off.invalid", false},
		{active.ID, "block", "prod.lab.invalid", true},
		{done.ID, "allow", "old.invalid", true},
	} {
		if _, err := assets.CreateTaskInterceptRule(r.task, r.action, "fuzzy_domain", r.value, "", r.enabled); err != nil {
			t.Fatal(err)
		}
	}
	got, err := assets.ActiveTaskScopeRules()
	if err != nil {
		t.Fatal(err)
	}
	var mine *TaskScopeRules
	for i := range got {
		if got[i].TaskID == done.ID {
			t.Fatal("a finished task's rules must not count")
		}
		if got[i].TaskID == active.ID {
			mine = &got[i]
		}
	}
	if mine == nil || len(mine.Allow) != 1 || mine.Allow[0].Pattern != "lab.invalid" || len(mine.Block) != 1 {
		t.Fatalf("active task rules: %+v", mine)
	}
	if !HostInTaskAllowScope("api.lab.invalid", nil, got) || HostInTaskAllowScope("prod.lab.invalid", nil, got) ||
		HostInTaskAllowScope("old.invalid", nil, got) || HostInTaskAllowScope("off.invalid", nil, got) {
		t.Fatal("scope decision over the loaded rules is wrong")
	}
}
