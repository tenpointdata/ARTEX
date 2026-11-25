package db

import (
	"slices"
	"testing"
)

// RetiredTasks names deleted, archived and missing tasks and the
// explorations only they used; a live task, its exploration and an
// exploration no task has named are not retired.
func TestRetiredTasks(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	mk := func() *Task {
		t.Helper()
		task, err := d.CreateTask("retired-tasks test", "goal", nil, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = d.DeleteTask(task.ID) })
		return task
	}
	live, archived, deleted := mk(), mk(), mk()
	if _, err := d.Exec(`UPDATE tasks SET archived_at=now(), deleted_at=now() WHERE id=$1`, archived.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteTask(deleted.ID); err != nil {
		t.Fatal(err)
	}
	var standalone int64
	if err := d.QueryRow(`INSERT INTO explorations(description, goal) VALUES ('', 'x') RETURNING id`).Scan(&standalone); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM explorations WHERE id=$1`, standalone) })

	tasks, exps, err := d.RetiredTasks(
		[]int64{live.ID, archived.ID, deleted.ID},
		[]int64{live.ExplorationID, archived.ExplorationID, deleted.ExplorationID, standalone})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{archived.ID, deleted.ID}; !slices.Equal(tasks, want) {
		t.Fatalf("retired tasks = %v, want %v", tasks, want)
	}
	if want := []int64{archived.ExplorationID, deleted.ExplorationID}; !slices.Equal(exps, want) {
		t.Fatalf("retired explorations = %v, want %v", exps, want)
	}
	if tasks, exps, err := d.RetiredTasks(nil, nil); err != nil || tasks != nil || exps != nil {
		t.Fatalf("empty input = %v %v %v", tasks, exps, err)
	}
}
