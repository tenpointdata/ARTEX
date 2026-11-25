package ticketing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Push deliveries (webhooks) from a tracker.
//
// A delivery is only a hint that a ticket changed: it names the tickets, and
// the caller then reads their state through the tracker's API (Tracker.State)
// as an ordinary sync does. Nothing in the payload is trusted as state, so a
// replayed or reordered delivery can at most cause an extra read.
//
// Authenticity is an HMAC-SHA256 of the raw body under a per-tracker secret
// (config key WebhookSecretKey), compared in constant time before the body is
// parsed. GitHub sends it as X-Hub-Signature-256; Jira Cloud webhooks that
// were registered with a secret send it as X-Hub-Signature. Both use the form
// "sha256=<hex>". Jira Connect JWT and secrets carried in the URL are not
// supported: a URL secret ends up in proxy and access logs.

// WebhookSecretKey is the config key holding a tracker's webhook secret. It
// is listed in SecretKeys of every tracker that accepts webhooks, so it is
// sealed, masked and scrubbed like the API credentials.
const WebhookSecretKey = "webhook_secret"

// WebhookSecretMinLen is the shortest secret accepted for verification; a
// shorter one is treated as not configured.
const WebhookSecretMinLen = 16

// ErrWebhookSignature is returned when a delivery's signature is missing or
// does not match, or when no usable secret is configured.
var ErrWebhookSignature = errors.New("webhook signature is missing or invalid")

// WebhookEvent is what a verified delivery is about.
type WebhookEvent struct {
	// DeliveryID is the tracker's id for this delivery ("" when it sends none);
	// a redelivery carries the same id.
	DeliveryID string
	// TicketIDs are the Ref.IDs of the tickets the delivery concerns. Empty
	// for an event that changes no ticket state (ping, comment, ...).
	TicketIDs []string
}

// WebhookReceiver is implemented by trackers that accept push deliveries.
type WebhookReceiver interface {
	// ParseWebhook verifies the delivery against cfg's webhook secret and
	// returns the tickets it names. It returns ErrWebhookSignature for an
	// unauthenticated delivery and never inspects the body before that check.
	ParseWebhook(cfg map[string]any, h http.Header, body []byte) (WebhookEvent, error)
}

// Webhooks returns the receiver of a tracker kind, if it accepts webhooks.
func Webhooks(kind string) (WebhookReceiver, bool) {
	t, ok := registry[kind]
	if !ok {
		return nil, false
	}
	r, ok := t.(WebhookReceiver)
	return r, ok
}

// OutboundSecretKeys lists the credentials of t that are sent to its
// destination: SecretKeys without the webhook secret, which only verifies
// inbound deliveries. A destination change must re-ask for these (see
// notify.PrepareConfigUpdateKeys); the webhook secret is kept.
func OutboundSecretKeys(t Tracker) []string {
	var out []string
	for _, k := range t.SecretKeys() {
		if k != WebhookSecretKey {
			out = append(out, k)
		}
	}
	return out
}

// validateWebhookSecret rejects a webhook secret too short to verify with; an
// empty one (webhooks off) is fine.
func validateWebhookSecret(cfg map[string]any) error {
	if s := cfgString(cfg, WebhookSecretKey); s != "" && len(s) < WebhookSecretMinLen {
		return fmt.Errorf("%s must be at least %d characters", WebhookSecretKey, WebhookSecretMinLen)
	}
	return nil
}

// VerifyHMACSHA256 checks a "sha256=<hex>" signature of body under secret in
// constant time. A secret shorter than WebhookSecretMinLen never verifies.
func VerifyHMACSHA256(secret string, body []byte, signature string) error {
	if len(secret) < WebhookSecretMinLen {
		return ErrWebhookSignature
	}
	hexSig, ok := strings.CutPrefix(strings.TrimSpace(signature), "sha256=")
	if !ok {
		return ErrWebhookSignature
	}
	got, err := hex.DecodeString(hexSig)
	if err != nil || len(got) != sha256.Size {
		return ErrWebhookSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return ErrWebhookSignature
	}
	return nil
}

// SignHMACSHA256 returns the "sha256=<hex>" signature of body, for tests and
// for callers that relay deliveries.
func SignHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// githubIssueActions are the issues-event actions that can change what
// Tracker.State reports (open/closed, state_reason, the resolution labels).
var githubIssueActions = map[string]bool{
	"closed": true, "reopened": true, "labeled": true, "unlabeled": true, "edited": true, "deleted": true, "transferred": true,
}

// ParseWebhook accepts GitHub "issues" deliveries for the configured repo.
func (githubTracker) ParseWebhook(cfg map[string]any, h http.Header, body []byte) (WebhookEvent, error) {
	if err := VerifyHMACSHA256(cfgString(cfg, WebhookSecretKey), body, h.Get("X-Hub-Signature-256")); err != nil {
		return WebhookEvent{}, err
	}
	ev := WebhookEvent{DeliveryID: h.Get("X-GitHub-Delivery")}
	if h.Get("X-GitHub-Event") != "issues" {
		return ev, nil // ping and any other event: nothing to sync
	}
	var p struct {
		Action string `json:"action"`
		Issue  struct {
			Number int `json:"number"`
		} `json:"issue"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return ev, fmt.Errorf("github webhook: %w", err)
	}
	// A webhook on another repo (an org-level hook, a misrouted URL) must not
	// map its issue numbers onto this tracker's tickets.
	if !strings.EqualFold(p.Repository.FullName, cfgString(cfg, "repo")) || p.Issue.Number <= 0 || !githubIssueActions[p.Action] {
		return ev, nil
	}
	ev.TicketIDs = []string{strconv.Itoa(p.Issue.Number)}
	return ev, nil
}

// ParseWebhook accepts Jira issue deliveries for the configured project.
func (jiraTracker) ParseWebhook(cfg map[string]any, h http.Header, body []byte) (WebhookEvent, error) {
	if err := VerifyHMACSHA256(cfgString(cfg, WebhookSecretKey), body, h.Get("X-Hub-Signature")); err != nil {
		return WebhookEvent{}, err
	}
	ev := WebhookEvent{DeliveryID: h.Get("X-Atlassian-Webhook-Identifier")}
	var p struct {
		WebhookEvent string `json:"webhookEvent"`
		Issue        struct {
			Key string `json:"key"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return ev, fmt.Errorf("jira webhook: %w", err)
	}
	if p.WebhookEvent != "jira:issue_updated" && p.WebhookEvent != "jira:issue_deleted" {
		return ev, nil
	}
	project := cfgString(cfg, "project_key")
	if p.Issue.Key == "" || !strings.HasPrefix(strings.ToUpper(p.Issue.Key), strings.ToUpper(project)+"-") {
		return ev, nil
	}
	ev.TicketIDs = []string{p.Issue.Key}
	return ev, nil
}
