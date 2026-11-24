package db

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func lifecycleInput(t *testing.T, notBefore, notAfter time.Time) AuthorizationInput {
	t.Helper()
	in, err := AuthorizationInput{Reference: "SOW-L", Scope: []AuthScopeEntry{{"domain", "example.com"}},
		NotBefore: notBefore, NotAfter: notAfter}.Normalize(knownTestTechniques)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// An artifact requires the target gate: storing one turns it on, and it
// cannot be turned off while the task has one.
func TestAuthorizationForcesTargetGatePG(t *testing.T) {
	d := openTargetDB(t)
	now := time.Now()
	task := targetTask(t, d, TaskCreateOptions{})
	if task.TargetGate {
		t.Fatal("a new task starts gated")
	}
	off := false
	if _, err := d.UpdateTask(task.ID, TaskPatch{TargetGate: &off}); err != nil {
		t.Fatalf("gate off without an artifact: %v", err)
	}
	if _, err := d.AddAuthorization(task.ID, lifecycleInput(t, now.Add(-time.Hour), now.Add(time.Hour)), false, "admin"); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetTask(task.ID)
	if err != nil || got == nil || !got.TargetGate {
		t.Fatalf("after AddAuthorization: %+v, %v; want target_gate on", got, err)
	}
	if _, err := d.UpdateTask(task.ID, TaskPatch{TargetGate: &off}); !errors.Is(err, ErrTargetGateRequired) {
		t.Fatalf("gate off with an artifact: err = %v, want ErrTargetGateRequired", err)
	}
	if got, _ := d.GetTask(task.ID); !got.TargetGate {
		t.Fatal("the refused patch turned the gate off")
	}
	// Other fields still patch; turning the gate on is always allowed.
	on, name := true, "renamed"
	if got, err := d.UpdateTask(task.ID, TaskPatch{Name: &name, TargetGate: &on}); err != nil || got.Name != name || !got.TargetGate {
		t.Fatalf("rename + gate on: %+v, %v", got, err)
	}
	if got, err := d.UpdateTask(999999999, TaskPatch{TargetGate: &off}); got != nil || err != nil {
		t.Fatalf("missing task: %+v, %v", got, err)
	}
}

// The gate's cached read sees a new version at once: AddAuthorization invalidates.
func TestCachedAuthorizationInvalidatedPG(t *testing.T) {
	d := openTargetDB(t)
	if d.caches == nil {
		t.Fatal("Open built a DB without caches")
	}
	now := time.Now()
	task := targetTask(t, d, TaskCreateOptions{})
	if a, err := d.CachedAuthorization(task.ID); a != nil || err != nil {
		t.Fatalf("no artifact yet: %+v, %v", a, err)
	}
	v1, err := d.AddAuthorization(task.ID, lifecycleInput(t, now.Add(-time.Hour), now.Add(time.Hour)), false, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if a, err := d.CachedAuthorization(task.ID); err != nil || a == nil || a.Version != 1 {
		t.Fatalf("cached after v1: %+v, %v (the cached nil survived AddAuthorization)", a, err)
	}
	if _, err := d.AddAuthorization(task.ID, v1.Input(), true, "admin"); err != nil {
		t.Fatal(err)
	}
	if a, err := d.CachedAuthorization(task.ID); err != nil || a == nil || !a.Revoked || a.Version != 2 {
		t.Fatalf("cached after revoke: %+v, %v; a revocation must apply to the next call", a, err)
	}
	// A change behind the store's back is served from the cache until the TTL.
	if _, err := d.Exec(`DELETE FROM engagement_authorizations WHERE task_id=$1 AND version=2`, task.ID); err != nil {
		t.Fatal(err)
	}
	if a, _ := d.CachedAuthorization(task.ID); a == nil || a.Version != 2 {
		t.Fatalf("out-of-band delete seen at once: %+v; the read was not cached", a)
	}
	if a, _ := d.CurrentAuthorization(task.ID); a == nil || a.Version != 1 {
		t.Fatalf("uncached read = %+v, want v1", a)
	}
}

func TestTasksWithEndedAuthorizationPG(t *testing.T) {
	d := openTargetDB(t)
	now := time.Now()
	add := func(id int64, in AuthorizationInput, revoked bool) {
		t.Helper()
		if _, err := d.AddAuthorization(id, in, revoked, "admin"); err != nil {
			t.Fatal(err)
		}
	}
	inForce := targetTask(t, d, TaskCreateOptions{})
	add(inForce.ID, lifecycleInput(t, now.Add(-time.Hour), now.Add(time.Hour)), false)
	expired := targetTask(t, d, TaskCreateOptions{})
	add(expired.ID, lifecycleInput(t, now.Add(-2*time.Hour), now.Add(-time.Minute)), false)
	revoked := targetTask(t, d, TaskCreateOptions{})
	add(revoked.ID, lifecycleInput(t, now.Add(-time.Hour), now.Add(time.Hour)), true)
	future := targetTask(t, d, TaskCreateOptions{})
	add(future.ID, lifecycleInput(t, now.Add(time.Hour), now.Add(2*time.Hour)), false)
	renewed := targetTask(t, d, TaskCreateOptions{}) // expired v1, in-force v2
	add(renewed.ID, lifecycleInput(t, now.Add(-2*time.Hour), now.Add(-time.Minute)), false)
	add(renewed.ID, lifecycleInput(t, now.Add(-time.Hour), now.Add(time.Hour)), false)
	pausedExpired := targetTask(t, d, TaskCreateOptions{})
	add(pausedExpired.ID, lifecycleInput(t, now.Add(-2*time.Hour), now.Add(-time.Minute)), false)
	if err := d.SetPaused(pausedExpired.ID, true); err != nil {
		t.Fatal(err)
	}
	doneExpired := targetTask(t, d, TaskCreateOptions{})
	add(doneExpired.ID, lifecycleInput(t, now.Add(-2*time.Hour), now.Add(-time.Minute)), false)
	if _, err := d.Exec(`UPDATE tasks SET status='done' WHERE id=$1`, doneExpired.ID); err != nil {
		t.Fatal(err)
	}
	none := targetTask(t, d, TaskCreateOptions{})

	ids, err := d.TasksWithEndedAuthorization(now)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int64{expired.ID, revoked.ID} {
		if !slices.Contains(ids, want) {
			t.Errorf("task %d missing from %v", want, ids)
		}
	}
	for _, not := range []int64{inForce.ID, future.ID, renewed.ID, pausedExpired.ID, doneExpired.ID, none.ID} {
		if slices.Contains(ids, not) {
			t.Errorf("task %d listed as ended: %v", not, ids)
		}
	}
}

func TestAuthorizationEnded(t *testing.T) {
	now := time.Now()
	a := &EngagementAuthorization{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	if a.Ended(now) {
		t.Error("in force reported ended")
	}
	if !a.Ended(a.NotAfter) {
		t.Error("not ended at not_after")
	}
	if (&EngagementAuthorization{NotBefore: now.Add(time.Hour), NotAfter: now.Add(2 * time.Hour)}).Ended(now) {
		t.Error("a window that has not started reported ended")
	}
	if !(&EngagementAuthorization{Revoked: true, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}).Ended(now) {
		t.Error("revoked not ended")
	}
}
