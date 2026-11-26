package ticketing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Autumn-27/artex/notify"
)

// recordedReq is one request seen by a fake tracker.
type recordedReq struct {
	Method, Path, Query string
	Header              http.Header
	Body                []byte
}

func (r recordedReq) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatalf("request body %q is not a JSON object: %v", r.Body, err)
	}
	return m
}

// recorder records every request a fake tracker receives.
type recorder struct {
	mu   sync.Mutex
	reqs []recordedReq
}

func (r *recorder) add(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, recordedReq{Method: req.Method, Path: req.URL.Path, Query: req.URL.RawQuery, Header: req.Header.Clone(), Body: body})
}

// matching returns the recorded requests with the given method and exact path.
func (r *recorder) matching(method, path string) []recordedReq {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []recordedReq
	for _, q := range r.reqs {
		if q.Method == method && q.Path == path {
			out = append(out, q)
		}
	}
	return out
}

func (r *recorder) all() []recordedReq {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedReq(nil), r.reqs...)
}

// newFake starts an httptest server whose handler is wrapped to record requests,
// and allows the notify client to dial loopback for the duration of the test.
func newFake(t *testing.T, h http.Handler) (*httptest.Server, *recorder) {
	t.Helper()
	t.Setenv(notify.AllowLocalTargetsEnv, "1")
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// fixture renders testdata/<name> with fmt.Sprintf arguments.
func fixture(t *testing.T, name string, args ...any) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if len(args) == 0 {
		return string(raw)
	}
	return fmt.Sprintf(string(raw), args...)
}

func readBody(r *http.Request) map[string]any {
	var m map[string]any
	_ = json.NewDecoder(r.Body).Decode(&m)
	return m
}

func strList(v any) []string {
	raw, _ := v.([]any)
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

var allResolutions = []Resolution{ResFixed, ResFalsePositive, ResRiskAccepted, ResDuplicate, ResWontFix}

// runLifecycle drives the contract every adapter must meet, against its fake:
// idempotent Push, every resolution round-tripping through State, reopen.
// creates reports how many tickets the fake has created.
func runLifecycle(t *testing.T, tr Tracker, cfg map[string]any, creates func() int) Ref {
	t.Helper()
	ctx := context.Background()
	tk := Ticket{
		Key: DedupKey("eng-1", "finding-1"), Title: "Reflected XSS in /search", Body: "Details **here**.",
		Severity: SevHigh, Labels: []string{"security"}, Open: true,
	}
	ref, created, err := Push(ctx, tr, cfg, nil, tk)
	if err != nil || !created || ref.ID == "" || ref.URL == "" {
		t.Fatalf("first push: ref=%+v created=%v err=%v", ref, created, err)
	}
	if n := creates(); n != 1 {
		t.Fatalf("creates after first push = %d, want 1", n)
	}
	found, err := tr.Find(ctx, cfg, tk.Key)
	if err != nil || found == nil || found.ID != ref.ID {
		t.Fatalf("Find after create = %+v, %v; want %s", found, err, ref.ID)
	}
	if none, err := tr.Find(ctx, cfg, DedupKey("other")); err != nil || none != nil {
		t.Fatalf("Find of unknown key = %+v, %v; want nil, nil", none, err)
	}

	// Re-push without a stored ref finds by key and updates: no duplicate.
	tk.Title = "Reflected XSS in /search (retitled)"
	ref2, created, err := Push(ctx, tr, cfg, nil, tk)
	if err != nil || created || ref2.ID != ref.ID {
		t.Fatalf("second push: ref=%+v created=%v err=%v", ref2, created, err)
	}
	// With a stored ref, no search is needed.
	if ref3, created, err := Push(ctx, tr, cfg, &ref, tk); err != nil || created || ref3.ID != ref.ID {
		t.Fatalf("push with existing ref: %+v %v %v", ref3, created, err)
	}
	if n := creates(); n != 1 {
		t.Fatalf("creates after re-push = %d, want still 1", n)
	}
	if st, err := tr.State(ctx, cfg, ref); err != nil || !st.Open || st.Resolution != ResNone {
		t.Fatalf("state of open ticket = %+v, %v", st, err)
	}

	for _, res := range allResolutions {
		tk.Open, tk.Resolution = false, res
		if _, created, err := Push(ctx, tr, cfg, nil, tk); err != nil || created {
			t.Fatalf("close as %s: created=%v err=%v", res, created, err)
		}
		st, err := tr.State(ctx, cfg, ref)
		if err != nil || st.Open || st.Resolution != res {
			t.Fatalf("close as %s: state = %+v, %v", res, st, err)
		}
		tk.Open, tk.Resolution = true, ResNone
		if _, created, err := Push(ctx, tr, cfg, nil, tk); err != nil || created {
			t.Fatalf("reopen after %s: created=%v err=%v", res, created, err)
		}
		if st, err := tr.State(ctx, cfg, ref); err != nil || !st.Open || st.Resolution != ResNone {
			t.Fatalf("reopen after %s: state = %+v, %v", res, st, err)
		}
	}
	if n := creates(); n != 1 {
		t.Fatalf("creates after lifecycle = %d, want 1", n)
	}

	// A ticket created already closed ends up closed with its resolution.
	closed := Ticket{Key: DedupKey("eng-1", "finding-2"), Title: "Old issue", Body: "b", Severity: SevLow, Resolution: ResFalsePositive}
	cref, created, err := Push(ctx, tr, cfg, nil, closed)
	if err != nil || !created {
		t.Fatalf("push closed: %v %v", created, err)
	}
	if st, err := tr.State(ctx, cfg, cref); err != nil || st.Open || st.Resolution != ResFalsePositive {
		t.Fatalf("created-closed state = %+v, %v", st, err)
	}
	return ref
}

// runErrorChecks points the adapter at a server that always answers 401 or 500
// and asserts errors are classified and never carry the secret.
func runErrorChecks(t *testing.T, tr Tracker, mkCfg func(base string) map[string]any, secret string) {
	t.Helper()
	ctx := context.Background()
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		srv, _ := newFake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, status, `{"message":"nope"}`)
		}))
		cfg := mkCfg(srv.URL)
		ref := Ref{ID: "1"}
		tk := Ticket{Key: DedupKey("x"), Title: "t", Body: "b", Severity: SevLow, Open: true}
		_, findErr := tr.Find(ctx, cfg, tk.Key)
		_, createErr := tr.Create(ctx, cfg, tk)
		updErr := tr.Update(ctx, cfg, ref, tk)
		_, stErr := tr.State(ctx, cfg, ref)
		for name, err := range map[string]error{"find": findErr, "create": createErr, "update": updErr, "state": stErr} {
			if err == nil {
				t.Fatalf("%s with HTTP %d: want error", name, status)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("%s error leaks the secret: %v", name, err)
			}
			if got := notify.IsPermanent(err); got != (status == http.StatusUnauthorized) {
				t.Fatalf("%s with HTTP %d: IsPermanent = %v", name, status, got)
			}
		}
	}
	// Invalid configuration errors must not echo the secret either.
	bad := mkCfg("not a url")
	if err := tr.Validate(bad); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("Validate(bad base_url) = %v", err)
	}
	if _, err := tr.Create(ctx, bad, Ticket{Key: "k", Title: "t"}); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("Create with bad config = %v", err)
	}
}
