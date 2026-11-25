package server

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/ticketing"
)

// Push-based ticket updates (E2, c-c2bdc8):
//
//	POST /api/hooks/trackers/{id}
//
// A tracker (GitHub, Jira) calls this when one of its tickets changes, so a
// tracker-side close reaches the finding without waiting for the background
// sync. The route is outside requireAuth: the caller is authenticated by an
// HMAC-SHA256 of the body under the tracker's webhook_secret (sealed with the
// rest of the tracker config), checked in constant time before the body is
// parsed (ticketing.WebhookReceiver).
//
// The delivery is only a hint. Its payload names the ticket; the state is
// then read from the tracker's API by the ordinary ticketSyncer.sync, under
// the per-tracker lock. A forged state cannot be injected, and a replayed
// delivery can at most cause one more read. Replays inside the window are
// also dropped by delivery id.
//
// Every refusal (unknown tracker, no secret, bad or missing signature) is the
// same 401, so the endpoint does not reveal which tracker ids exist.

const (
	ticketWebhookPrefix   = "/api/hooks/trackers/"
	ticketWebhookBodyMax  = 1 << 20 // GitHub issues payloads are a few KiB
	ticketWebhookInFlight = 4       // concurrent deliveries; more get 503 and are retried by the sender
	ticketWebhookReplay   = 10 * time.Minute
	ticketWebhookSeenMax  = 4096
	ticketWebhookTimeout  = 30 * time.Second
	ticketWebhookActor    = "webhook"
	// ticketWebhookReadMax bounds the body read, so a client dripping bytes
	// cannot hold one of the in-flight slots for the server's 30-minute read timeout.
	ticketWebhookReadMax = 10 * time.Second
	// ticketWebhookIDMax caps a delivery id (an unsigned header) kept in the replay map.
	ticketWebhookIDMax = 128
)

// ticketWebhookDummySecret is hashed for an unknown tracker or a kind without
// webhooks, so that refusal costs the same HMAC over the body as a real check
// and response time does not reveal which tracker ids exist.
const ticketWebhookDummySecret = "artex-webhook-timing-equalizer"

// ticketWebhookGuard bounds concurrent deliveries and remembers recent
// delivery ids. The zero value is ready.
type ticketWebhookGuard struct {
	inFlight atomic.Int32
	mu       sync.Mutex
	seen     map[string]time.Time // "<tracker>:<delivery>" → first seen
}

func (g *ticketWebhookGuard) acquire() bool {
	if g.inFlight.Add(1) > ticketWebhookInFlight {
		g.inFlight.Add(-1)
		return false
	}
	return true
}

func (g *ticketWebhookGuard) release() { g.inFlight.Add(-1) }

// firstSeen records a delivery and reports whether it is new within the
// replay window. An empty id is always new.
func (g *ticketWebhookGuard) firstSeen(trackerID int64, delivery string, now time.Time) bool {
	if delivery == "" || len(delivery) > ticketWebhookIDMax {
		return true
	}
	key := strconv.FormatInt(trackerID, 10) + ":" + delivery
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.seen == nil {
		g.seen = map[string]time.Time{}
	}
	if at, ok := g.seen[key]; ok && now.Sub(at) < ticketWebhookReplay {
		return false
	}
	if len(g.seen) >= ticketWebhookSeenMax {
		for k, at := range g.seen {
			if now.Sub(at) >= ticketWebhookReplay {
				delete(g.seen, k)
			}
		}
		if len(g.seen) >= ticketWebhookSeenMax {
			// Still full of fresh ids: forget them all rather than grow. Losing
			// them only lets a replay cause one more state read.
			g.seen = map[string]time.Time{}
		}
	}
	g.seen[key] = now
	return true
}

// forget drops a delivery id, so a redelivery of a delivery that was not
// processed (lock busy, sync failed) is handled.
func (g *ticketWebhookGuard) forget(trackerID int64, delivery string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.seen, strconv.FormatInt(trackerID, 10)+":"+delivery)
}

// ticketWebhookReply is the response body; it carries counts only, never
// finding data, because the caller is the tracker, not an operator.
type ticketWebhookReply struct {
	Status string `json:"status"` // ok | ignored | duplicate | deferred
	Synced int    `json:"synced,omitempty"`
}

