package db

import (
	"testing"
	"time"
)

// The list summary agrees with CurrentAuthorization + Ended for every case the
// pause loop distinguishes, and for paused or finished tasks too (the console
// needs them to explain a refused resume).
func TestAuthorizationSummariesPG(t *testing.T) {
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
	if err := d.SetPaused(expired.ID, true); err != nil {
		t.Fatal(err)
	}
	revoked := targetTask(t, d, TaskCreateOptions{})
	add(revoked.ID, lifecycleInput(t, now.Add(-time.Hour), now.Add(time.Hour)), true)
	future := targetTask(t, d, TaskCreateOptions{})
	add(future.ID, lifecycleInput(t, now.Add(time.Hour), now.Add(2*time.Hour)), false)
	renewed := targetTask(t, d, TaskCreateOptions{})
	add(renewed.ID, lifecycleInput(t, now.Add(-2*time.Hour), now.Add(-time.Minute)), false)
	add(renewed.ID, lifecycleInput(t, now.Add(-time.Hour), now.Add(time.Hour)), false)
	none := targetTask(t, d, TaskCreateOptions{})

	got, err := d.AuthorizationSummaries(now)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]AuthorizationSummary{
		inForce.ID: {Version: 1},
		expired.ID: {Version: 1, Ended: true},
		revoked.ID: {Version: 1, Revoked: true, Ended: true},
		future.ID:  {Version: 1},
		renewed.ID: {Version: 2},
	}
	for id, w := range want {
		if g, ok := got[id]; !ok || g != w {
			t.Errorf("task %d: got %+v (present %v), want %+v", id, g, ok, w)
		}
		a, err := d.CurrentAuthorization(id)
		if err != nil {
			t.Fatal(err)
		}
		if s := SummarizeAuthorization(a, now); s == nil || *s != got[id] {
			t.Errorf("task %d: SummarizeAuthorization %+v disagrees with the list %+v", id, s, got[id])
		}
	}
	if _, ok := got[none.ID]; ok {
		t.Errorf("task %d without an artifact is listed", none.ID)
	}
	if SummarizeAuthorization(nil, now) != nil {
		t.Error("SummarizeAuthorization(nil) is not nil")
	}
}
