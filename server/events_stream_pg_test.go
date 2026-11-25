package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// frameReader opens GET /api/events/stream with tok and returns its frames.
// The channel yields after the response headers arrive, i.e. once the
// subscription is in place.
func frameReader(t *testing.T, ctx context.Context, base, tok, query string) <-chan eventFrame {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/api/events/stream"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("events stream%s: %d", query, resp.StatusCode)
	}
	out := make(chan eventFrame, 1024)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			data, ok := strings.CutPrefix(sc.Text(), "data: ")
			if !ok {
				continue
			}
			var f eventFrame
			if json.Unmarshal([]byte(data), &f) == nil {
				out <- f
			}
		}
	}()
	return out
}

// awaitFrame reads frames until want matches one, failing after the deadline.
// It returns every frame read, including the match.
func awaitFrame(t *testing.T, frames <-chan eventFrame, within time.Duration, what string, want func(eventFrame) bool) []eventFrame {
	t.Helper()
	var seen []eventFrame
	deadline := time.After(within)
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("%s: stream closed after %+v", what, seen)
			}
			seen = append(seen, f)
			if want(f) {
				return seen
			}
		case <-deadline:
			t.Fatalf("%s: not within %s; saw %+v", what, within, seen)
		}
	}
}

// TestEventsStreamPushesScopedChangesPG: the stream delivers finding, approval
// and task-status changes within about a second, to an operator in full and to
// a client_read session bound to company A only for A's findings and tasks.
func TestEventsStreamPushesScopedChangesPG(t *testing.T) {
	fx := newClientFixture(t)
	s := fx.h
	hs := httptest.NewServer(s.Handler())
	t.Cleanup(hs.Close)
	// Registered after hs.Close so it runs first: Close waits for the open streams.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.ctx = ctx // the fixture's server context is already cancelled

	admin, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	user := seedUser(t, s, db.RoleClientRead, "client-pass-123456")
	_, tok := loginAs(t, s, user.Username, "client-pass-123456")
	if w := call(s.Handler(), admin, "PUT", fmt.Sprintf("/api/users/%d/companies", user.ID),
		fmt.Sprintf(`{"company_ids":[%d]}`, fx.companyA)); w.Code != 200 {
		t.Fatalf("bind: %d %s", w.Code, w.Body)
	}
	// A company the client is not bound to is refused before streaming.
	if w := call(s.Handler(), tok, "GET", fmt.Sprintf("/api/events/stream?company=%d", fx.companyB), ""); w.Code != 404 {
		t.Fatalf("client stream of company B: %d", w.Code)
	}

	adminFrames := frameReader(t, ctx, hs.URL, admin, "")
	// The first subscriber starts the listener, which announces itself with a resync.
	awaitFrame(t, adminFrames, 5*time.Second, "listener ready", func(f eventFrame) bool { return f.Kind == eventKindResync })
	clientFrames := frameReader(t, ctx, hs.URL, tok, fmt.Sprintf("?company=%d", fx.companyA))

	const within = 2 * time.Second
	isFinding := func(id int64) func(eventFrame) bool {
		return func(f eventFrame) bool { return f.Kind == db.ChangeFinding && f.ID == id }
	}

	// B's finding: the operator sees it; the client must not.
	findB, err := s.m.pg.AddFinding(fx.taskB, 0, "sqli", "events b", "high", "s", "e", "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitFrame(t, adminFrames, within, "operator: B finding", isFinding(findB))

	// An approval request on A's task: operators only.
	var approval int64
	if err := s.m.pg.QueryRow(`INSERT INTO intercept_pending(task_id, tool_name, reason)
		VALUES ($1, 'events_test', 'events test') RETURNING id`, strconv.FormatInt(fx.taskA, 10)).Scan(&approval); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = s.m.pg.Exec(`DELETE FROM intercept_pending WHERE id = $1`, approval) })
	awaitFrame(t, adminFrames, within, "operator: approval", func(f eventFrame) bool {
		return f.Kind == db.ChangeApproval && f.ID == approval && f.Op == db.ChangeInsert
	})

	// A triage change on A's finding reaches both, timed.
	start := time.Now()
	if _, err := s.m.pg.Exec(`UPDATE findings SET status = 'confirmed' WHERE id = $1`, fx.findA); err != nil {
		t.Fatal(err)
	}
	awaitFrame(t, adminFrames, within, "operator: A finding", isFinding(fx.findA))
	clientSeen := awaitFrame(t, clientFrames, within, "client: A finding", isFinding(fx.findA))
	t.Logf("finding change reached the client in %s", time.Since(start))

	// A's task changes status.
	if _, err := s.m.pg.Exec(`UPDATE tasks SET status = 'done' WHERE id = $1`, fx.taskA); err != nil {
		t.Fatal(err)
	}
	isTaskA := func(f eventFrame) bool { return f.Kind == db.ChangeTask && f.ID == fx.taskA }
	awaitFrame(t, adminFrames, within, "operator: task A", isTaskA)
	clientSeen = append(clientSeen, awaitFrame(t, clientFrames, within, "client: task A", isTaskA)...)

	for _, f := range clientSeen {
		if f.Kind == db.ChangeApproval || (f.Kind == db.ChangeFinding && f.ID != fx.findA) ||
			(f.Kind == db.ChangeTask && f.ID != fx.taskA) {
			t.Errorf("client bound to A received %+v", f)
		}
	}
}
