package ticketing

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/notify"
)

func TestDedupKeyStableAndSensitive(t *testing.T) {
	a := DedupKey("eng-1", "finding-1")
	if a != DedupKey("eng-1", "finding-1") {
		t.Fatal("DedupKey is not stable")
	}
	if !strings.HasPrefix(a, "artex-") || len(a) != len("artex-")+16 {
		t.Fatalf("unexpected key shape %q", a)
	}
	if a != DedupKey("  ENG-1 ", "Finding-1") {
		t.Fatal("parts must be trimmed and case-folded")
	}
	for _, other := range [][]string{{"eng-1", "finding-2"}, {"eng-2", "finding-1"}, {"finding-1", "eng-1"}, {"eng-1finding-1"}, {"eng-1", "finding-1", ""}} {
		if DedupKey(other...) == a {
			t.Fatalf("DedupKey(%q) collides with the base key", other)
		}
	}
}

func TestRegistry(t *testing.T) {
	if got, want := Kinds(), []string{"defectdojo", "github", "jira"}; !slices.Equal(got, want) {
		t.Fatalf("Kinds = %v, want %v", got, want)
	}
	for _, k := range Kinds() {
		tr, ok := Get(k)
		if !ok || tr.Kind() != k || len(tr.SecretKeys()) == 0 || len(tr.DestinationKeys()) == 0 {
			t.Fatalf("tracker %q is not well formed", k)
		}
	}
	if _, ok := Get("trello"); ok {
		t.Fatal("unknown kind resolved")
	}
}

func TestPushRejectsEmptyKey(t *testing.T) {
	tr, _ := Get(KindGitHub)
	if _, _, err := Push(context.Background(), tr, nil, nil, Ticket{Title: "x"}); err == nil {
		t.Fatal("empty key must be rejected")
	}
}

// The guarded client must refuse loopback unless the escape hatch is open.
func TestGuardedClientRefusesLoopback(t *testing.T) {
	srv, rec := newFake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"items":[]}`)
	}))
	t.Setenv(notify.AllowLocalTargetsEnv, "")
	tr, _ := Get(KindGitHub)
	cfg := map[string]any{"base_url": srv.URL, "repo": "acme/widgets", "token": "ghp_secret"}
	_, err := tr.Find(context.Background(), cfg, DedupKey("x"))
	if err == nil {
		t.Fatal("loopback target must be refused")
	}
	if !strings.Contains(err.Error(), notify.AllowLocalTargetsEnv) || strings.Contains(err.Error(), "ghp_secret") {
		t.Fatalf("unexpected error text: %v", err)
	}
	if len(rec.all()) != 0 {
		t.Fatal("the fake must not have been reached")
	}
}

func TestBaseURLValidation(t *testing.T) {
	for _, bad := range []string{"", "ftp://x.test", "http://", "https://user:pw@x.test", "https://x.test/?a=1", "x.test"} {
		if _, err := baseURL(map[string]any{"base_url": bad}, ""); err == nil {
			t.Fatalf("baseURL(%q) accepted", bad)
		} else if strings.Contains(err.Error(), "pw") {
			t.Fatalf("error echoes credentials: %v", err)
		}
	}
	got, err := baseURL(map[string]any{"base_url": "https://jira.example.com/ctx/"}, "")
	if err != nil || got != "https://jira.example.com/ctx" {
		t.Fatalf("baseURL = %q, %v", got, err)
	}
}

func TestMaskingViaNotifyKeyLists(t *testing.T) {
	for _, k := range Kinds() {
		tr, _ := Get(k)
		secret := tr.SecretKeys()[0]
		dest := tr.DestinationKeys()[0]
		stored := map[string]any{secret: "supersecretvalue", dest: "https://a.example.com"}
		masked := notify.MaskConfigKeys(tr.SecretKeys(), stored)
		if masked[secret] == "supersecretvalue" || !notify.IsMasked(masked[secret].(string)) || masked[dest] != stored[dest] {
			t.Fatalf("%s: masked = %v", k, masked)
		}
		// Changing the destination while keeping the masked credential is refused.
		_, err := notify.PrepareConfigUpdateKeys(tr.SecretKeys(), tr.DestinationKeys(), stored,
			map[string]any{dest: "https://evil.example.com", secret: masked[secret]})
		if err == nil {
			t.Fatalf("%s: destination change with masked credential accepted", k)
		}
	}
}