// ticketWebhook: POST /api/hooks/trackers/{id}.
func (s *Server) ticketWebhook(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	if !s.ticketWebhooks.acquire() {
		writeErr(w, 503, "busy, retry later")
		return
	}
	defer s.ticketWebhooks.release()

	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(ticketWebhookReadMax))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, ticketWebhookBodyMax))
	if err != nil {
		writeErr(w, 413, "payload too large")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 401, "Unauthorized")
		return
	}
	t, err := pg.TicketTrackerByID(r.Context(), id)
	if err != nil && !errors.Is(err, db.ErrTicketTrackerNotFound) {
		writeErr(w, 500, "tracker lookup failed")
		return
	}
	var recv ticketing.WebhookReceiver
	if t != nil {
		recv, _ = ticketing.Webhooks(t.Kind)
	}
	if recv == nil {
		_ = ticketing.VerifyHMACSHA256(ticketWebhookDummySecret, body, "")
		writeErr(w, 401, "Unauthorized")
		return
	}
	cfg := trackerConfig(t)
	ev, err := recv.ParseWebhook(cfg, r.Header, body)
	if errors.Is(err, ticketing.ErrWebhookSignature) {
		writeErr(w, 401, "Unauthorized")
		return
	}
	if err != nil {
		writeErr(w, 400, "malformed payload")
		return
	}
	// Ignored deliveries are not recorded, so a redelivery after the tracker is re-enabled is handled.
	if !t.Enabled || len(ev.TicketIDs) == 0 {
		writeJSON(w, 200, ticketWebhookReply{Status: "ignored"})
		return
	}
	if !s.ticketWebhooks.firstSeen(t.ID, ev.DeliveryID, time.Now()) {
		writeJSON(w, 200, ticketWebhookReply{Status: "duplicate"})
		return
	}
	// Finish the sync even if the sender hangs up (GitHub waits 10 s).
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), ticketWebhookTimeout)
	defer cancel()
	reply, err := s.syncWebhookTickets(ctx, pg, t, ev)
	if err != nil || reply.Status == "deferred" {
		s.ticketWebhooks.forget(t.ID, ev.DeliveryID)
	}
	if err != nil {
		log.Printf("[ticket-webhook] tracker %d: %v", t.ID, err)
		writeErr(w, 500, "sync failed")
		return
	}
	code := 200
	if reply.Status == "deferred" {
		code = 202
	}
	writeJSON(w, code, reply)
}

// syncWebhookTickets syncs the links named by a verified delivery under the
// tracker lock. A busy lock means a push or sync of the tracker is running
// now; the delivery is then left to it and to the next scheduled sync.
func (s *Server) syncWebhookTickets(ctx context.Context, pg *db.DB, t *db.TicketTracker, ev ticketing.WebhookEvent) (ticketWebhookReply, error) {
	sy, err := newTicketSyncer(pg, t, notifyPublicBaseURL(pg))
	if err != nil {
		return ticketWebhookReply{}, err
	}
	reply := ticketWebhookReply{Status: "ok"}
	err = withTrackerLock(ctx, pg, t.ID, func(ctx context.Context) error {
		for _, ext := range ev.TicketIDs {
			link, err := pg.FindingTicketByExternal(ctx, t.ID, ext)
			if err != nil {
				return err
			}
			if link == nil {
				continue // a ticket ARTEX did not create
			}
			res := sy.sync(ctx, link)
			reply.Synced++
			if res.Action == "pulled" || res.Action == "error" {
				s.appendAuditBestEffort(ticketWebhookActor, db.AuditTicketWebhook, ticketWebhookPrefix+strconv.FormatInt(t.ID, 10),
					map[string]any{"tracker_id": t.ID, "delivery": ev.DeliveryID, "result": res})
			}
		}
		return nil
	})
	if errors.Is(err, errTicketSyncBusy) {
		return ticketWebhookReply{Status: "deferred"}, nil
	}
	if reply.Synced == 0 && err == nil {
		reply.Status = "ignored"
	}
	return reply, err
}
