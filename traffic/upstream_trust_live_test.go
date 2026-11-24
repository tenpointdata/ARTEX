package traffic

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Real pip and git downloads through the capture proxy, with the most
// permissive settings (a policy that trusts every host, and the global
// opt-out): registries and code hosts are still verified and still work.
// Needs the network, pip and git: ARTEX_LIVE_NET=1 go test ./traffic -run Live
func TestLiveRegistryDownloadsThroughProxy(t *testing.T) {
	if os.Getenv("ARTEX_LIVE_NET") != "1" {
		t.Skip("set ARTEX_LIVE_NET=1 to reach pypi.org and github.com")
	}
	for _, bin := range []string{"python3", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip(bin, " not installed")
		}
	}
	t.Setenv("ARTEX_CAPTURE_INSECURE_UPSTREAM", "1")
	tr, _, proxyURL := startProxy(t)
	tr.SetUpstreamTrust(func(string) bool { return true })
	for _, h := range []string{"pypi.org", "files.pythonhosted.org", "github.com"} {
		if tr.upstreamInsecure(h) {
			t.Fatalf("%s must be verified", h)
		}
	}
	env := append(os.Environ(), "HTTPS_PROXY="+proxyURL, "HTTP_PROXY="+proxyURL,
		"PIP_CERT="+tr.CACertPath(), "GIT_SSL_CAINFO="+tr.CACertPath(), "PIP_NO_CACHE_DIR=1")
	run := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
		}
	}
	run("python3", "-m", "pip", "download", "--no-deps", "--isolated",
		"--proxy", proxyURL, "--cert", tr.CACertPath(), "-d", t.TempDir(), "six==1.16.0")
	run("git", "-c", "http.proxy="+proxyURL, "ls-remote", "--heads", "https://github.com/golang/example.git")
}
