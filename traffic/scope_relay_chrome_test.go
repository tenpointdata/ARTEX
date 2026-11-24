package traffic

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// chromeDump loads u in headless Chrome through proxy and returns the DOM.
// --proxy-bypass-list=<-loopback> makes Chrome proxy loopback targets too.
func chromeDump(t *testing.T, chrome, proxy, u string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-first-run",
		"--user-data-dir="+t.TempDir(), "--proxy-server="+proxy, "--proxy-bypass-list=<-loopback>",
		"--dump-dom", u).Output()
	return string(out)
}

// A real Chrome (ARTEX_TEST_CHROME=<path to the binary>; skipped otherwise),
// as the Playwright MCP launches it with --proxy-server:
//   - given the scoped URL itself, it makes no dial on the scope (it drops the
//     userinfo): the premise of the relay; logged, not asserted;
//   - given the relay's URL, its dial runs on the relay's scope;
//   - once the scope is forgotten, the relay's dial is refused.
func TestScopeRelayRealChrome(t *testing.T) {
	chrome := os.Getenv("ARTEX_TEST_CHROME")
	if chrome == "" {
		t.Skip("ARTEX_TEST_CHROME not set")
	}
	tr, _, _ := startProxy(t)
	spy := &scopeSpy{}
	tr.SetUpstreamDialer(spy.dial)
	target := textServer(t, "relay-marker")

	scoped := tr.ScopedProxyURL("task-chrome", runScope("task-chrome"))
	chromeDump(t, chrome, scoped, target.URL+"/direct")
	if got := dialsByScope(spy.take()); got["task-chrome"] != 0 {
		t.Logf("Chrome sent the scoped URL's credential (dials %v): the relay is not needed for this build", got)
	} else {
		t.Logf("Chrome given the scoped URL made no dial on its scope (dials %v)", got)
	}

	relay := startRelay(t, tr, "task-chrome")
	if dom := chromeDump(t, chrome, relay.URL, target.URL+"/relay"); !strings.Contains(dom, "relay-marker") {
		t.Fatalf("Chrome through the relay did not load the page: %q", dom)
	}
	got := spy.take()
	if by := dialsByScope(got); by["task-chrome"] == 0 || len(by) != 1 {
		t.Fatalf("Chrome through the relay dialled %v, want only scope task-chrome", got)
	}

	tr.ForgetScope("task-chrome")
	if dom := chromeDump(t, chrome, relay.URL, target.URL+"/forgotten"); strings.Contains(dom, "relay-marker") {
		t.Fatal("Chrome reached the target through a relay of a forgotten scope")
	}
	if got := spy.take(); len(got) != 0 {
		t.Fatalf("forgotten scope dialled %v", got)
	}
}
