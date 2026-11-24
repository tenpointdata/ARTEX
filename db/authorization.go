package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"
)

// ErrAuthorizationNoTask is returned when the task does not exist or was deleted.
var ErrAuthorizationNoTask = errors.New("task not found")

// An engagement authorization artifact is one task's rules of engagement:
// which targets may be touched, in which time window, with which technique
// classes (migration 0016). Rows are versions and never change; the newest
// version of a task is the one in force. The target gate (package intercept)
// is the only place that enforces it.

// Scope entry kinds of an authorization artifact.
const (
	AuthScopeDomain = "domain" // the name and every name below it
	AuthScopeHost   = "host"   // exactly this name
	AuthScopeIP     = "ip"     // exactly this address
	AuthScopeCIDR   = "cidr"   // every address in the prefix
)

// AuthScopeEntry is one entry of an artifact's scope.
type AuthScopeEntry struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// EngagementAuthorization is one version of a task's artifact.
type EngagementAuthorization struct {
	ID           int64            `json:"id"`
	TaskID       int64            `json:"task_id"`
	Version      int              `json:"version"`
	Reference    string           `json:"reference"`
	AuthorizedBy string           `json:"authorized_by"`
	Scope        []AuthScopeEntry `json:"scope"`
	Techniques   []string         `json:"techniques"`
	NotBefore    time.Time        `json:"not_before"`
	NotAfter     time.Time        `json:"not_after"`
	Revoked      bool             `json:"revoked"`
	Note         string           `json:"note"`
	CreatedBy    string           `json:"created_by"`
	CreatedAt    time.Time        `json:"created_at"`
}

// AuthorizationInput is what a new version carries; the store numbers it.
type AuthorizationInput struct {
	Reference    string           `json:"reference"`
	AuthorizedBy string           `json:"authorized_by"`
	Scope        []AuthScopeEntry `json:"scope"`
	Techniques   []string         `json:"techniques"`
	NotBefore    time.Time        `json:"not_before"`
	NotAfter     time.Time        `json:"not_after"`
	Note         string           `json:"note"`
}

// maxAuthScopeEntries bounds one artifact's scope list.
const maxAuthScopeEntries = 1000

// Normalize validates the input against the technique classes the gate knows
// and returns it in canonical form: lower-case names, masked prefixes,
// sorted and de-duplicated lists.
func (in AuthorizationInput) Normalize(knownTechniques []string) (AuthorizationInput, error) {
	if in.NotBefore.IsZero() || in.NotAfter.IsZero() {
		return in, errors.New("not_before and not_after are required")
	}
	if !in.NotAfter.After(in.NotBefore) {
		return in, errors.New("not_after must be later than not_before")
	}
	if len(in.Scope) > maxAuthScopeEntries {
		return in, fmt.Errorf("at most %d scope entries", maxAuthScopeEntries)
	}
	scope := make([]AuthScopeEntry, 0, len(in.Scope))
	for _, e := range in.Scope {
		n, err := normalizeAuthScope(e)
		if err != nil {
			return in, err
		}
		if !slices.Contains(scope, n) {
			scope = append(scope, n)
		}
	}
	slices.SortFunc(scope, func(a, b AuthScopeEntry) int {
		return strings.Compare(a.Kind+"\x00"+a.Value, b.Kind+"\x00"+b.Value)
	})
	techs := make([]string, 0, len(in.Techniques))
	for _, t := range in.Techniques {
		t = strings.ToLower(strings.TrimSpace(t))
		if !slices.Contains(knownTechniques, t) {
			return in, fmt.Errorf("unknown technique class %q (known: %s)", t, strings.Join(knownTechniques, ", "))
		}
		if !slices.Contains(techs, t) {
			techs = append(techs, t)
		}
	}
	slices.Sort(techs)
	in.Scope, in.Techniques = scope, techs
	in.Reference, in.AuthorizedBy, in.Note = strings.TrimSpace(in.Reference), strings.TrimSpace(in.AuthorizedBy), strings.TrimSpace(in.Note)
	in.NotBefore, in.NotAfter = in.NotBefore.UTC(), in.NotAfter.UTC()
	return in, nil
}

