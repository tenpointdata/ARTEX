package server

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Autumn-27/artex/db"
)

// changeHub fans the database's row-change notifications (db.ListenChanges)
// out to the console's event streams. It starts on the first subscriber and
// runs until the server's context ends, holding one pool connection.
//
// Delivery is lossy by design: a subscriber that falls behind, and every
// subscriber after the listener reconnects, gets a resync event telling it to
// re-read everything. The database stays the source of truth.
type changeHub struct {
	once sync.Once
	mu   sync.Mutex
	subs map[*changeSub]struct{}
}

// hubEvent is a change, or a resync (Change is zero).
type hubEvent struct {
	Change db.Change
	Resync bool
}

// changeSubBuffer is how many events a slow subscriber may lag before it is
// collapsed into one resync.
const changeSubBuffer = 256

type changeSub struct {
	ch      chan hubEvent
	mu      sync.Mutex
	overrun bool // dropped events; the next delivery is a resync
}

// changeListener is the source of row changes; *db.DB satisfies it.
type changeListener interface {
	ListenChanges(ctx context.Context, ready func(), fn func(db.Change)) error
}

// subscribe registers a subscriber, starting the listener on first use.
func (h *changeHub) subscribe(ctx context.Context, src changeListener) (<-chan hubEvent, func()) {
	h.once.Do(func() { go h.run(ctx, src) })
	sub := &changeSub{ch: make(chan hubEvent, changeSubBuffer)}
	h.mu.Lock()
	if h.subs == nil {
		h.subs = map[*changeSub]struct{}{}
	}
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	return sub.ch, func() {
		h.mu.Lock()
		delete(h.subs, sub)
		h.mu.Unlock()
	}
}

// publish delivers ev to every subscriber without blocking.
func (h *changeHub) publish(ev hubEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		sub.offer(ev)
	}
}

func (s *changeSub) offer(ev hubEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.overrun {
		ev = hubEvent{Resync: true}
	}
	select {
	case s.ch <- ev:
		s.overrun = false
	default:
		s.overrun = true
	}
}

// Listener reconnect backoff.
const (
	changeRetryMin = time.Second
	changeRetryMax = 30 * time.Second
)

// run listens until ctx ends, reconnecting with backoff. Every (re)connect
// publishes a resync: changes committed while no LISTEN was in place are lost.
func (h *changeHub) run(ctx context.Context, src changeListener) {
	wait := changeRetryMin
	for {
		connected := false
		err := src.ListenChanges(ctx, func() {
			connected = true
			h.publish(hubEvent{Resync: true})
		}, func(c db.Change) {
			h.publish(hubEvent{Change: c})
		})
		if ctx.Err() != nil {
			return
		}
		if connected {
			wait = changeRetryMin
		}
		log.Printf("[events] change listener stopped, retrying in %s: %v", wait, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, changeRetryMax)
	}
}
