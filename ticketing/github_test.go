package ticketing

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const ghToken = "ghp_SECRETTOKEN123"

type fakeIssue struct {
	number        int
	title, body   string
	state, reason string
	labels        []string
}

// fakeGitHub is a stateful GitHub Issues API for one repo (acme/widgets).
type fakeGitHub struct {
	mu     sync.Mutex
	issues map[int]*fakeIssue
	nextID int
}

func (f *fakeGitHub) render(t *testing.T, i *fakeIssue) string {
	reason := "null"
	if i.reason != "" {
		reason = mustJSON(i.reason)
	}
	labels := make([]map[string]string, 0, len(i.labels))
	for _, l := range i.labels {
		labels = append(labels, map[string]string{"name": l})
	}
	return fixture(t, "github_issue.json", i.number, i.number, i.number, i.state, reason, i.title, mustJSON(i.body), mustJSON(labels))
}

func (f *fakeGitHub) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+ghToken {
				writeJSON(w, 401, `{"message":"Bad credentials"}`)
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			h(w, r)
		}
	}
	mux.HandleFunc("GET /search/issues", auth(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		var items []string
		for _, i := range f.issues {
			// emulate fuzzy search: match on the marker phrase from the query.
			if strings.Contains(q, "repo:acme/widgets") && strings.Contains(q, "is:issue") && strings.Contains(q, `"artex:key=`) &&
				strings.Contains(i.body, strings.TrimSuffix(strings.SplitN(q, `"artex:key=`, 2)[1], `" is:issue`)) {
				items = append(items, f.render(t, i))
			}
		}
		if len(items) == 0 {
			writeJSON(w, 200, fixture(t, "github_search_empty.json"))
			return
		}
		writeJSON(w, 200, `{"total_count":`+strconv.Itoa(len(items))+`,"items":[`+strings.Join(items, ",")+`]}`)
	}))
	mux.HandleFunc("POST /repos/acme/widgets/issues", auth(func(w http.ResponseWriter, r *http.Request) {
		b := readBody(r)
		f.nextID++
		i := &fakeIssue{number: f.nextID, title: b["title"].(string), body: b["body"].(string), state: "open", labels: strList(b["labels"])}
		f.issues[i.number] = i
		writeJSON(w, 201, f.render(t, i))
	}))
	mux.HandleFunc("GET /repos/acme/widgets/issues/{n}", auth(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.PathValue("n"))
		i, ok := f.issues[n]
		if !ok {
			writeJSON(w, 404, `{"message":"Not Found"}`)
			return
		}
		writeJSON(w, 200, f.render(t, i))
	}))
	mux.HandleFunc("PATCH /repos/acme/widgets/issues/{n}", auth(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.PathValue("n"))
		i, ok := f.issues[n]
		if !ok {
			writeJSON(w, 404, `{"message":"Not Found"}`)
			return
		}
		b := readBody(r)
		i.title, i.body, i.labels = b["title"].(string), b["body"].(string), strList(b["labels"])
		i.state = b["state"].(string)
		i.reason, _ = b["state_reason"].(string)
		if i.reason == "reopened" {
			i.reason = ""
		}
		writeJSON(w, 200, f.render(t, i))
	}))
	return mux
}

func (f *fakeGitHub) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.issues)
}

func ghCfg(base string) map[string]any {
	return map[string]any{"base_url": base, "repo": "acme/widgets", "token": ghToken, "labels": []any{"artex"}}
}

func TestGitHubValidate(t *testing.T) {
	tr, _ := Get(KindGitHub)
	cases := []struct {
		cfg  map[string]any
		want string
	}{
		{map[string]any{"token": "t"}, "repo"},
		{map[string]any{"repo": "nobody", "token": "t"}, "repo"},
		{map[string]any{"repo": "a/b/c", "token": "t"}, "repo"},
		{map[string]any{"repo": "a/b"}, "token"},
		{map[string]any{"repo": "a/b", "token": "t", "base_url": "ftp://x"}, "base_url"},
	}
	for _, c := range cases {
		if err := tr.Validate(c.cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Validate(%v) = %v, want error naming %q", c.cfg, err, c.want)
		}
	}
	if err := tr.Validate(map[string]any{"repo": "a/b", "token": "t"}); err != nil {
		t.Fatalf("default base_url config rejected: %v", err)
	}
}