func normalizeAuthScope(e AuthScopeEntry) (AuthScopeEntry, error) {
	v := strings.ToLower(strings.TrimSpace(e.Value))
	switch e.Kind {
	case AuthScopeDomain, AuthScopeHost:
		v = strings.TrimSuffix(strings.TrimPrefix(v, "*."), ".")
		if v == "" || strings.ContainsAny(v, "/: \t*") {
			return e, fmt.Errorf("scope %s %q is not a host name", e.Kind, e.Value)
		}
	case AuthScopeIP:
		a, err := netip.ParseAddr(v)
		if err != nil {
			return e, fmt.Errorf("scope ip %q: %w", e.Value, err)
		}
		v = a.Unmap().WithZone("").String()
	case AuthScopeCIDR:
		p, err := netip.ParsePrefix(v)
		if err != nil {
			return e, fmt.Errorf("scope cidr %q: %w", e.Value, err)
		}
		v = p.Masked().String()
	default:
		return e, fmt.Errorf("scope kind %q must be domain, host, ip or cidr", e.Kind)
	}
	return AuthScopeEntry{Kind: e.Kind, Value: v}, nil
}

// InForce reports whether the artifact authorizes action at now: not revoked
// and inside [NotBefore, NotAfter).
func (a *EngagementAuthorization) InForce(now time.Time) bool {
	return !a.Revoked && !now.Before(a.NotBefore) && now.Before(a.NotAfter)
}

// AllowsTechnique reports whether the class may be used. An empty list allows all.
func (a *EngagementAuthorization) AllowsTechnique(class string) bool {
	return len(a.Techniques) == 0 || slices.Contains(a.Techniques, class)
}

