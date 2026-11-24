//go:build linux

package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Autumn-27/artex/ownerfw"
)

// Root tests of keeping the floor (egress_floor_keep.go): re-check while
// running, atomic rebuild, stale jumps, the filter turned off, and DNS only to a
// resolver in a refused range. Each puts the addresses it dials on lo with a
// listener behind them, so a refusal is the floor's, not a missing route. Needs
// root, CAP_NET_ADMIN, iptables (with iptables-restore), ip and curl.

// floorRootFixture is an executor privilege with the floor removed before and
// after the test.
func floorRootFixture(t *testing.T) ExecPrivilege {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	for _, bin := range []string{"iptables", "iptables-restore", "ip", "curl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("needs %s", bin)
		}
	}
	p := newExecRootFixture(t).priv
	removeEgressFloor(t, p)
	t.Cleanup(func() { removeEgressFloor(t, p) })
	t.Setenv(egressFilterEnv, "")
	t.Setenv(egressAllowEnv, "")
	resolvConfPath = filepath.Join(t.TempDir(), "resolv.conf")
	t.Cleanup(func() { resolvConfPath = "/etc/resolv.conf" })
	return p
}

// serveOnLo puts ip on lo and answers HTTP on ip:port.
func serveOnLo(t *testing.T, ip, port string) {
	t.Helper()
	if out, err := exec.Command("ip", "addr", "add", ip+"/32", "dev", "lo").CombinedOutput(); err != nil && !strings.Contains(string(out), "exists") {
		t.Skipf("cannot put %s on lo (needs CAP_NET_ADMIN): %v: %s", ip, err, out)
	}
	t.Cleanup(func() { _ = exec.Command("ip", "addr", "del", ip+"/32", "dev", "lo").Run() })
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, port))
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("meta-ok")) })}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
}

// reaches reports whether uid gets an answer from url.
func reaches(t *testing.T, uid uint32, url string) bool {
	t.Helper()
	c := exec.Command("curl", "-sS", "-m", "3", url)
	c.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: uid, Groups: []uint32{}}}
	c.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/"}
	out, err := c.CombinedOutput()
	return err == nil && strings.Contains(string(out), "meta-ok")
}

