package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// A subscriber that falls behind loses events but gets one resync instead.
func TestChangeHubOverrunBecomesResync(t *testing.T) {
	var h changeHub
	h.once.Do(func() {}) // no listener
	ch, unsub := h.subscribe(context.Background(), nil)
	defer unsub()
	for i := range changeSubBuffer + 10 {
		h.publish(hubEvent{Change: db.Change{Kind: db.ChangeFinding, Op: db.ChangeInsert, ID: int64(i + 1)}})
	}
	for range changeSubBuffer {
		<-ch
	}
	h.publish(hubEvent{Change: db.Change{Kind: db.ChangeTask, Op: db.ChangeUpdate, ID: 9}})
	if ev := <-ch; !ev.Resync {
		t.Fatalf("after an overrun got %+v, want a resync", ev)
	}
	h.publish(hubEvent{Change: db.Change{Kind: db.ChangeTask, Op: db.ChangeUpdate, ID: 10}})
	if ev := <-ch; ev.Resync || ev.Change.ID != 10 {
		t.Fatalf("after the resync got %+v, want the change", ev)
	}
}

type flakyListener struct{ calls chan struct{} }

func (f *flakyListener) ListenChanges(ctx context.Context, ready func(), fn func(db.Change)) error {
	f.calls <- struct{}{}
	ready()
	fn(db.Change{Kind: db.ChangeFinding, Op: db.ChangeInsert, ID: 1})
	return errors.New("connection lost")
}

// Every (re)connect announces a resync, since changes in the gap are lost.
func TestChangeHubResyncsOnReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := &flakyListener{calls: make(chan struct{}, 4)}
	var h changeHub
	ch, unsub := h.subscribe(ctx, src)
	defer unsub()
	want := []bool{true, false, true} // resync, change, then resync after the retry
	for i, resync := range want {
		select {
		case ev := <-ch:
			if ev.Resync != resync {
				t.Fatalf("event %d = %+v, want resync=%v", i, ev, resync)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("event %d not delivered", i)
		}
	}
}