// CoversHost reports whether a target is inside the scope. host is a
// lower-case name, an IP literal or a masked prefix (kind domain, ip, cidr as
// in package intercept). A name is matched only by domain and host entries,
// an address or a prefix only by ip and cidr entries: a name is never
// resolved here. An empty scope covers everything (the gate's own checks
// still apply).
func (a *EngagementAuthorization) CoversHost(kind, host string) bool {
	if len(a.Scope) == 0 {
		return true
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	var addr netip.Addr
	var prefix netip.Prefix
	switch kind {
	case "ip":
		var err error
		if addr, err = netip.ParseAddr(host); err != nil {
			return false
		}
		addr = addr.Unmap()
	case "cidr":
		var err error
		if prefix, err = netip.ParsePrefix(host); err != nil {
			return false
		}
	}
	for _, e := range a.Scope {
		switch {
		case kind == "domain" && e.Kind == AuthScopeHost:
			if host == e.Value {
				return true
			}
		case kind == "domain" && e.Kind == AuthScopeDomain:
			if host == e.Value || strings.HasSuffix(host, "."+e.Value) {
				return true
			}
		case kind == "ip" && (e.Kind == AuthScopeIP || e.Kind == AuthScopeCIDR):
			if p, err := netip.ParsePrefix(entryPrefix(e)); err == nil && p.Contains(addr) {
				return true
			}
		case kind == "cidr" && (e.Kind == AuthScopeIP || e.Kind == AuthScopeCIDR):
			if p, err := netip.ParsePrefix(entryPrefix(e)); err == nil &&
				p.Bits() <= prefix.Bits() && p.Contains(prefix.Addr()) {
				return true
			}
		}
	}
	return false
}

func entryPrefix(e AuthScopeEntry) string {
	if e.Kind == AuthScopeCIDR {
		return e.Value
	}
	a, err := netip.ParseAddr(e.Value)
	if err != nil {
		return ""
	}
	return netip.PrefixFrom(a, a.BitLen()).String()
}

const authorizationColumns = `id, task_id, version, reference, authorized_by, scope, array_to_json(techniques)::text,
	not_before, not_after, revoked, note, created_by, created_at`

func scanAuthorization(row interface{ Scan(...any) error }) (*EngagementAuthorization, error) {
	var a EngagementAuthorization
	var scope []byte
	var techs string
	if err := row.Scan(&a.ID, &a.TaskID, &a.Version, &a.Reference, &a.AuthorizedBy, &scope, &techs,
		&a.NotBefore, &a.NotAfter, &a.Revoked, &a.Note, &a.CreatedBy, &a.CreatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(scope, &a.Scope); err != nil {
		return nil, fmt.Errorf("authorization %d scope: %w", a.ID, err)
	}
	if err := json.Unmarshal([]byte(techs), &a.Techniques); err != nil {
		return nil, fmt.Errorf("authorization %d techniques: %w", a.ID, err)
	}
	if a.Scope == nil {
		a.Scope = []AuthScopeEntry{}
	}
	if a.Techniques == nil {
		a.Techniques = []string{}
	}
	return &a, nil
}

// CurrentAuthorization returns the newest version of the task's artifact, or
// nil when the task has none.
func (d *DB) CurrentAuthorization(taskID int64) (*EngagementAuthorization, error) {
	a, err := scanAuthorization(d.QueryRow(`SELECT `+authorizationColumns+` FROM engagement_authorizations
		WHERE task_id = $1 ORDER BY version DESC LIMIT 1`, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// ListAuthorizations returns every version of the task's artifact, newest first.
func (d *DB) ListAuthorizations(taskID int64) ([]*EngagementAuthorization, error) {
	rows, err := d.Query(`SELECT `+authorizationColumns+` FROM engagement_authorizations
		WHERE task_id = $1 ORDER BY version DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*EngagementAuthorization{}
	for rows.Next() {
		a, err := scanAuthorization(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AddAuthorization stores in (already normalized) as the task's next version.
// revoked stores a revocation: the version carries the previous one's terms
// and authorizes nothing. actor is recorded on the audit chain entry.
func (d *DB) AddAuthorization(taskID int64, in AuthorizationInput, revoked bool, actor string) (*EngagementAuthorization, error) {
	if in.Scope == nil {
		in.Scope = []AuthScopeEntry{}
	}
	if in.Techniques == nil {
		in.Techniques = []string{}
	}
	scope, err := json.Marshal(in.Scope)
	if err != nil {
		return nil, err
	}
	techs, err := json.Marshal(in.Techniques)
	if err != nil {
		return nil, err
	}
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT set_config('artex.actor', $1, true)`, actor); err != nil {
		return nil, err
	}
	// The task row lock serializes version numbering per task.
	var alive bool
	if err := tx.QueryRow(`SELECT deleted_at IS NULL FROM tasks WHERE id = $1 FOR UPDATE`, taskID).Scan(&alive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAuthorizationNoTask
		}
		return nil, err
	}
	if !alive {
		return nil, ErrAuthorizationNoTask
	}
	a, err := scanAuthorization(tx.QueryRow(`
INSERT INTO engagement_authorizations(task_id, version, reference, authorized_by, scope, techniques,
    not_before, not_after, revoked, note, created_by)
SELECT $1, COALESCE(MAX(version), 0) + 1, $2, $3, $4::jsonb, ARRAY(SELECT jsonb_array_elements_text($5::jsonb)),
    $6, $7, $8, $9, $10
FROM engagement_authorizations WHERE task_id = $1
RETURNING `+authorizationColumns,
		taskID, in.Reference, in.AuthorizedBy, string(scope), string(techs),
		in.NotBefore, in.NotAfter, revoked, in.Note, actor))
	if err != nil {
		return nil, err
	}
	return a, tx.Commit()
}

// Input returns the terms of a version, to carry into the next one.
func (a *EngagementAuthorization) Input() AuthorizationInput {
	return AuthorizationInput{Reference: a.Reference, AuthorizedBy: a.AuthorizedBy, Scope: a.Scope,
		Techniques: a.Techniques, NotBefore: a.NotBefore, NotAfter: a.NotAfter, Note: a.Note}
}