func ipt(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("iptables", append([]string{"-w"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("iptables %v: %v: %s", args, err, out)
	}
}

func outputRules(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("iptables", "-w", "-S", "OUTPUT").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// A firewall reload that flushes the chain or the jumps, or a rule inserted
// above the jumps, opens the metadata address to the executor; one check puts
// the floor back.
func TestAgentEgressFloorReappliedWhileRunningAsRoot(t *testing.T) {
	p := floorRootFixture(t)
	const url = "http://169.254.169.254/"
	serveOnLo(t, "169.254.169.254", "80")
	f := installAgentEgressFloor("linux", p, ownerfw.ExecRunner)
	if len(f.Status.Installed) == 0 {
		t.Fatalf("floor not installed: %s", f.Status)
	}
	if reaches(t, p.UID, url) {
		t.Fatal("executor reaches metadata with the floor installed")
	}
	if d := f.check(context.Background()); d != nil {
		t.Fatalf("drift right after install: %v", d)
	}
	jump := []string{"OUTPUT", "-m", "owner", "--uid-owner", execIDString(p.UID), "-m", "comment", "--comment", egressFloorTag, "-j", egressFloorChain}
	for _, c := range []struct {
		name    string
		breakIt func()
		want    string
	}{
		{"chain flushed", func() { ipt(t, "-F", egressFloorChain) }, "ipv4: chain " + egressFloorChain + " differs"},
		{"accept inserted above", func() { ipt(t, "-I", "OUTPUT", "1", "-j", "ACCEPT") }, "ipv4: OUTPUT rule 1 is not the jump"},
		{"jump deleted", func() { ipt(t, append([]string{"-D"}, jump...)...) }, "ipv4: OUTPUT rule 1 jumps to " + egressFloorChain + " for uid \"10002\""},
		{"OUTPUT flushed", func() { ipt(t, "-F", "OUTPUT") }, "ipv4: OUTPUT rule 1 is not the jump"},
	} {
		c.breakIt()
		if !reaches(t, p.UID, url) {
			t.Fatalf("%s: the executor should reach metadata now, so the check below means something", c.name)
		}
		d := f.check(context.Background())
		if len(d) == 0 || !strings.HasPrefix(d[0], c.want) {
			t.Fatalf("%s: drift %v, want %q", c.name, d, c.want)
		}
		if reaches(t, p.UID, url) {
			t.Fatalf("%s: executor reaches metadata after the floor was re-applied", c.name)
		}
		if d := f.check(context.Background()); d != nil {
			t.Fatalf("%s: drift after re-apply: %v", c.name, d)
		}
	}
	_ = exec.Command("iptables", "-w", "-D", "OUTPUT", "-j", "ACCEPT").Run()
}

// Rebuilding a floor already in force never leaves the executor a moment with
// the chain empty: after every iptables call of a rebuild, the metadata address
// is still refused.
func TestAgentEgressFloorRebuildIsAtomicAsRoot(t *testing.T) {
	p := floorRootFixture(t)
	const url = "http://169.254.169.254/"
	serveOnLo(t, "169.254.169.254", "80")
	if f := installAgentEgressFloor("linux", p, ownerfw.ExecRunner); len(f.Status.Installed) == 0 {
		t.Fatalf("floor not installed: %s", f.Status)
	}
	calls := 0
	checked := func(ctx context.Context, stdin, name string, args ...string) ([]byte, error) {
		out, err := ownerfw.ExecRunner(ctx, stdin, name, args...)
		calls++
		if reaches(t, p.UID, url) {
			t.Errorf("after %s %v the executor reached metadata", name, args)
		}
		return out, err
	}
	for i, allow := range []string{"", "10.99.0.0/16", ""} { // a restart, then one with changed rules, then back
		t.Setenv(egressAllowEnv, allow)
		f := installAgentEgressFloor("linux", p, checked)
		if len(f.Status.Installed) == 0 {
			t.Fatalf("rebuild %d: %s", i, f.Status)
		}
	}
	if calls == 0 {
		t.Fatal("no iptables call was checked")
	}
}

// Jumps of an earlier start for owners that are gone (an old uid range, an old
// MCP uid, with or without the tag) are removed; with the filter off the whole
// floor is removed and the status says so.
func TestAgentEgressFloorStaleJumpsAsRoot(t *testing.T) {
	p := floorRootFixture(t)
	ipt(t, "-N", egressFloorChain)
	ipt(t, "-I", "OUTPUT", "1", "-m", "owner", "--uid-owner", "30000-30009", "-j", egressFloorChain)
	ipt(t, "-A", "OUTPUT", "-m", "owner", "--uid-owner", "4242", "-m", "comment", "--comment", egressFloorTag, "-j", egressFloorChain)
	f := installAgentEgressFloor("linux", p, ownerfw.ExecRunner)
	if len(f.Status.Installed) == 0 {
		t.Fatalf("floor not installed: %s", f.Status)
	}
	out := outputRules(t)
	if strings.Contains(out, "30000-30009") || strings.Contains(out, "4242") {
		t.Fatalf("stale jumps kept:\n%s", out)
	}
	if n := strings.Count(out, "-j "+egressFloorChain); n != 2 || !strings.Contains(out, "--comment "+egressFloorTag) {
		t.Fatalf("want 2 tagged jumps:\n%s", out)
	}

	t.Setenv(egressFilterEnv, "0")
	off := installAgentEgressFloor("linux", p, ownerfw.ExecRunner)
	if len(off.Status.Installed) != 0 || !strings.Contains(off.Status.Problem, "removed the ipv4 floor of an earlier start (2 OUTPUT jumps)") {
		t.Fatalf("filter off: %+v", off.Status)
	}
	if out := outputRules(t); strings.Contains(out, egressFloorChain) {
		t.Fatalf("jumps left with the filter off:\n%s", out)
	}
	if exec.Command("iptables", "-w", "-S", egressFloorChain).Run() == nil {
		t.Fatal("chain left with the filter off")
	}
	if again := installAgentEgressFloor("linux", p, ownerfw.ExecRunner); strings.Contains(again.Status.Problem, "removed") {
		t.Fatalf("second off start: %+v", again.Status)
	}
}

// Port 53 passes only to the resolv.conf nameserver in a refused range, not to
// every refused address.
func TestAgentEgressFloorDNSOnlyToResolverAsRoot(t *testing.T) {
	p := floorRootFixture(t)
	serveOnLo(t, "169.254.169.253", "53")
	serveOnLo(t, "169.254.169.254", "53")
	if err := os.WriteFile(resolvConfPath, []byte("nameserver 169.254.169.253\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := installAgentEgressFloor("linux", p, ownerfw.ExecRunner)
	if len(f.Status.Installed) == 0 {
		t.Fatalf("floor not installed: %s", f.Status)
	}
	if !reaches(t, p.UID, "http://169.254.169.253:53/") {
		t.Fatal("executor refused the resolver's port 53")
	}
	if reaches(t, p.UID, "http://169.254.169.254:53/") {
		t.Fatal("executor reached port 53 of a refused address that is not a resolver")
	}
	// The resolver changes (DHCP): the next check follows it.
	if err := os.WriteFile(resolvConfPath, []byte("nameserver 169.254.169.254\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d := f.check(context.Background()); len(d) == 0 || !strings.HasSuffix(d[0], ownerfw.DriftRulesChanged) {
		t.Fatalf("drift %v", d)
	}
	if reaches(t, p.UID, "http://169.254.169.253:53/") || !reaches(t, p.UID, "http://169.254.169.254:53/") {
		t.Fatal("the floor did not follow the new resolver")
	}
}

// A start whose install failed (here a held lock, simulated) is retried by the
// next check, and the executor is refused from then on.
func TestAgentEgressFloorFailedStartRetriedAsRoot(t *testing.T) {
	p := floorRootFixture(t)
	const url = "http://169.254.169.254/"
	serveOnLo(t, "169.254.169.254", "80")
	locked := func(ctx context.Context, stdin, name string, args ...string) ([]byte, error) {
		if strings.HasSuffix(name, "-restore") {
			return []byte("Another app is currently holding the xtables lock."), errors.New("exit 4")
		}
		return ownerfw.ExecRunner(ctx, stdin, name, args...)
	}
	f := installAgentEgressFloor("linux", p, locked)
	if len(f.Status.Installed) != 0 || len(f.keepers) != 2 || !reaches(t, p.UID, url) {
		t.Fatalf("failed start: %+v, %d keepers", f.Status, len(f.keepers))
	}
	for _, k := range f.keepers {
		k.Run = ownerfw.ExecRunner // the lock is free again
	}
	if d := f.check(context.Background()); len(d) == 0 || !strings.HasSuffix(d[0], ownerfw.DriftNotInstalled) {
		t.Fatalf("retry: %v", d)
	}
	if reaches(t, p.UID, url) {
		t.Fatal("executor reaches metadata after the retry installed the floor")
	}
}
