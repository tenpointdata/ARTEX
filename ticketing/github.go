package ticketing

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/notify"
)

// GitHub Issues adapter.
//
// Config: base_url (default https://api.github.com; set for GitHub Enterprise),
// repo "owner/name", token (secret), labels (optional, added to every issue).
//
// Identity: the issue body carries a hidden `<!-- artex:key=<Key> -->` marker;
// Find searches for it. Note GitHub's search index lags a few seconds behind
// writes, so a push immediately after a create (without a stored Ref) may not
// find the issue yet; callers should persist the returned Ref.
//
// Resolution: GitHub only models completed / not_planned / duplicate, so the
// finer false_positive and risk_accepted resolutions are carried by the labels
// artex:false-positive and artex:risk-accepted on the closed issue.

const (
	defaultGitHubAPI    = "https://api.github.com"
	labelFalsePositive  = "artex:false-positive"
	labelRiskAccepted   = "artex:risk-accepted"
	githubManagedPrefix = "artex:"
)

var githubRepoRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type githubTracker struct{}

func (githubTracker) Kind() string              { return KindGitHub }
func (githubTracker) SecretKeys() []string      { return []string{"token", WebhookSecretKey} }
func (githubTracker) DestinationKeys() []string { return []string{"base_url", "repo"} }

func (githubTracker) Validate(cfg map[string]any) error {
	if _, err := baseURL(cfg, defaultGitHubAPI); err != nil {
		return err
	}
	repo := cfgString(cfg, "repo")
	if repo == "" {
		return fmt.Errorf("repo is required (owner/name)")
	}
	if !githubRepoRE.MatchString(repo) {
		return fmt.Errorf("repo must look like owner/name")
	}
	if cfgString(cfg, "token") == "" {
		return fmt.Errorf("token is required")
	}
	return validateWebhookSecret(cfg)
}

type githubIssue struct {
	Number      int    `json:"number"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	State       string `json:"state"`
	StateReason string `json:"state_reason"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (i githubIssue) ref() Ref {
	return Ref{ID: strconv.Itoa(i.Number), URL: i.HTMLURL}
}

func (i githubIssue) labelNames() []string {
	out := make([]string, 0, len(i.Labels))
	for _, l := range i.Labels {
		out = append(out, l.Name)
	}
	return out
}

type githubTarget struct {
	api, repo string
	headers   map[string]string
}

func githubOf(cfg map[string]any) (githubTarget, error) {
	if err := (githubTracker{}).Validate(cfg); err != nil {
		return githubTarget{}, notify.Permanent(err)
	}
	api, _ := baseURL(cfg, defaultGitHubAPI)
	return githubTarget{
		api:  api,
		repo: cfgString(cfg, "repo"),
		headers: map[string]string{
			"Authorization":        "Bearer " + cfgString(cfg, "token"),
			"Accept":               "application/vnd.github+json",
			"X-GitHub-Api-Version": "2022-11-28",
		},
	}, nil
}

func githubMarker(key string) string { return "artex:key=" + key }

func (g githubTarget) issueURL(id string) string {
	return g.api + "/repos/" + g.repo + "/issues/" + url.PathEscape(id)
}

func (githubTracker) Find(ctx context.Context, cfg map[string]any, key string) (*Ref, error) {
	g, err := githubOf(cfg)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`repo:%s in:body "%s" is:issue`, g.repo, githubMarker(key))
	res, err := callJSON[struct {
		Items []githubIssue `json:"items"`
	}](ctx, notify.Request{
		Method: http.MethodGet, URL: g.api + "/search/issues?per_page=5&q=" + url.QueryEscape(q), Headers: g.headers,
	})
	if err != nil {
		return nil, fmt.Errorf("github: search issues: %w", err)
	}
	for _, it := range res.Items {
		// The search is fuzzy; only accept an issue whose body really carries the marker.
		if strings.Contains(it.Body, "<!-- "+githubMarker(key)+" -->") {
			r := it.ref()
			return &r, nil
		}
	}
	return nil, nil
}

