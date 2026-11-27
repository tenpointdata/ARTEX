package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/Autumn-27/artex/db"
)

// GET /api/events/stream — the console's live push (Server-Sent Events).
//
// Each message is a JSON eventFrame telling the page that a task's status, a
// finding or the approval queue changed, or (kind "resync") that it may have
// missed changes and should re-read everything. Frames carry ids only, never
// row data: the page re-reads through the normal, access-checked endpoints.
//
// Visibility follows the read routes: a role with read sees every frame; a
// client_read session (bound to ?company= by bindCompany) sees only changes to
// findings and tasks of that company, judged with the same strict rules as
// /api/exploration/findings and the task binders, and never the approval
// queue or a deletion (the row is gone, so its company cannot be judged; the
// page's slow poll picks deletions up).

// eventFrame is one message on the stream.
type eventFrame struct {
	Kind string `json:"kind"` // db.ChangeFinding | db.ChangeApproval | db.ChangeTask | "resync"
	Op   string `json:"op,omitempty"`
	ID   int64  `json:"id,omitempty"`
	Task string `json:"task,omitempty"`
}

const eventKindResync = "resync"

// eventsPing is the keepalive period; each ping re-checks the session.
const eventsPing = 20 * time.Second

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return
	}
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	tok := extractToken(r)
	if !s.sessionOK(tok) {
		writeErr(w, 401, "token is invalid or expired")
		return
	}
	p, _ := principalFrom(r.Context())
	sc, client := clientScopeFrom(r.Context())
	if !client && !roleHas(p.Role, permRead) {
		writeErr(w, 403, "forbidden")
		return
	}
	visible := func(c db.Change) bool { return true }
	if client {
		visible = func(c db.Change) bool { return s.clientSeesChange(sc.Company, c) }
	}

	events, unsub := s.changes.subscribe(s.ctx, s.m.pg)
	defer unsub()
	openSSE(w, flusher)
	send := func(f eventFrame) {
		b, _ := json.Marshal(f)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}
	// Reconnect delay for the browser's EventSource.
	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()

	ctx := r.Context()
	ping := time.NewTicker(eventsPing)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			if !s.sessionOK(tok) {
				return
			}
			if ev.Resync {
				send(eventFrame{Kind: eventKindResync})
				continue
			}
			if visible(ev.Change) {
				c := ev.Change
				send(eventFrame{Kind: c.Kind, Op: c.Op, ID: c.ID, Task: c.Task})
			}
		case <-ping.C:
			if !s.sessionOK(tok) || (client && !s.clientStillBound(p.UserID, sc.Company)) {
				return
			}
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// clientSeesChange reports whether a client_read session narrowed to company
// may learn of change c. Errors deny.
func (s *Server) clientSeesChange(company int64, c db.Change) bool {
	if c.Op == db.ChangeDelete {
		return false
	}
	switch c.Kind {
	case db.ChangeFinding:
		ok, err := s.m.pg.FindingMatches(clientCompanyFilter(company), c.ID)
		if err != nil {
			log.Printf("[events] client finding %d visibility: %v", c.ID, err)
		}
		return err == nil && ok
	case db.ChangeTask:
		owned, err := s.m.pg.TaskCompaniesIn(c.ID, []int64{company})
		if err != nil {
			log.Printf("[events] client task %d visibility: %v", c.ID, err)
		}
		return err == nil && len(owned) > 0
	default: // the approval queue and any future kind stay operator-only
		return false
	}
}

// clientStillBound re-checks the binding the stream was opened under, so
// unbinding a company ends its live feed.
func (s *Server) clientStillBound(userID, company int64) bool {
	companies, err := s.m.pg.UserCompanies(userID)
	if err != nil {
		log.Printf("[events] client %d binding re-check: %v", userID, err)
		return false
	}
	return slices.Contains(companies, company)
}
