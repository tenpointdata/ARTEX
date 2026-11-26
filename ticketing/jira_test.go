package ticketing

import (
	"context"
	"encoding/base64"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const jiraToken = "ATATT-SECRET-JIRA-TOKEN"

type fakeJiraIssue struct {
	key, summary, desc string
	labels             []string
	category           string // new | indeterminate | done
	resolution         string // "" = unresolved
}

// fakeJira is a stateful Jira REST v2 server for project SEC.
type fakeJira struct {
	mu         sync.Mutex
	issues     map[string]*fakeJiraIssue
	nextID     int
	searchPath string // path serving search; "" = /rest/api/2/search
	wantAuth   string
}

func (f *fakeJira) render(t *testing.T, i *fakeJiraIssue) string {
	res := "null"
	if i.resolution != "" {
		res = `{"name":` + mustJSON(i.resolution) + `}`
	}
	names := map[string]string{"new": "To Do", "indeterminate": "In Progress", "done": "Done"}
	return fixture(t, "jira_issue.json", i.key, mustJSON(i.summary), mustJSON(i.labels), names[i.category], i.category, names[i.category], res)
}

func (f *fakeJira) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != f.wantAuth {
				writeJSON(w, 401, `{"errorMessages":["You do not have the permission"]}`)
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			h(w, r)
		}
	}
	search := auth(func(w http.ResponseWriter, r *http.Request) {
		jql, _ := readBody(r)["jql"].(string)
		for _, i := range f.issues {
			for _, l := range i.labels {
				if jql == `labels = "`+l+`"` {
					writeJSON(w, 200, `{"issues":[{"key":"`+i.key+`","fields":{"status":{"name":"x"}}}]}`)
					return
				}
			}
		}
		writeJSON(w, 200, `{"issues":[]}`)
	})
	sp := f.searchPath
	if sp == "" {
		sp = "/rest/api/2/search"
	}
	mux.HandleFunc("POST "+sp, search)
	mux.HandleFunc("POST /rest/api/2/issue", auth(func(w http.ResponseWriter, r *http.Request) {
		fields, _ := readBody(r)["fields"].(map[string]any)
		if fields["project"].(map[string]any)["key"] != "SEC" || fields["issuetype"].(map[string]any)["name"] == "" {
			writeJSON(w, 400, `{"errors":{"project":"bad"}}`)
			return
		}
		f.nextID++
		i := &fakeJiraIssue{key: "SEC-" + strconv.Itoa(f.nextID), summary: fields["summary"].(string), desc: fields["description"].(string),
			labels: strList(fields["labels"]), category: "new"}
		f.issues[i.key] = i
		writeJSON(w, 201, `{"id":"10042","key":"`+i.key+`","self":"x"}`)
	}))
	mux.HandleFunc("GET /rest/api/2/issue/{key}", auth(func(w http.ResponseWriter, r *http.Request) {
		i, ok := f.issues[r.PathValue("key")]
		if !ok {
			writeJSON(w, 404, `{"errorMessages":["Issue does not exist"]}`)
			return
		}
		writeJSON(w, 200, f.render(t, i))
	}))
	mux.HandleFunc("PUT /rest/api/2/issue/{key}", auth(func(w http.ResponseWriter, r *http.Request) {
		i, ok := f.issues[r.PathValue("key")]
		if !ok {
			writeJSON(w, 404, `{}`)
			return
		}
		b := readBody(r)
		fields, _ := b["fields"].(map[string]any)
		i.summary, i.desc = fields["summary"].(string), fields["description"].(string)
		ops, _ := b["update"].(map[string]any)["labels"].([]any)
		for _, op := range ops {
			m := op.(map[string]any)
			if l, ok := m["add"].(string); ok && !slices.Contains(i.labels, l) {
				i.labels = append(i.labels, l)
			}
			if l, ok := m["remove"].(string); ok {
				i.labels = slices.DeleteFunc(i.labels, func(x string) bool { return x == l })
			}
		}
		w.WriteHeader(204)
	}))
	mux.HandleFunc("GET /rest/api/2/issue/{key}/transitions", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, fixture(t, "jira_transitions.json"))
	}))
	mux.HandleFunc("POST /rest/api/2/issue/{key}/transitions", auth(func(w http.ResponseWriter, r *http.Request) {
		i := f.issues[r.PathValue("key")]
		id := readBody(r)["transition"].(map[string]any)["id"]
		switch id {
		case "21", "22":
			i.category, i.resolution = "done", "Done"
		case "31":
			i.category, i.resolution = "new", ""
		case "11":
			i.category = "indeterminate"
		default:
			writeJSON(w, 400, `{"errorMessages":["bad transition"]}`)
			return
		}
		w.WriteHeader(204)
	}))
	return mux
}

func (f *fakeJira) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.issues)
}

func basicAuth(email, token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token))
}

func jiraCfg(base string) map[string]any {
	return map[string]any{"base_url": base, "project_key": "SEC", "email": "bot@acme.test", "token": jiraToken}
}

func TestJiraValidate(t *testing.T) {
	tr, _ := Get(KindJira)
	cases := []struct {
		cfg  map[string]any
		want string
	}{
		{map[string]any{"project_key": "SEC", "token": "t"}, "base_url"},
		{map[string]any{"base_url": "https://j.test", "token": "t"}, "project_key"},
		{map[string]any{"base_url": "https://j.test", "project_key": "SEC"}, "token"},
		{map[string]any{"base_url": "javascript:alert(1)", "project_key": "SEC", "token": "t"}, "base_url"},
	}
	for _, c := range cases {
		if err := tr.Validate(c.cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Validate(%v) = %v, want error naming %q", c.cfg, err, c.want)
		}
	}
	if err := tr.Validate(map[string]any{"base_url": "https://j.test", "project_key": "SEC", "token": "t"}); err != nil {
		t.Fatal(err)
	}
}

