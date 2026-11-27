package traffic

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// countingServer answers "ok" and counts the requests it receives.
func countingServer(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_, _ = io.WriteString(w, "ok")
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "http://" + ln.Addr().String(), &n
}

// keepAliveGet sends one absolute-form GET on conn (with proxyAuth, if any)
// and reads its response.
func keepAliveGet(conn net.Conn, br *bufio.Reader, target, proxyAuth string) (int, error) {
	req, _ := http.NewRequest(http.MethodGet, target, nil)
	if proxyAuth != "" {
		req.Header.Set("Proxy-Authorization", proxyAuth)
	}
	if err := req.WriteProxy(conn); err != nil {
		return 0, err
	}
	res, err := http.ReadResponse(br, req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	return res.StatusCode, nil
}

// basicAuth is the Proxy-Authorization a client sends for proxyURL's userinfo.
func basicAuth(t *testing.T, proxyURL string) (host, header string) {
	t.Helper()
	u, err := url.Parse(proxyURL)
	if err != nil {
		t.Fatal(err)
	}
	req := &http.Request{Header: http.Header{}}
	if u.User != nil {
		pass, _ := u.User.Password()
		req.SetBasicAuth(u.User.Username(), pass)
	}
	return u.Host, req.Header.Get("Authorization")
}

// ForgetScope closes the proxy client connections bound to the scope, by
// credential and by scope relay: a keep-alive connection whose upstream is
// already open (pinned) cannot reach that host again once the scope ends.
func TestForgetScopeClosesBoundConnections(t *testing.T) {
	for _, path := range []string{"credential", "relay"} {
		t.Run(path, func(t *testing.T) {
			tr, _, _ := startProxy(t)
			target, hits := countingServer(t)
			key := "task-forget-" + path
			proxyURL := tr.ScopedProxyURL(key, runScope(key))
			if path == "relay" {
				proxyURL = startRelay(t, tr, key).URL
			}
			host, auth := basicAuth(t, proxyURL)
			conn, err := net.Dial("tcp", host)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			br := bufio.NewReader(conn)
			for i := range 2 { // the second request reuses the pinned upstream
				if code, err := keepAliveGet(conn, br, target+"/", auth); err != nil || code != http.StatusOK {
					t.Fatalf("request %d before ForgetScope: %d %v", i, code, err)
				}
			}
			before := hits.Load()

			tr.ForgetScope(key)
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			code, err := keepAliveGet(conn, br, target+"/", auth)
			if err == nil && code == http.StatusOK {
				t.Fatal("a keep-alive request to the pinned host reached it after ForgetScope")
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				t.Fatalf("the connection stayed open after ForgetScope: %v", err)
			}
			if got := hits.Load(); got != before {
				t.Fatalf("target saw %d requests after ForgetScope", got-before)
			}
		})
	}
}