func TestGitHubLifecycle(t *testing.T) {
	fake := &fakeGitHub{issues: map[int]*fakeIssue{}}
	srv, rec := newFake(t, fake.handler(t))
	tr, _ := Get(KindGitHub)
	ref := runLifecycle(t, tr, ghCfg(srv.URL), fake.count)
	if want := "https://github.com/acme/widgets/issues/" + ref.ID; ref.URL != want {
		t.Fatalf("ref.URL = %q, want %q", ref.URL, want)
	}

	// Request shape: headers, marker, labels.
	creates := rec.matching("POST", "/repos/acme/widgets/issues")
	if len(creates) < 1 {
		t.Fatal("no create recorded")
	}
	h := creates[0].Header
	if h.Get("Authorization") != "Bearer "+ghToken || h.Get("Accept") != "application/vnd.github+json" || h.Get("X-GitHub-Api-Version") != "2022-11-28" {
		t.Fatalf("headers = %v", h)
	}
	b := creates[0].json(t)
	key := DedupKey("eng-1", "finding-1")
	if !strings.Contains(b["body"].(string), "<!-- artex:key="+key+" -->") {
		t.Fatalf("body lacks the marker: %v", b["body"])
	}
	if got := strList(b["labels"]); !slices.Equal(got, []string{"artex", "security"}) {
		t.Fatalf("labels = %v", got)
	}
	search := rec.matching("GET", "/search/issues")[0]
	q, _ := url.ParseQuery(search.Query)
	if want := `repo:acme/widgets in:body "artex:key=` + key + `" is:issue`; q.Get("q") != want {
		t.Fatalf("search q = %q, want %q", q.Get("q"), want)
	}
}

func TestGitHubStateReasonsAndLabels(t *testing.T) {
	fake := &fakeGitHub{issues: map[int]*fakeIssue{}}
	srv, rec := newFake(t, fake.handler(t))
	tr, _ := Get(KindGitHub)
	cfg, ctx := ghCfg(srv.URL), context.Background()
	tk := Ticket{Key: DedupKey("k"), Title: "t", Body: "b", Severity: SevLow, Open: true}
	ref, _, err := Push(ctx, tr, cfg, nil, tk)
	if err != nil {
		t.Fatal(err)
	}
	// A human adds a label; a re-push must not drop it.
	fake.issues[1].labels = append(fake.issues[1].labels, "triaged")

	want := map[Resolution][2]string{
		ResFixed: {"completed", ""}, ResWontFix: {"not_planned", ""}, ResDuplicate: {"duplicate", ""},
		ResFalsePositive: {"not_planned", labelFalsePositive}, ResRiskAccepted: {"not_planned", labelRiskAccepted},
	}
	for res, w := range want {
		tk.Open, tk.Resolution = false, res
		if err := tr.Update(ctx, cfg, ref, tk); err != nil {
			t.Fatal(err)
		}
		patches := rec.matching("PATCH", "/repos/acme/widgets/issues/1")
		last := patches[len(patches)-1].json(t)
		if last["state"] != "closed" || last["state_reason"] != w[0] {
			t.Fatalf("%s: patch = %v", res, last)
		}
		labels := strList(last["labels"])
		if !slices.Contains(labels, "triaged") || (w[1] != "" && !slices.Contains(labels, w[1])) {
			t.Fatalf("%s: labels = %v", res, labels)
		}
		if w[1] == "" && (slices.Contains(labels, labelFalsePositive) || slices.Contains(labels, labelRiskAccepted)) {
			t.Fatalf("%s: stale refinement label kept: %v", res, labels)
		}
	}
	// Reopen sends state_reason "reopened" and clears the refinement labels.
	tk.Open, tk.Resolution = true, ResNone
	if err := tr.Update(ctx, cfg, ref, tk); err != nil {
		t.Fatal(err)
	}
	patches := rec.matching("PATCH", "/repos/acme/widgets/issues/1")
	last := patches[len(patches)-1].json(t)
	if last["state"] != "open" || last["state_reason"] != "reopened" || slices.Contains(strList(last["labels"]), labelRiskAccepted) {
		t.Fatalf("reopen patch = %v", last)
	}

	// A close done by hand in GitHub with no reason reads as fixed.
	fake.issues[1].state, fake.issues[1].reason, fake.issues[1].labels = "closed", "", nil
	if st, err := tr.State(ctx, cfg, ref); err != nil || st.Open || st.Resolution != ResFixed {
		t.Fatalf("state = %+v, %v", st, err)
	}
}

func TestGitHubFindIgnoresFuzzyHits(t *testing.T) {
	srv, _ := newFake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"items":[{"number":9,"html_url":"https://github.com/acme/widgets/issues/9","body":"mentions artex:key=other only"}]}`)
	}))
	tr, _ := Get(KindGitHub)
	if ref, err := tr.Find(context.Background(), ghCfg(srv.URL), DedupKey("x")); err != nil || ref != nil {
		t.Fatalf("Find = %+v, %v; want nil, nil", ref, err)
	}
}

func TestGitHubErrorsDoNotLeakToken(t *testing.T) {
	tr, _ := Get(KindGitHub)
	runErrorChecks(t, tr, ghCfg, ghToken)
}
