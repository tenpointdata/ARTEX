package ticketing

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

const hookSecret = "0123456789abcdef-hook"

func TestVerifyHMACSHA256(t *testing.T) {
	body := []byte(`{"a":1}`)
	good := SignHMACSHA256(hookSecret, body)
	if err := VerifyHMACSHA256(hookSecret, body, good); err != nil {
		t.Fatalf("good signature: %v", err)
	}
	bad := map[string]struct {
		secret, sig string
		body        []byte
	}{
		"empty sig":     {hookSecret, "", body},
		"no prefix":     {hookSecret, strings.TrimPrefix(good, "sha256="), body},
		"bad hex":       {hookSecret, "sha256=xyz", body},
		"short mac":     {hookSecret, good[:20], body},
		"other body":    {hookSecret, good, []byte(`{"a":2}`)},
		"other secret":  {hookSecret + "x", good, body},
		"short secret":  {"short", SignHMACSHA256("short", body), body},
		"empty secret":  {"", SignHMACSHA256("", body), body},
		"uppercase alg": {hookSecret, "SHA256=" + strings.TrimPrefix(good, "sha256="), body},
	}
	for name, c := range bad {
		if err := VerifyHMACSHA256(c.secret, c.body, c.sig); !errors.Is(err, ErrWebhookSignature) {
			t.Errorf("%s: %v, want ErrWebhookSignature", name, err)
		}
	}
}

func signed(header, body string) http.Header {
	h := http.Header{}
	h.Set(header, SignHMACSHA256(hookSecret, []byte(body)))
	return h
}

func TestGitHubParseWebhook(t *testing.T) {
	cfg := map[string]any{"repo": "Acme/App", WebhookSecretKey: hookSecret}
	recv, ok := Webhooks(KindGitHub)
	if !ok {
		t.Fatal("github accepts no webhooks")
	}
	cases := []struct {
		name, event, body string
		want              []string
	}{
		{"closed", "issues", `{"action":"closed","issue":{"number":7},"repository":{"full_name":"acme/app"}}`, []string{"7"}},
		{"labeled", "issues", `{"action":"labeled","issue":{"number":7},"repository":{"full_name":"acme/app"}}`, []string{"7"}},
		{"opened", "issues", `{"action":"opened","issue":{"number":7},"repository":{"full_name":"acme/app"}}`, nil},
		{"other repo", "issues", `{"action":"closed","issue":{"number":7},"repository":{"full_name":"evil/app"}}`, nil},
		{"ping", "ping", `{"zen":"hi"}`, nil},
	}
	for _, c := range cases {
		h := signed("X-Hub-Signature-256", c.body)
		h.Set("X-GitHub-Event", c.event)
		h.Set("X-GitHub-Delivery", "abc")
		ev, err := recv.ParseWebhook(cfg, h, []byte(c.body))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !reflect.DeepEqual(ev.TicketIDs, c.want) || ev.DeliveryID != "abc" {
			t.Errorf("%s: %+v, want %v", c.name, ev, c.want)
		}
	}
	// The body is never parsed before the signature check.
	if _, err := recv.ParseWebhook(cfg, http.Header{"X-Github-Event": {"issues"}}, []byte("{not json")); !errors.Is(err, ErrWebhookSignature) {
		t.Fatalf("unsigned garbage: %v", err)
	}
}

func TestJiraParseWebhook(t *testing.T) {
	cfg := map[string]any{"project_key": "SEC", WebhookSecretKey: hookSecret}
	recv, ok := Webhooks(KindJira)
	if !ok {
		t.Fatal("jira accepts no webhooks")
	}
	body := `{"webhookEvent":"jira:issue_updated","issue":{"key":"SEC-12"}}`
	h := signed("X-Hub-Signature", body)
	h.Set("X-Atlassian-Webhook-Identifier", "j-1")
	ev, err := recv.ParseWebhook(cfg, h, []byte(body))
	if err != nil || !reflect.DeepEqual(ev.TicketIDs, []string{"SEC-12"}) || ev.DeliveryID != "j-1" {
		t.Fatalf("issue_updated: %+v %v", ev, err)
	}
	other := `{"webhookEvent":"jira:issue_updated","issue":{"key":"OPS-12"}}`
	if ev, err := recv.ParseWebhook(cfg, signed("X-Hub-Signature", other), []byte(other)); err != nil || len(ev.TicketIDs) != 0 {
		t.Fatalf("other project: %+v %v", ev, err)
	}
	if _, err := recv.ParseWebhook(cfg, signed("X-Hub-Signature-256", body), []byte(body)); !errors.Is(err, ErrWebhookSignature) {
		t.Fatalf("signature in the wrong header: %v", err)
	}
	if _, ok := Webhooks(KindDefectDojo); ok {
		t.Fatal("defectdojo should not accept webhooks")
	}
}

func TestWebhookSecretValidation(t *testing.T) {
	cfg := map[string]any{"repo": "a/b", "token": "t", WebhookSecretKey: "short"}
	if err := (githubTracker{}).Validate(cfg); err == nil {
		t.Fatal("a short webhook secret was accepted")
	}
	cfg[WebhookSecretKey] = hookSecret
	if err := (githubTracker{}).Validate(cfg); err != nil {
		t.Fatal(err)
	}
	delete(cfg, WebhookSecretKey)
	if err := (githubTracker{}).Validate(cfg); err != nil {
		t.Fatalf("no webhook secret should be valid: %v", err)
	}
}

func TestOutboundSecretKeysExcludeWebhookSecret(t *testing.T) {
	if got := OutboundSecretKeys(githubTracker{}); !reflect.DeepEqual(got, []string{"token"}) {
		t.Fatalf("github outbound secrets: %v", got)
	}
}
