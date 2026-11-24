package server

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/ownerfw"
)

func TestAgentEgressRulesOwners(t *testing.T) {
	p := ExecPrivilege{Enabled: true, UID: 10001, GID: 10001, Range: ExecUIDRange{Lo: 20000, Hi: 29999}}
	r, bad, err := agentEgressRules(p, "10.9.0.0/16, 169.254.10.5 fd00::1", "")
	if err != nil || bad != nil {
		t.Fatal(err, bad)
	}
	if !slices.Equal(r.Owners, []string{"10001", "10002", "20000-29999"}) || r.Tag != egressFloorTag {
		t.Fatalf("owners %v tag %q", r.Owners, r.Tag)
	}
	if len(r.Pass) != 3 || r.Pass[1].String() != "169.254.10.5/32" || r.Pass[2].String() != "fd00::1/128" {
		t.Fatalf("pass %v", r.Pass)
	}
	if len(r.PassPorts) != 0 {
		t.Fatalf("DNS passes with no refused nameserver: %v", r.PassPorts)
	}
	p.MCPUID, p.Range = 30000, ExecUIDRange{}
	if r, _, _ := agentEgressRules(p, "", ""); !slices.Equal(r.Owners, []string{"10001", "30000"}) {
		t.Fatalf("owners %v", r.Owners)
	}
	// A bad entry is skipped, not a reason to drop the floor.
	if r, bad, err := agentEgressRules(p, "not-an-ip, 10.0.0.1", ""); err != nil || len(r.Pass) != 1 || len(r.Refuse) == 0 || !slices.Equal(bad, []string{"not-an-ip"}) {
		t.Fatalf("rules %+v, bad %v, err %v", r, bad, err)
	}
	if h := metadataReachedHint(EgressFloorStatus{Installed: []ownerfw.Family{ownerfw.IPv4}}); !strings.Contains(h, "another rule") || strings.Contains(h, "Install iptables") {
		t.Fatalf("hint with the floor installed: %s", h)
	}
}

// DNS passes only to the resolv.conf nameservers that are in a refused range.
func TestAgentEgressRulesDNS(t *testing.T) {
	p := ExecPrivilege{Enabled: true, UID: 10001, GID: 10001}
	resolv := "# comment\nnameserver 169.254.169.253\nnameserver 10.0.0.2\nnameserver fe80::1%eth0\nnameserver 169.254.169.253\nsearch x\n"
	r, _, err := agentEgressRules(p, "", resolv)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, port := range r.PassPorts {
		got = append(got, port.Proto+" "+port.To.String())
	}
	want := []string{"udp 169.254.169.253/32", "tcp 169.254.169.253/32", "udp fe80::1/128", "tcp fe80::1/128"}
	if !slices.Equal(got, want) {
		t.Fatalf("pass ports %v, want %v", got, want)
	}
}

// fakeIPTables answers -S with an empty OUTPUT and an existing chain, and
// records every call.
type fakeIPTables struct{ calls []string }

func (f *fakeIPTables) run(_ context.Context, stdin, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " ")+"\n"+stdin)
	if args[len(args)-1] == "OUTPUT" {
		return []byte("-P OUTPUT ACCEPT"), nil
	}
	if strings.HasSuffix(name, "-restore") {
		return nil, nil
	}
	return []byte("-N " + args[len(args)-1]), nil
}

func (f *fakeIPTables) has(s string) bool {
	return slices.ContainsFunc(f.calls, func(c string) bool { return strings.Contains(c, s) })
}

