package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/ticketing"
)

const testWebhookSecret = "whsec_test_0123456789abcdef"

// webhookFixture is a synced finding whose GitHub tracker has a webhook secret.
func newWebhookFixture(t *testing.T) (*fakeGitHub, *syncFixture) {
	t.Helper()
	gh, srv := newFakeGitHub(t)
	f := newSyncFixture(t, srv.URL)
	w := f.request("PATCH", fmt.Sprintf("/api/integrations/trackers/%d", f.tracker),
		fmt.Sprintf(`{"config":{"webhook_secret":%q}}`, testWebhookSecret))
	if w.Code != 200 {
		t.Fatalf("set webhook secret: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), testWebhookSecret) {
		t.Fatal("webhook secret is not masked on readback")
	}
	f.push(t, f.fid)
	return gh, f
}

// deliver posts a delivery without a session, as GitHub does.
func deliver(f *syncFixture, tracker int64, event, delivery, sig, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", fmt.Sprintf("%s%d", ticketWebhookPrefix, tracker), strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-GitHub-Event", event)
	r.Header.Set("X-GitHub-Delivery", delivery)
	if sig != "" {
		r.Header.Set("X-Hub-Signature-256", sig)
	}
	w := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(w, r)
	return w
}

func closedPayload(repo string, n int) string {
	return fmt.Sprintf(`{"action":"closed","issue":{"number":%d,"state":"open"},"repository":{"full_name":%q}}`, n, repo)
}

// TestGitHubWebhookClosesFinding: a signed issues.closed delivery reconciles
// the link at once; the finding's new status comes from the tracker's API,
// and a redelivery of the same id is dropped.
func TestGitHubWebhookClosesFinding(t *testing.T) {
	gh, f := newWebhookFixture(t)
	gh.close(1, "completed")
	body := closedPayload("acme/app", 1)

	w := deliver(f, f.tracker, "issues", "d-1", ticketing.SignHMACSHA256(testWebhookSecret, []byte(body)), body)
	if w.Code != 200 {
		t.Fatalf("delivery: %d %s", w.Code, w.Body)
	}
	var reply ticketWebhookReply
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	if reply.Status != "ok" || reply.Synced != 1 {
		t.Fatalf("reply: %+v", reply)
	}
	if st := f.status(t, f.fid); st != db.FindingFixed {
		t.Fatalf("finding after webhook: %s, want %s", st, db.FindingFixed)
	}
	entries, err := f.pg.ListAudit(db.AuditFilter{Kind: db.AuditTicketWebhook})
	if err != nil || len(entries) == 0 {
		t.Fatalf("no %s audit entry: %v", db.AuditTicketWebhook, err)
	}

	w = deliver(f, f.tracker, "issues", "d-1", ticketing.SignHMACSHA256(testWebhookSecret, []byte(body)), body)
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	if w.Code != 200 || reply.Status != "duplicate" {
		t.Fatalf("redelivery: %d %s", w.Code, w.Body)
	}

	// Another repo's issue #1 is not this tracker's ticket.
	other := closedPayload("evil/app", 1)
	w = deliver(f, f.tracker, "issues", "d-2", ticketing.SignHMACSHA256(testWebhookSecret, []byte(other)), other)
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	if w.Code != 200 || reply.Status != "ignored" {
		t.Fatalf("other repo: %d %s", w.Code, w.Body)
	}
}

// TestGitHubWebhookRejectsBadSignature: a missing, wrong or other-secret
// signature, and an unknown tracker, all get 401 and change nothing.
func TestGitHubWebhookRejectsBadSignature(t *testing.T) {
	gh, f := newWebhookFixture(t)
	gh.close(1, "completed")
	body := closedPayload("acme/app", 1)
	cases := []struct {
		name, sig string
		tracker   int64
	}{
		{"missing", "", f.tracker},
		{"garbage", "sha256=zz", f.tracker},
		{"wrong secret", ticketing.SignHMACSHA256("not-the-secret-at-all", []byte(body)), f.tracker},
		{"tampered body", ticketing.SignHMACSHA256(testWebhookSecret, []byte(body+" ")), f.tracker},
		{"sha1 form", "sha1=" + strings.TrimPrefix(ticketing.SignHMACSHA256(testWebhookSecret, []byte(body)), "sha256="), f.tracker},
		{"unknown tracker", ticketing.SignHMACSHA256(testWebhookSecret, []byte(body)), f.tracker + 1_000_000},
	}
	for i, c := range cases {
		w := deliver(f, c.tracker, "issues", fmt.Sprintf("bad-%d", i), c.sig, body)
		if w.Code != 401 {
			t.Errorf("%s: %d %s, want 401", c.name, w.Code, w.Body)
		}
	}
	if st := f.status(t, f.fid); st == db.FindingFixed {
		t.Fatal("an unauthenticated delivery changed the finding")
	}
}

// TestWebhookNeedsConfiguredSecret: a tracker without a webhook secret
// refuses every delivery, even one signed with an empty key.
func TestWebhookNeedsConfiguredSecret(t *testing.T) {
	gh, srv := newFakeGitHub(t)
	f := newSyncFixture(t, srv.URL)
	f.push(t, f.fid)
	gh.close(1, "completed")
	body := closedPayload("acme/app", 1)
	if w := deliver(f, f.tracker, "issues", "n-1", ticketing.SignHMACSHA256("", []byte(body)), body); w.Code != 401 {
		t.Fatalf("no secret configured: %d %s, want 401", w.Code, w.Body)
	}
	if st := f.status(t, f.fid); st == db.FindingFixed {
		t.Fatal("a delivery to a tracker without a secret changed the finding")
	}
}
