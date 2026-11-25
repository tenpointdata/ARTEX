package server

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Autumn-27/artex/db"
)

// Ticket sync runs: the per-tracker lock, the capped batch with its cursor,
// and the optional background sync (E2 follow-up, c-da9198).
//
// Every push and sync of one tracker runs under the database advisory lock
// ticket_sync:<id>, so two requests (or a request and the background run, or
// two ARTEX instances on one database) never reconcile the same links at once.
// A busy lock is refused (409 to the API, a skipped round in the background),
// never waited on: the run holding it already does the work.

const (
	// settingTicketSyncInterval is the background sync period in minutes.
	// Unset or 0 = off (the default): sync then runs only on request.
	settingTicketSyncInterval = "ticket_sync_interval_min"
	ticketSyncIntervalMax     = 24 * 60

	// ticketSyncBatchDefault caps the links one run reconciles; each link is
	// one sequential round trip to the tracker. The API may ask for up to
	// ticketSyncBatchMax.
	ticketSyncBatchDefault = 100
	ticketSyncBatchMax     = 500

	// ticketSyncTick is how often the background runner checks for due trackers.
	ticketSyncTick = 30 * time.Second
)

// errTicketSyncBusy is returned when another push or sync of the tracker holds its lock.
var errTicketSyncBusy = errors.New("a push or sync of this tracker is already running")

func ticketSyncLockName(trackerID int64) string {
	return "ticket_sync:" + strconv.FormatInt(trackerID, 10)
}

// withTrackerLock runs fn under the tracker's advisory lock, or returns
// errTicketSyncBusy when it is held.
func withTrackerLock(ctx context.Context, pg *db.DB, trackerID int64, fn func(context.Context) error) error {
	err := pg.WithTryAdvisoryLock(ctx, ticketSyncLockName(trackerID), fn)
	if errors.Is(err, db.ErrLockBusy) {
		return errTicketSyncBusy
	}
	return err
}

// ticketCursors holds each tracker's sync cursor in memory. The zero value is
// ready. A restart (or another instance) starts a new pass, which costs one
// extra visit per link, never a skipped one.
type ticketCursors struct {
	mu  sync.Mutex
	cur map[int64]db.TicketSyncCursor
}

func (c *ticketCursors) get(id int64) db.TicketSyncCursor {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur[id]
}

func (c *ticketCursors) set(id int64, v db.TicketSyncCursor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cur == nil {
		c.cur = map[int64]db.TicketSyncCursor{}
	}
	c.cur[id] = v
}

// ticketSyncBatch is what one capped run did.
type ticketSyncBatch struct {
	Results []ticketSyncResult `json:"results"`
	// More reports that the pass has links left; the next run continues it.
	More bool `json:"more"`
}

// syncBatch reconciles up to limit links of the syncer's tracker from its
// cursor. The caller holds the tracker lock.
func (s *Server) syncBatch(ctx context.Context, sy *ticketSyncer, limit int) (ticketSyncBatch, error) {
	links, next, done, err := sy.pg.FindingTicketsPage(ctx, sy.tracker.ID, s.ticketCursors.get(sy.tracker.ID), limit)
	if err != nil {
		return ticketSyncBatch{}, err
	}
	out := ticketSyncBatch{Results: make([]ticketSyncResult, 0, len(links)), More: !done}
	for _, l := range links {
		if ctx.Err() != nil {
			// Stopped mid-page: keep the cursor so the next run revisits this page.
			return out, ctx.Err()
		}
		out.Results = append(out.Results, sy.sync(ctx, l))
	}
	s.ticketCursors.set(sy.tracker.ID, next)
	return out, nil
}

// ticketSyncInterval reads the background sync period; 0 = off.
func ticketSyncInterval(pg *db.DB) int {
	v, ok, _ := pg.GetSetting(settingTicketSyncInterval)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 || n > ticketSyncIntervalMax {
		return 0
	}
	return n
}

// ticketSyncRunner is the background sync. It reads the interval every tick,
// so turning it on or off applies without a restart.
type ticketSyncRunner struct {
	s       *Server
	pg      *db.DB
	tick    time.Duration
	now     func() time.Time
	lastRun map[int64]time.Time // tracker id → start of its last full-interval run
}

func newTicketSyncRunner(s *Server) *ticketSyncRunner {
	return &ticketSyncRunner{s: s, pg: s.m.pg, tick: ticketSyncTick, now: time.Now, lastRun: map[int64]time.Time{}}
}

// Run loops until ctx ends. Started once by server.New, beside the Scheduler.
func (r *ticketSyncRunner) Run(ctx context.Context) {
	if r.pg == nil {
		return
	}
	t := time.NewTicker(r.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.step(ctx)
		}
	}
}

// step syncs every enabled tracker whose interval has elapsed. A tracker
// whose pass has links left runs again on the next tick, so a large tracker
// is worked through one capped batch per tick instead of one per interval.
// Failures only log: the next round retries.
func (r *ticketSyncRunner) step(ctx context.Context) {
	mins := ticketSyncInterval(r.pg)
	if mins == 0 {
		return
	}
	trackers, err := r.pg.ListTicketTrackers(ctx)
	if err != nil {
		log.Printf("[ticket-sync] list trackers: %v", err)
		return
	}
	baseURL := notifyPublicBaseURL(r.pg)
	now := r.now()
	for _, t := range trackers {
		if ctx.Err() != nil {
			return
		}
		if !t.Enabled {
			continue
		}
		if last, ok := r.lastRun[t.ID]; ok && now.Sub(last) < time.Duration(mins)*time.Minute {
			continue
		}
		sy, err := newTicketSyncer(r.pg, t, baseURL)
		if err != nil {
			log.Printf("[ticket-sync] tracker %d: %v", t.ID, err)
			r.lastRun[t.ID] = now
			continue
		}
		var batch ticketSyncBatch
		err = withTrackerLock(ctx, r.pg, t.ID, func(ctx context.Context) error {
			var err error
			batch, err = r.s.syncBatch(ctx, sy, ticketSyncBatchDefault)
			return err
		})
		switch {
		case errors.Is(err, errTicketSyncBusy):
			continue // a request is syncing it now; try again next tick
		case err != nil:
			log.Printf("[ticket-sync] tracker %d: %v", t.ID, err)
		}
		if batch.More {
			delete(r.lastRun, t.ID) // continue the pass next tick
		} else {
			r.lastRun[t.ID] = now
		}
	}
}
