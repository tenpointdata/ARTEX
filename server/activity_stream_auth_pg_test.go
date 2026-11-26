package server

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

// The console holds its session only in the HttpOnly cookie (no script-readable
// token), so the browser's EventSource authenticates with that cookie alone.
// These tests sign in through the real login handlers, as the console form
// does, and open the activity stream with nothing but the issued cookie, for
// both kinds of session: the bootstrap ARTEX login and a console user.

// streamTestServer is a Server over the test database with an engine (the
// stream's broadcaster), the bootstrap password initialised under test-only
// settings keys, and the activity stream wired as in production.
func streamTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { _ = m.Close() })
	stamp := time.Now().UnixNano()
	s := &Server{
		m: m, engine: NewEngine(m), ctx: context.Background(),
		jwtKey:         []byte("0123456789abcdef0123456789abcdef"),
		loginLimit:     newLoginLimiter(time.Now),
		pwdLimit:       newLoginLimiter(time.Now),
		authHashKey:    fmt.Sprintf("test.password_hash.%d", stamp),
		authVersionKey: fmt.Sprintf("test.password_version.%d", stamp),
		setupToken:     "setup-ok",
	}
	t.Cleanup(func() {
		_, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1 OR key=$2`, s.authHashKey, s.authVersionKey)
	})
	w := httptest.NewRecorder()
	s.authInit(w, httptest.NewRequest("POST", "/api/auth/init",
		strings.NewReader(`{"password":"legacy-pass-1","setup_token":"setup-ok"}`)))
	if w.Code != 200 {
		t.Fatalf("init: %d %s", w.Code, w.Body.String())
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", s.authLogin)
	mux.HandleFunc("GET /api/exploration/activity/stream", s.streamActivity)
	hs := httptest.NewServer(cors(s.requireAuth(mux)))
	t.Cleanup(hs.Close)
	return s, hs
}

// consoleLogin posts the console form's request and returns the session cookie.
func consoleLogin(t *testing.T, base, username, password string) *http.Cookie {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	resp, err := http.Post(base+"/api/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login %s: %d", username, resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookiePlain && c.Value != "" {
			return c
		}
	}
	t.Fatalf("login %s set no session cookie", username)
	return nil
}

// openStream opens the task's activity stream as the browser's EventSource
// does (same origin, cookie only, no Authorization header, no token in the URL).
func openStream(t *testing.T, ctx context.Context, base, taskID string, c *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/api/exploration/activity/stream?task="+taskID+"&since=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if c != nil {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestActivityStreamAcceptsConsoleCookieSessions(t *testing.T) {
	s, hs := streamTestServer(t)
	task, err := s.m.CreateTask("activity stream auth", "", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	u := seedUser(t, s, db.RoleReviewer, "reviewer-pass-1")

	t.Run("no session", func(t *testing.T) {
		if resp := openStream(t, context.Background(), hs.URL, task.ID, nil); resp.StatusCode != 401 {
			t.Fatalf("stream without a session: %d", resp.StatusCode)
		}
	})
	for i, kind := range []struct{ name, user, pass string }{
		{"legacy ARTEX", legacyUsername, "legacy-pass-1"},
		{"console user", u.Username, "reviewer-pass-1"},
	} {
		t.Run(kind.name, func(t *testing.T) {
			cookie := consoleLogin(t, hs.URL, kind.user, kind.pass)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			resp := openStream(t, ctx, hs.URL, task.ID, cookie)
			if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
				t.Fatalf("stream: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
			}
			// The stream opens with a comment before any event (proxies that wait
			// for a first body byte pass the headers on at once); the handler has
			// subscribed by then, so an event published now reaches this client.
			sc := bufio.NewScanner(resp.Body)
			if !sc.Scan() || sc.Text() != ": open" {
				t.Fatalf("first line %q, want the open comment (%v)", sc.Text(), sc.Err())
			}
			want := int64(1000 + i)
			s.engine.Broadcaster().Publish(task.ID, db.Activity{ID: want, Kind: "note", Summary: "hello"})
			for sc.Scan() {
				if sc.Text() == fmt.Sprintf("id: %d", want) {
					return
				}
			}
			t.Fatalf("no event frame before the stream ended: %v", sc.Err())
		})
	}
}