// githubLabels is the label set we want on the issue: configured + ticket labels
// + the resolution refinement label when the ticket is closed.
func githubLabels(cfg map[string]any, t Ticket) []string {
	want := uniqueStrings(append(cfgStrings(cfg, "labels"), t.Labels...)...)
	if !t.Open {
		switch closedResolution(t) {
		case ResFalsePositive:
			want = uniqueStrings(append(want, labelFalsePositive)...)
		case ResRiskAccepted:
			want = uniqueStrings(append(want, labelRiskAccepted)...)
		}
	}
	return want
}

func githubBody(t Ticket) string {
	return strings.TrimRight(t.Body, "\n") + "\n\n<!-- " + githubMarker(t.Key) + " -->"
}

// githubClose maps a resolution to GitHub's state_reason.
func githubClose(r Resolution) string {
	switch r {
	case ResFixed:
		return "completed"
	case ResDuplicate:
		return "duplicate"
	default: // wont_fix, false_positive, risk_accepted
		return "not_planned"
	}
}

func (githubTracker) Create(ctx context.Context, cfg map[string]any, t Ticket) (Ref, error) {
	g, err := githubOf(cfg)
	if err != nil {
		return Ref{}, err
	}
	issue, err := callJSON[githubIssue](ctx, notify.Request{
		Method: http.MethodPost, URL: g.api + "/repos/" + g.repo + "/issues", Headers: g.headers,
		Body: map[string]any{"title": t.Title, "body": githubBody(t), "labels": githubLabels(cfg, t)},
	})
	if err != nil {
		return Ref{}, fmt.Errorf("github: create issue: %w", err)
	}
	ref := issue.ref()
	if !t.Open {
		// The create endpoint cannot close an issue; do it in a second call.
		if err := (githubTracker{}).Update(ctx, cfg, ref, t); err != nil {
			return ref, err
		}
	}
	return ref, nil
}

func (githubTracker) Update(ctx context.Context, cfg map[string]any, ref Ref, t Ticket) error {
	g, err := githubOf(cfg)
	if err != nil {
		return err
	}
	cur, err := callJSON[githubIssue](ctx, notify.Request{Method: http.MethodGet, URL: g.issueURL(ref.ID), Headers: g.headers})
	if err != nil {
		return fmt.Errorf("github: read issue: %w", err)
	}
	// PATCH replaces the whole label set, so keep labels humans added and only
	// drop artex:-prefixed ones that are no longer wanted.
	want := githubLabels(cfg, t)
	labels := append([]string(nil), want...)
	for _, name := range cur.labelNames() {
		if !strings.HasPrefix(name, githubManagedPrefix) && !hasString(labels, name) {
			labels = append(labels, name)
		}
	}
	patch := map[string]any{"title": t.Title, "body": githubBody(t), "labels": labels}
	switch {
	case t.Open && cur.State == "closed":
		patch["state"] = "open"
		patch["state_reason"] = "reopened"
	case t.Open:
		patch["state"] = "open"
	default:
		patch["state"] = "closed"
		patch["state_reason"] = githubClose(closedResolution(t))
	}
	if _, err := callJSON[githubIssue](ctx, notify.Request{
		Method: http.MethodPatch, URL: g.issueURL(ref.ID), Headers: g.headers, Body: patch,
	}); err != nil {
		return fmt.Errorf("github: update issue: %w", err)
	}
	return nil
}

func (githubTracker) State(ctx context.Context, cfg map[string]any, ref Ref) (State, error) {
	g, err := githubOf(cfg)
	if err != nil {
		return State{}, err
	}
	cur, err := callJSON[githubIssue](ctx, notify.Request{Method: http.MethodGet, URL: g.issueURL(ref.ID), Headers: g.headers})
	if err != nil {
		return State{}, fmt.Errorf("github: read issue: %w", err)
	}
	if cur.State != "closed" {
		return State{Open: true}, nil
	}
	labels := cur.labelNames()
	switch {
	case hasString(labels, labelFalsePositive):
		return State{Resolution: ResFalsePositive}, nil
	case hasString(labels, labelRiskAccepted):
		return State{Resolution: ResRiskAccepted}, nil
	}
	switch cur.StateReason {
	case "not_planned":
		return State{Resolution: ResWontFix}, nil
	case "duplicate":
		return State{Resolution: ResDuplicate}, nil
	default: // completed, or no reason recorded (GitHub's default close)
		return State{Resolution: ResFixed}, nil
	}
}
