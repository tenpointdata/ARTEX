package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLog collects what the standard logger writes while fn runs.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(prev); log.SetFlags(flags) }()
	fn()
	return buf.String()
}

// The start decision, branch by branch.
func TestEnforceIsolation(t *testing.T) {
	cause := errors.New("Bash runs as uid 0, not the executor uid 10001")
	root := ExecPrivilege{Enabled: true, UID: 10001, GID: 10001} // drop on: the server is root
	off := ExecPrivilege{}                                       // not root, or -exec-uid=-1
	const banner = "AGENT ISOLATION IS DEGRADED"

	var (
		st  IsolationStatus
		err error
	)

	// Hard failure: Linux, root, self-test failed, no escape. No banner: no start.
	out := captureLog(t, func() { st, err = EnforceIsolation("linux", root, cause, false) })
	if err == nil || !strings.Contains(err.Error(), cause.Error()) || !strings.Contains(err.Error(), "-"+DevNoIsolationFlag) {
		t.Fatalf("linux root: err = %v, want the cause and the flag named", err)
	}
	if st.Mode != "" || strings.Contains(out, banner) {
		t.Fatalf("linux root: status %+v, banner in\n%s", st, out)
	}

	// Linux, root, with the development escape: degraded, banner names the flag.
	out = captureLog(t, func() { st, err = EnforceIsolation("linux", root, cause, true) })
	if err != nil || st.Mode != IsolationDegraded || st.Reason != cause.Error() {
		t.Fatalf("linux root with the escape: status %+v err %v, want degraded", st, err)
	}
	for _, want := range []string{banner, "-" + DevNoIsolationFlag, "DEVELOPMENT ONLY", cause.Error(), "keys", "database"} {
		if !strings.Contains(out, want) {
			t.Errorf("escape banner lacks %q:\n%s", want, out)
		}
	}

	// A host that cannot isolate (Linux not root, Windows, macOS root or not) fails
	// closed: no start without the operator's opt-in, and the error names it.
	hosts := []struct {
		goos string
		p    ExecPrivilege
	}{{"linux", off}, {"windows", off}, {"darwin", off}, {"darwin", root}}
	for _, v := range []string{"", "0", "no"} {
		t.Setenv(AllowUnsandboxedEnv, v)
		for _, c := range hosts {
			out = captureLog(t, func() { st, err = EnforceIsolation(c.goos, c.p, cause, false) })
			if err == nil || st.Mode != "" {
				t.Fatalf("%s drop=%v %s=%q: status %+v err %v, want a refused start", c.goos, c.p.Enabled, AllowUnsandboxedEnv, v, st, err)
			}
			for _, want := range []string{AllowUnsandboxedEnv + "=1", cause.Error(), "unsandboxed", "start.sh"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%s: refusal lacks %q: %v", c.goos, want, err)
				}
			}
			if strings.Contains(out, banner) {
				t.Errorf("%s: a refused start logged the degraded banner:\n%s", c.goos, out)
			}
		}
	}

	// With the opt-in (the environment variable, or the development flag) the host
	// starts degraded, and the banner says why and exactly what is exposed.
	for _, optIn := range []struct {
		env    string
		escape bool
	}{{"1", false}, {"true", false}, {"", true}} {
		t.Setenv(AllowUnsandboxedEnv, optIn.env)
		for _, c := range hosts {
			out = captureLog(t, func() { st, err = EnforceIsolation(c.goos, c.p, cause, optIn.escape) })
			if err != nil || st.Mode != IsolationDegraded || st.Reason != cause.Error() {
				t.Fatalf("%s drop=%v opt-in %+v: status %+v err %v, want a degraded start", c.goos, c.p.Enabled, optIn, st, err)
			}
			for _, want := range []string{banner, "UNSANDBOXED", "cannot isolate agents", AllowUnsandboxedEnv, cause.Error(),
				"jwt.key", "secret.key", "database", "approval rules", "artex binary", "start.sh", "SSH keys"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s drop=%v: banner lacks %q:\n%s", c.goos, c.p.Enabled, want, out)
				}
			}
			if strings.Contains(out, "DEVELOPMENT ONLY") {
				t.Errorf("%s drop=%v: the degraded banner is not the escape's:\n%s", c.goos, c.p.Enabled, out)
			}
		}
	}
	t.Setenv(AllowUnsandboxedEnv, "")

	// The environment opt-in does not replace the development escape where
	// isolation is expected and broken (Linux, root, drop on).
	t.Setenv(AllowUnsandboxedEnv, "1")
	if _, err = EnforceIsolation("linux", root, cause, false); err == nil {
		t.Fatalf("linux root with %s=1: want the start refused", AllowUnsandboxedEnv)
	}
	t.Setenv(AllowUnsandboxedEnv, "")

	// A passing self-test starts isolated, escape or not, without a banner.
	for _, escape := range []bool{false, true} {
		out = captureLog(t, func() { st, err = EnforceIsolation("linux", root, nil, escape) })
		if err != nil || st.Mode != IsolationIsolated || !strings.Contains(out, "self-test passed") || strings.Contains(out, banner) {
			t.Fatalf("passing self-test (escape %v): status %+v err=%v\n%s", escape, st, err, out)
		}
	}
}

// GET /api/health reports the isolation status to a signed-in caller only: the web
// UI shows the degraded banner from it.
func TestHealthReportsIsolation(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	s := &Server{jwtKey: key}
	WithIsolationStatus(IsolationStatus{Mode: IsolationDegraded, Reason: "the server is not root"})(s)

	w := httptest.NewRecorder()
	s.health(w, httptest.NewRequest("GET", "/api/health", nil))
	if got := strings.TrimSpace(w.Body.String()); got != `{"ok":true}` {
		t.Fatalf("no token: %s, want no isolation status", got)
	}

	tok, err := signJWT(key)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/health", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	s.health(w, r)
	var body struct {
		Isolation *IsolationStatus `json:"isolation"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Isolation == nil ||
		body.Isolation.Mode != IsolationDegraded || body.Isolation.Reason != "the server is not root" {
		t.Fatalf("signed in: %s, want the degraded status", w.Body.String())
	}
}

// With the drop disabled (the server is not root, -exec-uid=-1, or no credentials
// on this platform) the self-test reports isolation as unavailable.
func TestIsolationSelfTestUnavailableWithoutDrop(t *testing.T) {
	if err := IsolationSelfTest(ExecPrivilege{}, IsolationTargets{}); err == nil {
		t.Fatal("a disabled drop must fail the self-test")
	}
}
