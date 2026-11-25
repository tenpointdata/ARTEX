package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/notify"
)

// gatedGitHub fronts fakeGitHub: issue reads can be held (to keep a sync
// inside its lock) or failed for one issue number.
type gatedGitHub struct {
	*fakeGitHub
	hold    atomic.Bool
	entered chan struct{}
	release chan struct{}
	failNum atomic.Int64
}

func newGatedGitHub(t *testing.T) (*gatedGitHub, *httptest.Server) {
	gh, _ := newFakeGitHub(t)
	g := &gatedGitHub{fakeGitHub: gh, entered: make(chan struct{}, 1), release: make(chan struct{})}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/repos/acme/app/issues/"
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, prefix) {
			if n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, prefix)); n > 0 && int64(n) == g.failNum.Load() {
				w.WriteHeader(502)
				return
			}
			if g.hold.Load() {
				select {
				case g.entered <- struct{}{}:
				default:
				}
				<-g.release
			}
		}
		g.serve(w, r)
	}))
	t.Cleanup(srv.Close)
	return g, srv
}

type syncFixture struct {
	s       *Server
	pg      *db.DB
	request func(string, string, string) *httptest.ResponseRecorder
	tracker int64
	fid     int64
}

func newSyncFixture(t *testing.T, ghURL string) *syncFixture {
	t.Helper()
	t.Setenv(notify.AllowLocalTargetsEnv, "1")
	s, rec, request := trafficEvidenceServer(t)
	cfg := fmt.Sprintf(`{"kind":"github","name":"gh","config":{"base_url":%q,"repo":"acme/app","token":"ghp_sync_run_secret"}}`, ghURL)
	w := request("POST", "/api/integrations/trackers", cfg)
	if w.Code != 201 {
		t.Fatalf("create tracker: %d %s", w.Code, w.Body)
	}
	var tr ticketTrackerDTO
	_ = json.Unmarshal(w.Body.Bytes(), &tr)
	t.Cleanup(func() { _ = s.m.pg.DeleteTicketTracker(context.Background(), tr.ID) })
	return &syncFixture{s: s, pg: s.m.pg, request: request, tracker: tr.ID, fid: rec.FindingID}
}

func (f *syncFixture) push(t *testing.T, ids ...int64) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"finding_ids": ids})
	w := f.request("POST", fmt.Sprintf("/api/integrations/trackers/%d/push", f.tracker), string(b))
	if w.Code != 200 || strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("push: %d %s", w.Code, w.Body)
	}
}

