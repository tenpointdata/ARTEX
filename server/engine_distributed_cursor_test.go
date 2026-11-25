package server

import (
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// A fresh task's first poll finds nothing settled. The intents worker nodes
// settle afterwards must still be returned, so the planner wakes for them.
func TestSettleCursorNotifiesAfterAnEmptyFirstPoll(t *testing.T) {
	var c settleCursor
	if got := c.advance(nil, time.Time{}); got != nil {
		t.Fatalf("first poll returned %v", got)
	}
	at := time.Now()
	batch := []db.SettledIntent{{ID: 7, Owner: "node-a/work#1"}}
	if got := c.advance(batch, at); len(got) != 1 || got[0].ID != 7 {
		t.Fatalf("second poll returned %v, want intent 7", got)
	}
	if !c.since.Equal(at) {
		t.Fatalf("cursor %v, want %v", c.since, at)
	}
}

// Intents already settled when the loop starts (a control-node restart) are
// not new: the first poll only places the cursor.
func TestSettleCursorSkipsWhatTheFirstPollFinds(t *testing.T) {
	var c settleCursor
	at := time.Now()
	if got := c.advance([]db.SettledIntent{{ID: 1}}, at); got != nil {
		t.Fatalf("first poll returned %v", got)
	}
	if !c.since.Equal(at) {
		t.Fatalf("cursor %v, want %v", c.since, at)
	}
}
