package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/artex/traffic"
)

// scopeDial is one upstream dial of the recording proxy and the target scope
// its context carried.
type scopeDial struct {
	addr  string
	scope intercept.TargetScope
	ok    bool
}

// scopeDialSpy is the proxy's upstream dialer: it records each dial's scope
// and dials directly.
type scopeDialSpy struct {
	mu    sync.Mutex
	dials []scopeDial
}

func (s *scopeDialSpy) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	sc, ok := intercept.TargetScopeFromContext(ctx)
	s.mu.Lock()
	s.dials = append(s.dials, scopeDial{addr: addr, scope: sc, ok: ok})
	s.mu.Unlock()
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

func (s *scopeDialSpy) take() []scopeDial {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.dials
	s.dials = nil
	return out
}

// startScopedProxy starts a recording proxy whose upstream dials go through spy,
// and a Manager with capture on that owns it.
func startScopedProxy(t *testing.T) (*Manager, *scopeDialSpy) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	tr, err := traffic.Open(t.TempDir(), addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	spy := &scopeDialSpy{}
	tr.SetUpstreamDialer(spy.dial)
	go func() { _ = tr.Start() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("recording proxy didn't start: %v", err)
		}
		time.Sleep(15 * time.Millisecond)
	}
	return &Manager{traffic: tr, trafficOn: true, tasks: map[string]*Task{}}, spy
}

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	t.Cleanup(srv.Close)
	return srv
}

// statusVia GETs target through the transport and returns the status (0 on a
// transport error).
func statusVia(t *testing.T, rt http.RoundTripper, target string) int {
	t.Helper()
	resp, err := (&http.Client{Transport: rt, Timeout: 5 * time.Second}).Get(target)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func proxyTransport(t *testing.T, proxy string) *http.Transport {
	t.Helper()
	u, err := url.Parse(proxy)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Transport{Proxy: http.ProxyURL(u), DisableKeepAlives: true}
}

func conversationCtx(id int64) context.Context {
	return intercept.WithTargetScope(context.Background(), intercept.TargetScope{
		ConversationID: id, Enabled: func() bool { return true }, WaitBound: intercept.DefaultTargetWaitBound,
	})
}

// The chat agent's proxy URL (its resolver is ScopedProxyAddr) and the http
// custom tool's recording-proxy transport carry the calling conversation's
// scope: the proxy's upstream dials run under it, with no wait. A deleted
// conversation's URL is refused, never downgraded to the global rules.
func TestScopedProxyDialsCarryConversationScope(t *testing.T) {
	m, spy := startScopedProxy(t)
	target := okServer(t)
	ctx := conversationCtx(7)

	chatURL := m.ScopedProxyAddr(ctx)
	if chatURL == m.ProxyAddr() {
		t.Fatalf("conversation proxy URL %q carries no scope", chatURL)
	}
	s := &Server{m: m}
	toolTr := s.httpProxyTransport(ctx, httpExec{UseRecordingProxy: true})
	if toolTr == nil {
		t.Fatal("http tool: no recording-proxy transport")
	}
	for name, rt := range map[string]http.RoundTripper{"chat": proxyTransport(t, chatURL), "http tool": toolTr} {
		if code := statusVia(t, rt, target.URL); code != http.StatusOK {
			t.Fatalf("%s: status %d", name, code)
		}
		got := spy.take()
		if len(got) != 1 || !got[0].ok || got[0].scope.ConversationID != 7 || got[0].scope.TaskID != "" {
			t.Fatalf("%s: dials %+v, want one on conversation 7", name, got)
		}
		if got[0].scope.WaitBound != 0 || got[0].scope.Enabled == nil || !got[0].scope.Enabled() {
			t.Fatalf("%s: dial scope %+v, want the turn's gate with no wait", name, got[0].scope)
		}
	}
	if again := m.ScopedProxyAddr(conversationCtx(7)); again != chatURL {
		t.Fatalf("a later turn got %q, want the conversation's stable URL %q", again, chatURL)
	}
	if other := m.ScopedProxyAddr(conversationCtx(8)); other == chatURL {
		t.Fatal("two conversations share one scope token")
	}

	m.forgetConversationScope(7)
	if code := statusVia(t, proxyTransport(t, chatURL), target.URL); code == http.StatusOK {
		t.Fatal("a deleted conversation's proxy URL still reached the target")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("a deleted conversation's URL dialled %v", got)
	}
}

// A task scope gets its task's URL while the task is loaded and a refused URL
// once it is not; a context with no scope gets the global rules; with capture
// off the URL is ProxyAddr.
func TestScopedProxyAddrTaskAndUnscoped(t *testing.T) {
	m, spy := startScopedProxy(t)
	target := okServer(t)
	task := &Task{ID: "task-9"}
	m.tasks[task.ID] = task
	ctx := gatedCtx(task, intercept.DefaultTargetWaitBound)

	if got, want := m.ScopedProxyAddr(ctx), m.TaskProxyAddr(task); got != want {
		t.Fatalf("task URL %q, want TaskProxyAddr %q", got, want)
	}
	if code := statusVia(t, proxyTransport(t, m.ScopedProxyAddr(ctx)), target.URL); code != http.StatusOK {
		t.Fatalf("task: status %d", code)
	}
	if got := spy.take(); len(got) != 1 || got[0].scope.TaskID != task.ID {
		t.Fatalf("task dials %+v, want one on %s", got, task.ID)
	}

	m.forgetTask(task.ID, 0)
	// Neither path mints the forgotten task a token again (one nothing would free).
	for _, ended := range []string{m.ScopedProxyAddr(ctx), m.TaskProxyAddr(task)} {
		if code := statusVia(t, proxyTransport(t, ended), target.URL); code == http.StatusOK {
			t.Fatalf("a forgotten task's call reached the target via %s", ended)
		}
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("a forgotten task dialled %v", got)
	}

	if got := m.ScopedProxyAddr(context.Background()); got != m.ProxyAddr() {
		t.Fatalf("unscoped URL %q, want %q", got, m.ProxyAddr())
	}
	if code := statusVia(t, proxyTransport(t, m.ProxyAddr()), target.URL); code != http.StatusOK {
		t.Fatalf("unscoped: status %d", code)
	}
	if got := spy.take(); len(got) != 1 || got[0].ok {
		t.Fatalf("unscoped dials %+v, want one with no scope", got)
	}

	m.trafficOn = false
	if got := m.ScopedProxyAddr(conversationCtx(7)); got != m.ProxyAddr() {
		t.Fatalf("capture off: %q, want ProxyAddr %q", got, m.ProxyAddr())
	}
}
