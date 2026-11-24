package ticketing

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Autumn-27/artex/notify"
)

// Jira adapter (REST API v2, Server/Data Center and Cloud).
//
// Config: base_url, project_key, issue_type (default "Bug"), token (secret) with
// an optional email (basic auth: email + API token; without email the token is a
// Bearer PAT), done_transition / reopen_transition (optional transition names
// used instead of picking by status category).
//
// Identity: every issue gets the labels "artex" and <Key>; Find searches
// `labels = "<Key>"`. Resolution is carried by an `artex-res-<resolution>` label
// (set on close, removed on reopen) because Jira's own resolution field cannot
// be set portably (it depends on the workflow's transition screen). Labels are
// edited with add/remove operations so labels people added are preserved.
//
// The description is sent as the Markdown text unchanged (REST v2 takes wiki
// markup, so Markdown renders as plain text there).

const (
	defaultJiraIssueType = "Bug"
	jiraResLabelPrefix   = "artex-res-"
)

type jiraTracker struct{}

func (jiraTracker) Kind() string              { return KindJira }
func (jiraTracker) SecretKeys() []string      { return []string{"token"} }
func (jiraTracker) DestinationKeys() []string { return []string{"base_url"} }

func (jiraTracker) Validate(cfg map[string]any) error {
	if _, err := baseURL(cfg, ""); err != nil {
		return err
	}
	if cfgString(cfg, "project_key") == "" {
		return fmt.Errorf("project_key is required")
	}
	if cfgString(cfg, "token") == "" {
		return fmt.Errorf("token is required")
	}
	return nil
}

type jiraTarget struct {
	base    string
	project string
	itype   string
	done    string
	reopen  string
	headers map[string]string
}

func jiraOf(cfg map[string]any) (jiraTarget, error) {
	if err := (jiraTracker{}).Validate(cfg); err != nil {
		return jiraTarget{}, notify.Permanent(err)
	}
	base, _ := baseURL(cfg, "")
	token := cfgString(cfg, "token")
	auth := "Bearer " + token
	if email := cfgString(cfg, "email"); email != "" {
		auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token))
	}
	itype := cfgString(cfg, "issue_type")
	if itype == "" {
		itype = defaultJiraIssueType
	}
	return jiraTarget{
		base: base, project: cfgString(cfg, "project_key"), itype: itype,
		done: cfgString(cfg, "done_transition"), reopen: cfgString(cfg, "reopen_transition"),
		headers: map[string]string{"Authorization": auth, "Accept": "application/json"},
	}, nil
}

type jiraIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Labels []string `json:"labels"`
		Status struct {
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		Resolution *struct {
			Name string `json:"name"`
		} `json:"resolution"`
	} `json:"fields"`
}

func (i jiraIssue) done() bool { return i.Fields.Status.StatusCategory.Key == "done" }

func (j jiraTarget) ref(key string) Ref {
	return Ref{ID: key, URL: j.base + "/browse/" + key}
}

func (j jiraTarget) issueURL(key string) string {
	return j.base + "/rest/api/2/issue/" + url.PathEscape(key)
}

func (j jiraTarget) get(ctx context.Context, key string) (jiraIssue, error) {
	i, err := callJSON[jiraIssue](ctx, notify.Request{
		Method: http.MethodGet, URL: j.issueURL(key) + "?fields=status,labels,resolution", Headers: j.headers,
	})
	if err != nil {
		return i, fmt.Errorf("jira: read issue: %w", err)
	}
	return i, nil
}

func (jiraTracker) Find(ctx context.Context, cfg map[string]any, key string) (*Ref, error) {
	j, err := jiraOf(cfg)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"jql": fmt.Sprintf(`labels = "%s"`, jqlEscape(key)), "maxResults": 1, "fields": []string{"status"}}
	type searchResult struct {
		Issues []jiraIssue `json:"issues"`
	}
	req := notify.Request{Method: http.MethodPost, URL: j.base + "/rest/api/2/search", Headers: j.headers, Body: body}
	resp, err := call(ctx, req)
	if err != nil && resp != nil && (resp.Status == http.StatusNotFound || resp.Status == http.StatusGone) {
		// Jira Cloud retired the classic search endpoint in favour of /search/jql.
		req.URL = j.base + "/rest/api/2/search/jql"
		resp, err = call(ctx, req)
	}
	if err != nil {
		return nil, fmt.Errorf("jira: search: %w", err)
	}
	res, err := decodeBody[searchResult](resp)
	if err != nil {
		return nil, fmt.Errorf("jira: search: %w", err)
	}
	if len(res.Issues) == 0 {
		return nil, nil
	}
	r := j.ref(res.Issues[0].Key)
	return &r, nil
}

func (jiraTracker) Create(ctx context.Context, cfg map[string]any, t Ticket) (Ref, error) {
	j, err := jiraOf(cfg)
	if err != nil {
		return Ref{}, err
	}
	labels := uniqueStrings(append([]string{"artex", t.Key}, jiraLabels(t.Labels)...)...)
	created, err := callJSON[jiraIssue](ctx, notify.Request{
		Method: http.MethodPost, URL: j.base + "/rest/api/2/issue", Headers: j.headers,
		Body: map[string]any{"fields": map[string]any{
			"project":     map[string]string{"key": j.project},
			"summary":     t.Title,
			"description": t.Body,
			"issuetype":   map[string]string{"name": j.itype},
			"labels":      labels,
		}},
	})
	if err != nil {
		return Ref{}, fmt.Errorf("jira: create issue: %w", err)
	}
	ref := j.ref(created.Key)
	if !t.Open {
		if err := (jiraTracker{}).Update(ctx, cfg, ref, t); err != nil {
			return ref, err
		}
	}
	return ref, nil
}