func TestJiraLifecycleBasicAuth(t *testing.T) {
	fake := &fakeJira{issues: map[string]*fakeJiraIssue{}, wantAuth: basicAuth("bot@acme.test", jiraToken)}
	srv, rec := newFake(t, fake.handler(t))
	tr, _ := Get(KindJira)
	ref := runLifecycle(t, tr, jiraCfg(srv.URL), fake.count)
	if ref.ID != "SEC-1" || ref.URL != srv.URL+"/browse/SEC-1" {
		t.Fatalf("ref = %+v", ref)
	}
	create := rec.matching("POST", "/rest/api/2/issue")[0].json(t)
	fields := create["fields"].(map[string]any)
	if fields["issuetype"].(map[string]any)["name"] != "Bug" {
		t.Fatalf("default issue type not Bug: %v", fields["issuetype"])
	}
	if got := strList(fields["labels"]); !slices.Equal(got, []string{"artex", DedupKey("eng-1", "finding-1"), "security"}) {
		t.Fatalf("labels = %v", got)
	}
	// Reopen must not have left a resolution label behind.
	for _, l := range fake.issues["SEC-1"].labels {
		if strings.HasPrefix(l, jiraResLabelPrefix) {
			t.Fatalf("stale resolution label %q after reopen", l)
		}
	}
}

func TestJiraBearerPATAndCustomTransitions(t *testing.T) {
	fake := &fakeJira{issues: map[string]*fakeJiraIssue{}, wantAuth: "Bearer " + jiraToken}
	srv, rec := newFake(t, fake.handler(t))
	tr, _ := Get(KindJira)
	cfg := map[string]any{"base_url": srv.URL + "/", "project_key": "SEC", "token": jiraToken, "issue_type": "Task", "done_transition": "resolve issue", "reopen_transition": "Reopen"}
	ctx := context.Background()
	tk := Ticket{Key: DedupKey("a"), Title: "t", Body: "b", Severity: SevLow, Open: true}
	ref, _, err := Push(ctx, tr, cfg, nil, tk)
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.matching("POST", "/rest/api/2/issue")[0].json(t)["fields"].(map[string]any)["issuetype"].(map[string]any)["name"]; got != "Task" {
		t.Fatalf("issue_type = %v", got)
	}
	tk.Open, tk.Resolution = false, ResWontFix
	if err := tr.Update(ctx, cfg, ref, tk); err != nil {
		t.Fatal(err)
	}
	posts := rec.matching("POST", "/rest/api/2/issue/SEC-1/transitions")
	if id := posts[0].json(t)["transition"].(map[string]any)["id"]; id != "22" {
		t.Fatalf("done_transition by name picked id %v, want 22", id)
	}
	// Idempotent: already done, so no second transition.
	if err := tr.Update(ctx, cfg, ref, tk); err != nil || len(rec.matching("POST", "/rest/api/2/issue/SEC-1/transitions")) != 1 {
		t.Fatalf("second close should not transition again: %v", err)
	}
	// A done issue with no artex label falls back to Jira's own resolution name.
	fake.issues["SEC-1"].labels = []string{"artex"}
	fake.issues["SEC-1"].resolution = "Won't Do"
	if st, _ := tr.State(ctx, cfg, ref); st.Open || st.Resolution != ResWontFix {
		t.Fatalf("state from resolution name = %+v", st)
	}
	fake.issues["SEC-1"].resolution = "Duplicate"
	if st, _ := tr.State(ctx, cfg, ref); st.Resolution != ResDuplicate {
		t.Fatalf("state = %+v", st)
	}
}

func TestJiraMissingTransitionIsExplained(t *testing.T) {
	fake := &fakeJira{issues: map[string]*fakeJiraIssue{}, wantAuth: basicAuth("bot@acme.test", jiraToken)}
	srv, _ := newFake(t, fake.handler(t))
	tr, _ := Get(KindJira)
	cfg := jiraCfg(srv.URL)
	cfg["done_transition"] = "Does Not Exist"
	ref, _, err := Push(context.Background(), tr, cfg, nil, Ticket{Key: DedupKey("a"), Title: "t", Open: true})
	if err != nil {
		t.Fatal(err)
	}
	err = tr.Update(context.Background(), cfg, ref, Ticket{Key: DedupKey("a"), Title: "t", Resolution: ResFixed})
	if err == nil || !strings.Contains(err.Error(), "done_transition") || strings.Contains(err.Error(), jiraToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestJiraSearchFallsBackToSearchJQL(t *testing.T) {
	fake := &fakeJira{issues: map[string]*fakeJiraIssue{}, wantAuth: basicAuth("bot@acme.test", jiraToken), searchPath: "/rest/api/2/search/jql"}
	srv, rec := newFake(t, fake.handler(t))
	tr, _ := Get(KindJira)
	cfg := jiraCfg(srv.URL)
	tk := Ticket{Key: DedupKey("a"), Title: "t", Body: "b", Open: true}
	if _, created, err := Push(context.Background(), tr, cfg, nil, tk); err != nil || !created {
		t.Fatal(created, err)
	}
	if _, created, err := Push(context.Background(), tr, cfg, nil, tk); err != nil || created {
		t.Fatalf("second push created=%v err=%v", created, err)
	}
	if len(rec.matching("POST", "/rest/api/2/search/jql")) == 0 {
		t.Fatal("fallback endpoint never used")
	}
}

func TestJiraErrorsDoNotLeakToken(t *testing.T) {
	tr, _ := Get(KindJira)
	runErrorChecks(t, tr, jiraCfg, jiraToken)
}
