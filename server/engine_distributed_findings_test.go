package server

import (
	"testing"

	"github.com/Autumn-27/artex/db"
)

// The control node relays a worker node's findings to the planner as finding
// triggers (which intent found what), as a local report_finding does; a
// finding a local worker wrote already woke the planner and is not relayed
// again.
func TestRelayRemoteFindings(t *testing.T) {
	task := &Task{notify: make(chan struct{}, 1)}
	relayRemoteFindings(task, []db.FindingNode{
		{ID: 1, Worker: "node-b/work#1", IntentID: 7, Summary: "IDOR on /orders"},
		{ID: 2, Worker: "work#0", IntentID: 8, Summary: "local, already notified"},
		{ID: 3, Worker: "node-c/work#2", IntentID: 9, Summary: "SQLi on /search"},
	})
	ev := task.drainTriggers()
	if len(ev) != 2 || ev[0].Kind != "finding" || ev[0].IntentID != 7 || ev[0].Detail != "IDOR on /orders" ||
		ev[1].IntentID != 9 || ev[1].Detail != "SQLi on /search" {
		t.Fatalf("triggers = %+v; want the two remote findings", ev)
	}
	select {
	case <-task.notify:
	default:
		t.Fatal("the planner was not woken")
	}
	relayRemoteFindings(task, []db.FindingNode{{ID: 4, Worker: "work#1"}})
	if ev := task.drainTriggers(); len(ev) != 0 {
		t.Fatalf("local-only findings relayed: %+v", ev)
	}
}
