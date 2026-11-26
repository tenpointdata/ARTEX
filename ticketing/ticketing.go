// Package ticketing pushes findings to external issue trackers (GitHub Issues,
// Jira, DefectDojo) and reads their status back.
//
// It is a general mechanism: no database or server imports. Trackers are
// stateless adapters configured by a map[string]any (the JSON config column),
// every HTTP call goes through notify.Do (SSRF-guarded, redacted errors), and
// credentials are declared via SecretKeys/DestinationKeys so callers can reuse
// notify.MaskConfigKeys / notify.PrepareConfigUpdateKeys.
//
// Idempotency is built on a stable dedup key (DedupKey) that each adapter stores
// in a tracker-native searchable place, so Push can re-find a ticket it created
// earlier even when the caller lost the tracker id.
package ticketing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Resolution is why a ticket is closed; "" while open.
type Resolution string

const (
	ResNone          Resolution = ""
	ResFixed         Resolution = "fixed"
	ResFalsePositive Resolution = "false_positive"
	ResRiskAccepted  Resolution = "risk_accepted"
	ResDuplicate     Resolution = "duplicate"
	ResWontFix       Resolution = "wont_fix"
)

// Severity values accepted in Ticket.Severity.
const (
	SevCritical = "critical"
	SevHigh     = "high"
	SevMedium   = "medium"
	SevLow      = "low"
)

// Ticket is the tracker-neutral description of one finding to publish.
type Ticket struct {
	Key         string // stable dedup key, see DedupKey
	Title, Body string // Body is Markdown
	Severity    string // critical|high|medium|low
	Labels      []string
	// Open and Resolution are the desired state. A closed ticket (Open=false)
	// should carry a Resolution; an empty one is treated as ResFixed.
	Open       bool
	Resolution Resolution
	// Verified marks the finding as machine-verified (DefectDojo's "verified"
	// field). It is never inferred by an adapter.
	Verified bool
}

// Ref is the tracker-native identity of a published ticket.
type Ref struct {
	ID  string `json:"id"`  // GitHub issue number, Jira key, DefectDojo finding id
	URL string `json:"url"` // human-facing link
}

// State is what a tracker currently says about a ticket.
type State struct {
	Open       bool
	Resolution Resolution
}

// Tracker is the adapter for one external tracker. Implementations must be
// stateless: credentials always arrive via cfg.
type Tracker interface {
	// Kind returns the tracker type identifier; it matches the registry key.
	Kind() string
	// Validate checks required fields and formats; the error names the
	// missing/bad field and never contains a secret value.
	Validate(cfg map[string]any) error
	// SecretKeys lists credential config keys (masked on readback).
	SecretKeys() []string
	// DestinationKeys lists config keys deciding where credentials are sent; see
	// notify.PrepareConfigUpdateKeys for why they are tracked separately.
	DestinationKeys() []string
	// Find returns the ticket carrying key, or nil, nil when there is none.
	Find(ctx context.Context, cfg map[string]any, key string) (*Ref, error)
	// Create publishes a new ticket in the state t describes.
	Create(ctx context.Context, cfg map[string]any, t Ticket) (Ref, error)
	// Update rewrites title/body/labels and moves the ticket to t's open/closed
	// state and resolution.
	Update(ctx context.Context, cfg map[string]any, ref Ref, t Ticket) error
	// State reads the ticket's current open/closed state and resolution.
	State(ctx context.Context, cfg map[string]any, ref Ref) (State, error)
}

// Tracker kinds.
const (
	KindGitHub     = "github"
	KindJira       = "jira"
	KindDefectDojo = "defectdojo"
)

// registry is an explicit literal (not init() self-registration) so "which
// trackers exist" is visible in one place, like notify's channel registry.
var registry = map[string]Tracker{
	KindGitHub:     githubTracker{},
	KindJira:       jiraTracker{},
	KindDefectDojo: defectDojoTracker{},
}

// Get returns the tracker implementation for a kind.
func Get(kind string) (Tracker, bool) {
	t, ok := registry[kind]
	return t, ok
}

// Kinds returns the registered tracker kinds, sorted.
func Kinds() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DedupKey derives the stable ticket key from identifying parts (for example
// engagement id, finding fingerprint). Parts are trimmed and lower-cased, then
// joined with a unit separator, so the key is stable across runs but sensitive
// to part order and content.
func DedupKey(parts ...string) string {
	norm := make([]string, len(parts))
	for i, p := range parts {
		norm[i] = strings.ToLower(strings.TrimSpace(p))
	}
	sum := sha256.Sum256([]byte(strings.Join(norm, "\x1f")))
	return "artex-" + hex.EncodeToString(sum[:])[:16]
}

// Push publishes t to tr idempotently and returns the ticket's ref.
//
// With existing == nil the tracker is searched for t.Key first, so a repeated
// push (even after the caller lost the stored ref) updates the same ticket
// instead of creating a duplicate. created reports whether a new ticket was made.
func Push(ctx context.Context, tr Tracker, cfg map[string]any, existing *Ref, t Ticket) (ref Ref, created bool, err error) {
	if strings.TrimSpace(t.Key) == "" {
		return Ref{}, false, fmt.Errorf("ticket key is empty (use DedupKey)")
	}
	if existing == nil {
		found, err := tr.Find(ctx, cfg, t.Key)
		if err != nil {
			return Ref{}, false, err
		}
		existing = found
	}
	if existing != nil {
		if err := tr.Update(ctx, cfg, *existing, t); err != nil {
			return *existing, false, err
		}
		return *existing, false, nil
	}
	ref, err = tr.Create(ctx, cfg, t)
	if err != nil {
		return Ref{}, false, err
	}
	return ref, true, nil
}

// closedResolution is the resolution to apply to a closed ticket: an empty one
// defaults to fixed.
func closedResolution(t Ticket) Resolution {
	if t.Resolution == ResNone {
		return ResFixed
	}
	return t.Resolution
}