func (jiraTracker) Update(ctx context.Context, cfg map[string]any, ref Ref, t Ticket) error {
	j, err := jiraOf(cfg)
	if err != nil {
		return err
	}
	cur, err := j.get(ctx, ref.ID)
	if err != nil {
		return err
	}
	// Label edits as add/remove operations so labels added by people survive.
	wantRes := ""
	if !t.Open {
		wantRes = jiraResLabelPrefix + string(closedResolution(t))
	}
	var ops []map[string]string
	for _, l := range uniqueStrings(append([]string{"artex", t.Key, wantRes}, jiraLabels(t.Labels)...)...) {
		ops = append(ops, map[string]string{"add": l})
	}
	for _, l := range cur.Fields.Labels {
		if strings.HasPrefix(l, jiraResLabelPrefix) && l != wantRes {
			ops = append(ops, map[string]string{"remove": l})
		}
	}
	if _, err := call(ctx, notify.Request{
		Method: http.MethodPut, URL: j.issueURL(ref.ID), Headers: j.headers,
		Body: map[string]any{
			"fields": map[string]any{"summary": t.Title, "description": t.Body},
			"update": map[string]any{"labels": ops},
		},
	}); err != nil {
		return fmt.Errorf("jira: update issue: %w", err)
	}
	if cur.done() == !t.Open {
		return nil
	}
	return j.transition(ctx, ref.ID, !t.Open)
}

type jiraTransition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"to"`
}

// transition moves the issue to a done (toDone) or not-done status.
func (j jiraTarget) transition(ctx context.Context, key string, toDone bool) error {
	list, err := callJSON[struct {
		Transitions []jiraTransition `json:"transitions"`
	}](ctx, notify.Request{Method: http.MethodGet, URL: j.issueURL(key) + "/transitions", Headers: j.headers})
	if err != nil {
		return fmt.Errorf("jira: list transitions: %w", err)
	}
	named, field := j.reopen, "reopen_transition"
	if toDone {
		named, field = j.done, "done_transition"
	}
	pick := pickTransition(list.Transitions, named, toDone)
	if pick == nil {
		return notify.Permanent(fmt.Errorf("jira: no transition available to move %s; set %s to the workflow transition name", key, field))
	}
	if _, err := call(ctx, notify.Request{
		Method: http.MethodPost, URL: j.issueURL(key) + "/transitions", Headers: j.headers,
		Body: map[string]any{"transition": map[string]string{"id": pick.ID}},
	}); err != nil {
		return fmt.Errorf("jira: transition issue: %w", err)
	}
	return nil
}

// pickTransition prefers the configured name, then a status category match
// (for reopen, "new"/To Do over other open categories).
func pickTransition(list []jiraTransition, named string, toDone bool) *jiraTransition {
	if named != "" {
		for i := range list {
			if strings.EqualFold(list[i].Name, named) {
				return &list[i]
			}
		}
		return nil
	}
	var fallback *jiraTransition
	for i := range list {
		cat := list[i].To.StatusCategory.Key
		switch {
		case toDone && cat == "done":
			return &list[i]
		case !toDone && cat == "new":
			return &list[i]
		case !toDone && cat != "done" && fallback == nil:
			fallback = &list[i]
		}
	}
	return fallback
}

func (jiraTracker) State(ctx context.Context, cfg map[string]any, ref Ref) (State, error) {
	j, err := jiraOf(cfg)
	if err != nil {
		return State{}, err
	}
	cur, err := j.get(ctx, ref.ID)
	if err != nil {
		return State{}, err
	}
	if !cur.done() {
		return State{Open: true}, nil
	}
	for _, l := range cur.Fields.Labels {
		if strings.HasPrefix(l, jiraResLabelPrefix) {
			if r := Resolution(strings.TrimPrefix(l, jiraResLabelPrefix)); knownResolution(r) {
				return State{Resolution: r}, nil
			}
		}
	}
	if cur.Fields.Resolution != nil {
		switch strings.ToLower(cur.Fields.Resolution.Name) {
		case "won't do", "won't fix", "wont do", "wont fix":
			return State{Resolution: ResWontFix}, nil
		case "duplicate":
			return State{Resolution: ResDuplicate}, nil
		}
	}
	return State{Resolution: ResFixed}, nil
}

// jiraLabels rewrites labels for Jira, which forbids whitespace inside a label.
func jiraLabels(in []string) []string {
	out := make([]string, len(in))
	for i, l := range in {
		out[i] = strings.Join(strings.Fields(l), "-")
	}
	return out
}

// jqlEscape escapes a value for use inside a double-quoted JQL string.
func jqlEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

func knownResolution(r Resolution) bool {
	switch r {
	case ResFixed, ResFalsePositive, ResRiskAccepted, ResDuplicate, ResWontFix:
		return true
	}
	return false
}