func TestInstallAgentEgressFloor(t *testing.T) {
	p := ExecPrivilege{Enabled: true, UID: 10001, GID: 10001}
	resolvConfPath = filepath.Join(t.TempDir(), "resolv.conf")
	t.Cleanup(func() { resolvConfPath = "/etc/resolv.conf" })
	fake := &fakeIPTables{}
	t.Setenv(egressFilterEnv, "")
	t.Setenv(egressAllowEnv, "")
	f := installAgentEgressFloor("linux", p, fake.run)
	if st := f.Status; len(st.Installed) != 2 || st.Problem != "" || len(f.keepers) != 2 {
		t.Fatalf("status %+v", st)
	}
	for _, want := range []string{
		"iptables-restore -w --noflush\n*filter\n:" + egressFloorChain + " - [0:0]\n",
		"-A " + egressFloorChain + " -d 169.254.0.0/16 -j REJECT\n",
		"-I OUTPUT 1 -m owner --uid-owner 10002 -m comment --comment " + egressFloorTag + " -j " + egressFloorChain + "\n",
		"ip6tables-restore -w --noflush\n",
		"-A " + egressFloorChain + " -d fd00:ec2::254/128 -j REJECT\n",
	} {
		if !fake.has(want) {
			t.Errorf("missing %q in %v", want, fake.calls)
		}
	}
	// The fake's OUTPUT never holds the jumps: each check sees that and re-applies.
	before := len(fake.calls)
	if d := f.check(context.Background()); len(d) != 2 || !strings.Contains(d[0], "ipv4: OUTPUT rule 1") || !fake.has("ip6tables-restore") || len(fake.calls) <= before {
		t.Fatalf("check: %v", d)
	}
	// Ipv6 refused: ipv4 still counts, the problem says so.
	no6 := func(ctx context.Context, stdin, name string, args ...string) ([]byte, error) {
		if strings.HasPrefix(name, "ip6tables") {
			return []byte("can't initialize ip6tables"), errors.New("exit 3")
		}
		return fake.run(ctx, stdin, name, args...)
	}
	if f := installAgentEgressFloor("linux", p, no6); len(f.Status.Installed) != 1 || len(f.keepers) != 1 || !strings.Contains(f.Status.String(), "ipv6 rules not installed") {
		t.Fatalf("status %+v", f.Status)
	}
	fail := func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte("Permission denied"), errors.New("exit 4")
	}
	if f := installAgentEgressFloor("linux", p, fail); len(f.Status.Installed) != 0 || len(f.keepers) != 0 || !strings.Contains(f.Status.String(), "CAP_NET_ADMIN") {
		t.Fatalf("status %+v", f.Status)
	}
	never := func(context.Context, string, string, ...string) ([]byte, error) {
		t.Fatal("iptables ran")
		return nil, nil
	}
	if f := installAgentEgressFloor("darwin", p, never); !strings.Contains(f.Status.Problem, "needs Linux") {
		t.Fatalf("status %+v", f.Status)
	}
	// Off on Linux: a floor an earlier start left is removed, and the status says so.
	for _, c := range []struct {
		p    ExecPrivilege
		env  string
		want string
	}{
		{ExecPrivilege{}, "", "privilege drop is off"},
		{p, "0", egressFilterEnv + "=0"},
	} {
		t.Setenv(egressFilterEnv, c.env)
		fake := &fakeIPTables{}
		f := installAgentEgressFloor("linux", c.p, fake.run)
		if len(f.Status.Installed) != 0 || !strings.Contains(f.Status.Problem, c.want) ||
			!strings.Contains(f.Status.Problem, "removed the ipv4 floor of an earlier start") ||
			!fake.has("-X "+egressFloorChain+"\n") {
			t.Errorf("%s: %+v %v", c.want, f.Status, fake.calls)
		}
		if st := installAgentEgressFloor("linux", c.p, fail); strings.Contains(st.Status.Problem, "removed") {
			t.Errorf("%s, iptables unreadable: %+v", c.want, st.Status)
		}
	}
}

func TestMetadataProbeAddrsOptOut(t *testing.T) {
	t.Setenv(AllowMetadataEgressEnv, "")
	if got := MetadataProbeAddrs(); !slices.Contains(got, "169.254.169.254:80") {
		t.Fatalf("addrs %v", got)
	}
	t.Setenv(AllowMetadataEgressEnv, "1")
	if got := MetadataProbeAddrs(); got != nil {
		t.Fatalf("opted out, addrs %v", got)
	}
}

// The probe's dial check: an address that accepts is reported, a closed one is not.
func TestDialableAddrs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddr := closed.Addr().String()
	closed.Close()
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	got := dialableAddrs([]string{closedAddr, ln.Addr().String()})
	if !slices.Equal(got, []string{ln.Addr().String()}) {
		t.Fatalf("dialable %v", got)
	}
	spec := isolationSpec(ExecPrivilege{Enabled: true, UID: 10001, GID: 10001},
		IsolationTargets{MetadataDial: []string{"169.254.169.254:80"}, EgressFloor: EgressFloorStatus{Problem: "x"}})
	if !slices.Equal(spec.NoDial, []string{"169.254.169.254:80"}) || !strings.Contains(spec.NoDialHint, AllowMetadataEgressEnv) ||
		!strings.Contains(spec.NoDialHint, "not installed: x") || !strings.Contains(spec.NoDialHint, "NET_ADMIN") {
		t.Fatalf("spec %+v", spec)
	}
}
