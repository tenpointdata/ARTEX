package server

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/notify"
	"github.com/Autumn-27/artex/ticketing"
)

// Finding ↔ ticket status round-trip (E2). The tracker adapters are package
// ticketing; this file maps ARTEX's disposition statuses onto an open/closed
// ticket with a resolution and back, and decides per sync which side changed
// against the state both agreed on last time (db.FindingTicket).
//
// Conflict rule: when both sides changed since the last sync, the tracker wins.
// Remediation is tracked there; the finding keeps its verification state
// either way, because a ticket status is a human disposition, never a proof.

// statusToTicket maps a finding status onto the ticket state to push.
func statusToTicket(status string) ticketing.State {
	switch status {
	case db.FindingResolved, db.FindingFixed:
		return ticketing.State{Open: false, Resolution: ticketing.ResFixed}
	case db.FindingFalsePositive:
		return ticketing.State{Open: false, Resolution: ticketing.ResFalsePositive}
	case db.FindingRiskAccepted:
		return ticketing.State{Open: false, Resolution: ticketing.ResRiskAccepted}
	case db.FindingDuplicate:
		return ticketing.State{Open: false, Resolution: ticketing.ResDuplicate}
	case db.FindingIgnored:
		return ticketing.State{Open: false, Resolution: ticketing.ResWontFix}
	}
	return ticketing.State{Open: true}
}

// ticketToStatus maps a ticket state read from the tracker onto the finding
// status, given the current one. An open ticket leaves an open finding as it
// is and reopens a closed one as confirmed.
func ticketToStatus(st ticketing.State, current string) string {
	if st.Open {
		if statusToTicket(current).Open {
			return current
		}
		return db.FindingConfirmed
	}
	if !statusToTicket(current).Open && statusToTicket(current).Resolution == st.Resolution {
		return current // resolved vs fixed both read back as fixed: keep the finer one
	}
	switch st.Resolution {
	case ticketing.ResFalsePositive:
		return db.FindingFalsePositive
	case ticketing.ResRiskAccepted:
		return db.FindingRiskAccepted
	case ticketing.ResDuplicate:
		return db.FindingDuplicate
	case ticketing.ResWontFix:
		return db.FindingIgnored
	case ticketing.ResFixed:
		return db.FindingFixed
	}
	return db.FindingResolved
}

