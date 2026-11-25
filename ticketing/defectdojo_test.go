package ticketing

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const ddKey = "dd-SECRET-API-KEY-0001"

// fakeDD is a stateful DefectDojo findings API for test 7.
type fakeDD struct {
	mu       sync.Mutex
	findings map[int]map[string]any
	nextID   int
}

func (f *fakeDD) render(t *testing.T, id int) string {
	m := f.findings[id]
	b := func(k string) bool { v, _ := m[k].(bool); return v }
	s := func(k string) string { v, _ := m[k].(string); return mustJSON(v) }
	return fixture(t, "defectdojo_finding.json", id, s("title"), s("severity"), s("numerical_severity"), s("description"),
		b("active"), b("verified"), b("false_p"), b("risk_accepted"), b("is_mitigated"), b("duplicate"), s("unique_id_from_tool"))
}

func (f *fakeDD) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Token "+ddKey {
				writeJSON(w, 401, `{"detail":"Invalid token."}`)
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			h(w, r)
		}
	}
	mux.HandleFunc("GET /api/v2/findings/", auth(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var hits []string
		for id, m := range f.findings {
			if q.Get("test") == "7" && m["unique_id_from_tool"] == q.Get("unique_id_from_tool") {
				hits = append(hits, f.render(t, id))
			}
		}
		writeJSON(w, 200, fixture(t, "defectdojo_findings_list.json", len(hits), strings.Join(hits, ",")))
	}))
	mux.HandleFunc("POST /api/v2/findings/", auth(func(w http.ResponseWriter, r *http.Request) {
		b := readBody(r)
		if b["test"] != float64(7) || b["title"] == "" || b["severity"] == nil {
			writeJSON(w, 400, `{"test":["required"]}`)
			return
		}
		f.nextID++
		f.findings[f.nextID] = b
		writeJSON(w, 201, f.render(t, f.nextID))
	}))
	mux.HandleFunc("GET /api/v2/findings/{id}/", auth(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		if _, ok := f.findings[id]; !ok {
			writeJSON(w, 404, `{"detail":"Not found."}`)
			return
		}
		writeJSON(w, 200, f.render(t, id))
	}))
	mux.HandleFunc("PATCH /api/v2/findings/{id}/", auth(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		m, ok := f.findings[id]
		if !ok {
			writeJSON(w, 404, `{"detail":"Not found."}`)
			return
		}
		for k, v := range readBody(r) {
			m[k] = v
		}
		writeJSON(w, 200, f.render(t, id))
	}))
	return mux
}

func (f *fakeDD) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.findings)
}

func ddCfg(base string) map[string]any {
	return map[string]any{"base_url": base, "api_key": ddKey, "test_id": float64(7), "found_by": []any{float64(3)}}
}

func TestDefectDojoValidate(t *testing.T) {
	tr, _ := Get(KindDefectDojo)
	cases := []struct {
		cfg  map[string]any
		want string
	}{
		{map[string]any{"api_key": "k", "test_id": 1}, "base_url"},
		{map[string]any{"base_url": "https://dd.test", "test_id": 1}, "api_key"},
		{map[string]any{"base_url": "https://dd.test", "api_key": "k"}, "test_id"},
		{map[string]any{"base_url": "https://dd.test", "api_key": "k", "test_id": float64(-2)}, "test_id"},
	}
	for _, c := range cases {
		if err := tr.Validate(c.cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Validate(%v) = %v, want error naming %q", c.cfg, err, c.want)
		}
	}
	if err := tr.Validate(map[string]any{"base_url": "https://dd.test", "api_key": "k", "test_id": "12"}); err != nil {
		t.Fatalf("string test_id rejected: %v", err)
	}
}

