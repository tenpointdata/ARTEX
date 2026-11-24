//go:build linux

package server

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/Autumn-27/artex/ownerfw"
)

// A native Linux root start installs the floor, and then curl as the executor
// uid (and as the MCP uid) cannot reach 169.254.169.254, while root still can.
// The metadata address is put on the loopback interface with a listener behind
// it, so the test sees a refusal of the floor, not a host without a route: before
// the floor is installed the executor reaches it. Needs root, CAP_NET_ADMIN,
// iptables, ip and curl (run it in a container with --cap-add NET_ADMIN).
func TestAgentEgressFloorRefusesMetadataAsRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	for _, bin := range []string{"iptables", "ip", "curl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("needs %s", bin)
		}
	}
	const meta = "169.254.169.254"
	if out, err := exec.Command("ip", "addr", "add", meta+"/32", "dev", "lo").CombinedOutput(); err != nil {
		t.Skipf("cannot put %s on lo (needs CAP_NET_ADMIN): %v: %s", meta, err, out)
	}
	t.Cleanup(func() { _ = exec.Command("ip", "addr", "del", meta+"/32", "dev", "lo").Run() })
	ln, err := net.Listen("tcp", meta+":80")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("iam/security-credentials"))
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })

	f := newIsolationRootFixture(t) // the start-up self-test, through the real Bash shim
	p := f.priv
	removeEgressFloor(t, p)
	t.Cleanup(func() { removeEgressFloor(t, p) })
	t.Setenv(egressFilterEnv, "")
	t.Setenv(egressAllowEnv, "")

	curlAs := func(uid uint32) error {
		c := exec.Command("curl", "-sS", "-m", "3", "http://"+meta+"/")
		if uid != 0 {
			c.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: uid, Groups: []uint32{}}}
		}
		c.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/"}
		out, err := c.CombinedOutput()
		if err == nil && !strings.Contains(string(out), "security-credentials") {
			t.Fatalf("uid %d: unexpected answer %q", uid, out)
		}
		return err
	}
	if err := curlAs(p.UID); err != nil {
		t.Fatalf("before the floor the executor should reach the test listener, so the check below means something: %v", err)
	}
	targets := f.targets()
	targets.MetadataDial = []string{meta + ":80"}
	targets.EgressFloor = "the agent egress floor is not installed: test"
	err = IsolationSelfTest(p, targets)
	if err == nil || !strings.Contains(err.Error(), "Bash connected to the instance-metadata address "+meta+":80") ||
		!strings.Contains(err.Error(), AllowMetadataEgressEnv) {
		t.Fatalf("self-test without the floor, metadata answering: %v", err)
	}
	if _, err := EnforceIsolation("linux", p, err, false); err == nil {
		t.Fatal("a Linux root start went on with the metadata address reachable by the executor")
	}

	for range 2 { // a second start converges on the same rules
		st := installAgentEgressFloor("linux", p, ownerfw.ExecRunner)
		if len(st.Installed) == 0 || st.Installed[0] != ownerfw.IPv4 {
			t.Fatalf("floor not installed: %s", st)
		}
	}
	out, err := exec.Command("iptables", "-w", "-S", "OUTPUT").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(out), "-j "+egressFloorChain); n != 2 {
		t.Fatalf("want one OUTPUT jump per owner (2), got %d:\n%s", n, out)
	}

	if err := curlAs(p.UID); err == nil {
		t.Fatalf("curl %s as uid %d succeeded with the floor installed", meta, p.UID)
	}
	if err := curlAs(p.mcpUID()); err == nil {
		t.Fatalf("curl %s as the MCP uid %d succeeded with the floor installed", meta, p.mcpUID())
	}
	if err := curlAs(0); err != nil {
		t.Fatalf("root (the server) must stay unfiltered: %v", err)
	}
	targets.EgressFloor = "installed"
	if err := IsolationSelfTest(p, targets); err != nil {
		t.Fatalf("self-test with the floor installed: %v", err)
	}

}

// removeEgressFloor deletes the floor chain and its OUTPUT jumps for p's owners.
func removeEgressFloor(t *testing.T, p ExecPrivilege) {
	t.Helper()
	r, err := agentEgressRules(p, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, bin := range []string{"iptables", "ip6tables"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		for _, o := range r.Owners {
			for exec.Command(bin, "-w", "-D", "OUTPUT", "-m", "owner", "--uid-owner", o, "-j", egressFloorChain).Run() == nil {
			}
		}
		_ = exec.Command(bin, "-w", "-F", egressFloorChain).Run()
		_ = exec.Command(bin, "-w", "-X", egressFloorChain).Run()
	}
}