// findingDedupKey is the stable key a finding's ticket carries. An imported
// finding reuses its import key, so the same scanner result maps to the same
// ticket across ARTEX instances; any other finding is keyed on what it is
// (class, name, assets), not on its row id.
func findingDedupKey(f *db.DBFinding, importKey string) string {
	if importKey != "" {
		return ticketing.DedupKey("import", importKey)
	}
	ids := append([]int64(nil), f.AssetIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return ticketing.DedupKey("finding", f.VulnClass, f.Name, strings.Join(parts, ","))
}

// ticketEvidenceMax caps the evidence copied into a ticket body.
const ticketEvidenceMax = 8 << 10

// sensitiveHeaderRe matches credential-bearing header lines in captured evidence.
var sensitiveHeaderRe = regexp.MustCompile(`(?im)^(\s*(?:authorization|proxy-authorization|cookie|set-cookie|x-api-key|x-auth-token|api-key)\s*:\s*).*$`)

// redactEvidence blanks credential headers and caps the text.
func redactEvidence(s string) string {
	return notify.TruncateBytes(sensitiveHeaderRe.ReplaceAllString(s, "${1}[redacted]"), ticketEvidenceMax)
}

// ticketOptions are the server-side keys of a tracker config, next to the
// adapter's own keys.
type ticketOptions struct {
	IncludeEvidence bool
	BaseURL         string // ARTEX public base URL for the link back, "" for none
}

func trackerOptions(cfg map[string]any, baseURL string) ticketOptions {
	inc, _ := cfg["include_evidence"].(bool)
	return ticketOptions{IncludeEvidence: inc, BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/")}
}

// buildTicket renders a finding as a ticket. Evidence is opt-in per tracker
// (include_evidence) because captured traffic can carry session material; when
// included, credential headers are redacted.
func buildTicket(f *db.DBFinding, key string, opt ticketOptions) ticketing.Ticket {
	title := f.Name
	if title == "" {
		title = f.VulnClass
	}
	sev := f.Severity
	if !db.ValidSeverity(sev) {
		sev = db.SeverityLow
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**Severity:** %s  \n**Class:** %s  \n**ARTEX status:** %s  \n**Verification:** %s",
		sev, f.VulnClass, f.Status, f.VerificationStatus)
	if f.PoELevel > 0 {
		fmt.Fprintf(&b, " (POE %d)", f.PoELevel)
	}
	b.WriteString("\n\n")
	if opt.BaseURL != "" {
		fmt.Fprintf(&b, "[View in ARTEX](%s/findings/%d)\n\n", opt.BaseURL, f.ID)
	}
	b.WriteString("## Summary\n\n" + f.Summary + "\n")
	if opt.IncludeEvidence && strings.TrimSpace(f.Evidence) != "" {
		b.WriteString("\n## Evidence\n\n```\n" + strings.ReplaceAll(redactEvidence(f.Evidence), "```", "'''") + "\n```\n")
	}
	st := statusToTicket(f.Status)
	return ticketing.Ticket{
		Key: key, Title: "[ARTEX] " + notify.OneLine(title, 200), Body: b.String(), Severity: sev,
		Labels: []string{"artex", "severity:" + sev}, Open: st.Open, Resolution: st.Resolution,
		Verified: f.VerificationStatus == db.VerificationVerified,
	}
}

// ticketSyncer pushes and syncs the findings of one tracker.
type ticketSyncer struct {
	pg      *db.DB
	tracker *db.TicketTracker
	impl    ticketing.Tracker
	cfg     map[string]any
	opt     ticketOptions
}

func newTicketSyncer(pg *db.DB, t *db.TicketTracker, baseURL string) (*ticketSyncer, error) {
	impl, ok := ticketing.Get(t.Kind)
	if !ok {
		return nil, fmt.Errorf("tracker kind %q is not supported", t.Kind)
	}
	cfg := map[string]any{}
	if len(t.Config) > 0 {
		if err := json.Unmarshal(t.Config, &cfg); err != nil {
			return nil, fmt.Errorf("tracker config is not valid JSON")
		}
	}
	if err := impl.Validate(cfg); err != nil {
		return nil, err
	}
	return &ticketSyncer{pg: pg, tracker: t, impl: impl, cfg: cfg, opt: trackerOptions(cfg, baseURL)}, nil
}

func (s *ticketSyncer) key(ctx context.Context, f *db.DBFinding) (string, error) {
	src, err := s.pg.FindingImportSourceByFinding(ctx, f.ID)
	if err != nil {
		return "", err
	}
	imp := ""
	if src != nil {
		imp = src.DedupKey
	}
	return findingDedupKey(f, imp), nil
}

// push creates or updates the finding's ticket and records the agreed state.
func (s *ticketSyncer) push(ctx context.Context, f *db.DBFinding) (*db.FindingTicket, bool, error) {
	link, err := s.pg.FindingTicket(ctx, s.tracker.ID, f.ID)
	if err != nil {
		return nil, false, err
	}
	key := ""
	var existing *ticketing.Ref
	if link != nil {
		key, existing = link.DedupKey, &ticketing.Ref{ID: link.ExternalID, URL: link.ExternalURL}
	} else if key, err = s.key(ctx, f); err != nil {
		return nil, false, err
	}
	t := buildTicket(f, key, s.opt)
	ref, created, err := ticketing.Push(ctx, s.impl, s.cfg, existing, t)
	if err != nil {
		if link != nil {
			_ = s.pg.SetFindingTicketError(ctx, link.ID, err.Error())
		}
		return nil, false, err
	}
	out := &db.FindingTicket{FindingID: f.ID, TrackerID: s.tracker.ID, DedupKey: key, ExternalID: ref.ID, ExternalURL: ref.URL,
		SyncedStatus: f.Status, RemoteOpen: t.Open, RemoteResolution: string(t.Resolution)}
	return out, created, s.pg.UpsertFindingTicket(ctx, out)
}

// ticketSyncResult says what one sync did for one link.
type ticketSyncResult struct {
	FindingID  int64  `json:"finding_id"`
	ExternalID string `json:"external_id"`
	Action     string `json:"action"` // unchanged | pulled | pushed | error
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	Error      string `json:"error,omitempty"`
}

// sync reconciles one link: a remote change is applied to the finding (the
// tracker wins a conflict), otherwise a local change is pushed.
func (s *ticketSyncer) sync(ctx context.Context, link *db.FindingTicket) ticketSyncResult {
	res := ticketSyncResult{FindingID: link.FindingID, ExternalID: link.ExternalID}
	fail := func(err error) ticketSyncResult {
		_ = s.pg.SetFindingTicketError(ctx, link.ID, err.Error())
		res.Action, res.Error = "error", err.Error()
		return res
	}
	f, err := s.pg.GetFinding(link.FindingID)
	if err != nil || f == nil {
		return fail(fmt.Errorf("finding %d: not found", link.FindingID))
	}
	remote, err := s.impl.State(ctx, s.cfg, ticketing.Ref{ID: link.ExternalID, URL: link.ExternalURL})
	if err != nil {
		return fail(err)
	}
	remoteChanged := remote.Open != link.RemoteOpen || string(remote.Resolution) != link.RemoteResolution
	localChanged := f.Status != link.SyncedStatus
	switch {
	case remoteChanged:
		to := ticketToStatus(remote, f.Status)
		if to != f.Status {
			if _, found, _, err := s.pg.SetFindingStatusWithNotify(ctx, f.ID, to); err != nil || !found {
				return fail(fmt.Errorf("set finding status: %v", err))
			}
		}
		res.Action, res.From, res.To = "pulled", f.Status, to
		link.SyncedStatus, link.RemoteOpen, link.RemoteResolution = to, remote.Open, string(remote.Resolution)
	case localChanged:
		t := buildTicket(f, link.DedupKey, s.opt)
		if err := s.impl.Update(ctx, s.cfg, ticketing.Ref{ID: link.ExternalID, URL: link.ExternalURL}, t); err != nil {
			return fail(err)
		}
		res.Action, res.From, res.To = "pushed", link.SyncedStatus, f.Status
		link.SyncedStatus, link.RemoteOpen, link.RemoteResolution = f.Status, t.Open, string(t.Resolution)
	default:
		res.Action = "unchanged"
	}
	if err := s.pg.UpsertFindingTicket(ctx, link); err != nil {
		return fail(err)
	}
	return res
}
