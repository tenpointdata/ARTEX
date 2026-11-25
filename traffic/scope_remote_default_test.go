package traffic

import (
	"net/http"
	"testing"
)

// A proxy without a remote resolver (a single node, any role) refuses a
// non-loopback client that names no run scope by default, so
// ARTEX_PROXY_ALLOW_NONLOOPBACK=1 alone never makes it an open relay. Loopback
// clients keep the global rules. ARTEX_PROXY_ALLOW_UNSCOPED_REMOTE=1 is the
// explicit opt-out, and a remote resolver turns the requirement back on.
func TestRemoteClientScopeRequiredByDefault(t *testing.T) {
	target := textServer(t, "ok")

	tr, _, proxyURL := startProxy(t)
	if code, _ := getBody(t, proxyClient(t, tr, proxyURL), target.URL); code != http.StatusOK {
		t.Fatalf("a loopback client without a token was refused: %d", code)
	}
	treatAllRemote(tr)
	if code, body := getBody(t, proxyClient(t, tr, proxyURL), target.URL); code == http.StatusOK {
		t.Fatalf("default: a remote client without a token reached the target: %q", body)
	}

	t.Setenv("ARTEX_PROXY_ALLOW_UNSCOPED_REMOTE", "1")
	opt, _, optURL := startProxy(t)
	treatAllRemote(opt)
	if code, _ := getBody(t, proxyClient(t, opt, optURL), target.URL); code != http.StatusOK {
		t.Fatalf("opt-out: a remote client without a token was refused: %d", code)
	}
	opt.SetRemoteScopeResolver(newFakeRemote(map[string]string{}).resolve)
	if code, body := getBody(t, proxyClient(t, opt, optURL), target.URL); code == http.StatusOK {
		t.Fatalf("opt-out with a remote resolver: a remote client without a token reached the target: %q", body)
	}
}