func TestDefectDojoLifecycle(t *testing.T) {
	fake := &fakeDD{findings: map[int]map[string]any{}}
	srv, rec := newFake(t, fake.handler(t))
	tr, _ := Get(KindDefectDojo)
	ref := runLifecycle(t, tr, ddCfg(srv.URL), fake.count)
	if ref.ID != "1" || ref.URL != srv.URL+"/finding/1" {
		t.Fatalf("ref = %+v", ref)
	}
	create := rec.matching("POST", "/api/v2/findings/")[0]
	if create.Header.Get("Authorization") != "Token "+ddKey {
		t.Fatalf("auth header = %q", create.Header.Get("Authorization"))
	}
	b := create.json(t)
	if b["severity"] != "High" || b["numerical_severity"] != "S1" || b["unique_id_from_tool"] != DedupKey("eng-1", "finding-1") ||
		b["test"] != float64(7) || b["active"] != true {
		t.Fatalf("create body = %v", b)
	}
	if fb, _ := b["found_by"].([]any); len(fb) != 1 || fb[0] != float64(3) {
		t.Fatalf("found_by = %v", b["found_by"])
	}
	search := rec.matching("GET", "/api/v2/findings/")[0]
	if q, _ := url.ParseQuery(search.Query); q.Get("test") != "7" || q.Get("unique_id_from_tool") == "" {
		t.Fatalf("search query = %q", search.Query)
	}
}

func TestDefectDojoFlagsPerResolution(t *testing.T) {
	want := map[Resolution]map[string]bool{
		ResNone:          {"active": true},
		ResFixed:         {"is_mitigated": true},
		ResFalsePositive: {"false_p": true},
		ResRiskAccepted:  {"risk_accepted": true},
		ResDuplicate:     {"duplicate": true},
		ResWontFix:       {},
	}
	for res, set := range want {
		f := ddFlags(Ticket{Open: res == ResNone, Resolution: res})
		for _, k := range []string{"active", "is_mitigated", "false_p", "risk_accepted", "duplicate"} {
			if got, _ := f[k].(bool); got != set[k] {
				t.Errorf("resolution %q: %s = %v, want %v", res, k, got, set[k])
			}
		}
	}
}

// verified must never be true unless the Ticket says so, and an update must not
// clear a verification a human set.
func TestDefectDojoNeverInfersVerified(t *testing.T) {
	fake := &fakeDD{findings: map[int]map[string]any{}}
	srv, rec := newFake(t, fake.handler(t))
	tr, _ := Get(KindDefectDojo)
	cfg, ctx := ddCfg(srv.URL), context.Background()
	tk := Ticket{Key: DedupKey("v"), Title: "t", Body: "b", Severity: SevCritical, Open: true}
	ref, _, err := Push(ctx, tr, cfg, nil, tk)
	if err != nil {
		t.Fatal(err)
	}
	if v, present := rec.matching("POST", "/api/v2/findings/")[0].json(t)["verified"]; !present || v != false {
		t.Fatalf("create verified = %v (present=%v), want explicit false", v, present)
	}
	fake.findings[1]["verified"] = true // a human verified it in DefectDojo
	tk.Open, tk.Resolution = false, ResFixed
	if err := tr.Update(ctx, cfg, ref, tk); err != nil {
		t.Fatal(err)
	}
	if _, present := rec.matching("PATCH", "/api/v2/findings/1/")[0].json(t)["verified"]; present || fake.findings[1]["verified"] != true {
		t.Fatalf("update touched verified: %v", rec.matching("PATCH", "/api/v2/findings/1/")[0].json(t))
	}
	// An explicitly verified ticket creates and updates with verified=true.
	vt := Ticket{Key: DedupKey("w"), Title: "t", Body: "b", Severity: SevMedium, Open: true, Verified: true}
	vref, _, err := Push(ctx, tr, cfg, nil, vt)
	if err != nil {
		t.Fatal(err)
	}
	if fake.findings[2]["verified"] != true || fake.findings[2]["numerical_severity"] != "S2" {
		t.Fatalf("verified create = %v", fake.findings[2])
	}
	delete(fake.findings[2], "verified")
	if err := tr.Update(ctx, cfg, vref, vt); err != nil || fake.findings[2]["verified"] != true {
		t.Fatalf("verified update: %v %v", err, fake.findings[2])
	}
}

func TestDefectDojoErrorsDoNotLeakKey(t *testing.T) {
	tr, _ := Get(KindDefectDojo)
	runErrorChecks(t, tr, ddCfg, ddKey)
}