func (f *syncFixture) sync(t *testing.T, query string) (int, ticketSyncBatch) {
	t.Helper()
	w := f.request("POST", fmt.Sprintf("/api/integrations/trackers/%d/sync%s", f.tracker, query), ``)
	var out ticketSyncBatch
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (f *syncFixture) status(t *testing.T, id int64) string {
	t.Helper()
	got, err := f.pg.GetFinding(id)
	if err != nil || got == nil {
		t.Fatalf("finding %d: %v", id, err)
	}
	return got.Status
}

// TestTicketSyncIsSerializedPerTracker: while one sync of a tracker is inside
// its advisory lock, a second sync, a push, a destination change and the
// background runner are all refused, and the lock is free again afterwards.
func TestTicketSyncIsSerializedPerTracker(t *testing.T) {
	gh, srv := newGatedGitHub(t)
	f := newSyncFixture(t, srv.URL)
	f.push(t, f.fid)

	gh.hold.Store(true)
	type result struct {
		code  int
		batch ticketSyncBatch
	}
	first := make(chan result, 1)
	go func() {
		code, b := f.sync(t, "")
		first <- result{code, b}
	}()
	select {
	case <-gh.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("first sync never reached the tracker")
	}

	if code, _ := f.sync(t, ""); code != 409 {
		t.Fatalf("concurrent sync: %d, want 409", code)
	}
	b, _ := json.Marshal(map[string]any{"finding_ids": []int64{f.fid}})
	if w := f.request("POST", fmt.Sprintf("/api/integrations/trackers/%d/push", f.tracker), string(b)); w.Code != 409 {
		t.Fatalf("concurrent push: %d %s, want 409", w.Code, w.Body)
	}
	if w := f.request("PATCH", fmt.Sprintf("/api/integrations/trackers/%d", f.tracker),
		`{"config":{"repo":"acme/other","token":"ghp_sync_run_secret"}}`); w.Code != 409 {
		t.Fatalf("destination change during a sync: %d %s, want 409", w.Code, w.Body)
	}
	if err := f.pg.SetSetting(settingTicketSyncInterval, "1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.pg.SetSetting(settingTicketSyncInterval, "0") })
	r := newTicketSyncRunner(f.s)
	r.step(context.Background())
	if _, ran := r.lastRun[f.tracker]; ran {
		t.Fatal("background run did not skip a locked tracker")
	}

	gh.hold.Store(false)
	close(gh.release)
	res := <-first
	if res.code != 200 || len(res.batch.Results) != 1 || res.batch.Results[0].Action != "unchanged" {
		t.Fatalf("first sync: %d %+v", res.code, res.batch)
	}
	if code, out := f.sync(t, ""); code != 200 || len(out.Results) != 1 {
		t.Fatalf("sync after release: %d %+v", code, out)
	}
	if links, _ := f.pg.FindingTicketsByTracker(context.Background(), f.tracker); len(links) != 1 {
		t.Fatalf("links: %d, want the one link kept", len(links))
	}
}

// TestScheduledTicketSyncPicksUpGitHubClose: with the interval set, the
// background runner pulls an issue closed in GitHub into the finding without
// a sync request; with it off (the default) nothing runs.
func TestScheduledTicketSyncPicksUpGitHubClose(t *testing.T) {
	gh, srv := newFakeGitHub(t)
	f := newSyncFixture(t, srv.URL)
	f.push(t, f.fid)
	gh.close(1, "completed")
	t.Cleanup(func() { _ = f.pg.SetSetting(settingTicketSyncInterval, "0") })

	now := time.Now()
	r := newTicketSyncRunner(f.s)
	r.now = func() time.Time { return now }
	if err := f.pg.SetSetting(settingTicketSyncInterval, "0"); err != nil {
		t.Fatal(err)
	}
	r.step(context.Background())
	if st := f.status(t, f.fid); st == db.FindingFixed {
		t.Fatal("background sync ran while off")
	}

	if err := f.pg.SetSetting(settingTicketSyncInterval, "5"); err != nil {
		t.Fatal(err)
	}
	r.step(context.Background())
	if st := f.status(t, f.fid); st != db.FindingFixed {
		t.Fatalf("finding after scheduled sync: %s, want %s", st, db.FindingFixed)
	}

	// Not due again until the interval has passed.
	gh.mu.Lock()
	gh.issues[1]["state"], gh.issues[1]["state_reason"] = "open", "reopened"
	gh.mu.Unlock()
	now = now.Add(time.Minute)
	r.step(context.Background())
	if st := f.status(t, f.fid); st != db.FindingFixed {
		t.Fatalf("ran before the interval elapsed: %s", st)
	}
	now = now.Add(5 * time.Minute)
	r.step(context.Background())
	if st := f.status(t, f.fid); st != db.FindingConfirmed {
		t.Fatalf("finding after the next interval: %s, want %s", st, db.FindingConfirmed)
	}
}

// TestTicketSyncCapAndCursor: a run reconciles at most limit links, the next
// run continues the pass, and a link that keeps failing is passed over in its
// pass instead of starving the rest, then retried in the next pass.
func TestTicketSyncCapAndCursor(t *testing.T) {
	gh, srv := newGatedGitHub(t)
	f := newSyncFixture(t, srv.URL)
	ids := []int64{f.fid}
	for i := 0; i < 2; i++ {
		id, err := f.pg.AddFinding(0, 0, "TEST", fmt.Sprintf("cursor %d", i), "low", "s", "", "test", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = f.pg.Exec(`DELETE FROM findings WHERE id=$1`, id) })
		ids = append(ids, id)
	}
	for _, id := range ids {
		f.push(t, id) // one by one: links get ascending synced_at, issues 1, 2, 3
	}
	gh.failNum.Store(1)

	if code, _ := f.sync(t, "?limit=0"); code != 400 {
		t.Fatalf("limit=0: %d, want 400", code)
	}
	actions := func(b ticketSyncBatch) string {
		var parts []string
		for _, r := range b.Results {
			parts = append(parts, r.ExternalID+":"+r.Action)
		}
		return strings.Join(parts, ",")
	}
	steps := []struct {
		want string
		more bool
	}{
		{"1:error,2:unchanged", true}, // first page of the pass
		{"3:unchanged", false},        // the pass ends; 2 is not revisited in it
		{"1:error,2:unchanged", true}, // a new pass retries the failing link
	}
	for i, st := range steps {
		code, out := f.sync(t, "?limit=2")
		if code != 200 || actions(out) != st.want || out.More != st.more {
			t.Fatalf("step %d: %d %q more=%v, want %q more=%v", i, code, actions(out), out.More, st.want, st.more)
		}
	}
}

// TestTicketSyncDefaultCapLoopsToCompletion: with more links than the default
// cap, calling sync while "more" (what the UI's Sync button does) visits every
// link of the tracker exactly once, in ceil(n/cap) calls.
func TestTicketSyncDefaultCapLoopsToCompletion(t *testing.T) {
	_, srv := newGatedGitHub(t)
	f := newSyncFixture(t, srv.URL)
	ids := []int64{f.fid}
	for i := 0; len(ids) < ticketSyncBatchDefault+50; i++ {
		id, err := f.pg.AddFinding(0, 0, "TEST", fmt.Sprintf("loop %d", i), "low", "s", "", "test", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = f.pg.Exec(`DELETE FROM findings WHERE id=$1`, id) })
		ids = append(ids, id)
	}
	f.push(t, ids...)

	seen := map[string]int{}
	calls := 0
	for more := true; more; calls++ {
		if calls > 5 {
			t.Fatalf("sync did not finish its pass after %d calls", calls)
		}
		code, out := f.sync(t, "")
		if code != 200 || len(out.Results) > ticketSyncBatchDefault {
			t.Fatalf("call %d: %d, %d results", calls, code, len(out.Results))
		}
		for _, r := range out.Results {
			seen[r.ExternalID]++
		}
		more = out.More
	}
	if calls != 2 || len(seen) != len(ids) {
		t.Fatalf("calls=%d distinct=%d, want 2 and %d", calls, len(seen), len(ids))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("link %s visited %d times in one pass", id, n)
		}
	}
}
