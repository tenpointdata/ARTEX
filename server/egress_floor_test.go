package server

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"
)

func TestAgentEgressRulesOwners(t *testing.T) {
	p := ExecPrivilege{Enabled: true, UID: 10001, GID: 10001, Range: ExecUIDRange{Lo: 20000, Hi: 29999}}
	r, err := agentEgressRules(p, "10.9.0.0/16, 169.254.10.5 fd00::1")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Owners, []string{"10001", "10002", "20000-29999"}) {
		t.Fatalf("owners %v", r.Owners)
	}
	if len(r.Pass) != 3 || r.Pass[1].String() != "169.254.10.5/32" || r.Pass[2].String() != "fd00::1/128" {
		t.Fatalf("pass %v", r.Pass)
	}
	p.MCPUID, p.Range = 30000, ExecUIDRange{}
	if r, _ := agentEgressRules(p, ""); !slices.Equal(r.Owners, []string{"10001", "30000"}) {
		t.Fatalf("owners %v", r.Owners)
	}
	if _, err := agentEgressRules(p, "not-an-ip"); err == nil || !strings.Contains(err.Error(), egressAllowEnv) {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallAgentEgressFloor(t *testing.T) {
	p := ExecPrivilege{Enabled: true, UID: 10001, GID: 10001}
	var calls []string
	ok := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	t.Setenv(egressFilterEnv, "")
	t.Setenv(egressAllowEnv, "")
	st := installAgentEgressFloor("linux", p, ok)
	if len(st.Installed) != 2 || st.Problem != "" {
		t.Fatalf("status %+v", st)
	}
	for _, want := range []string{
		"iptables -w -A " + egressFloorChain + " -d 169.254.0.0/16 -j REJECT",
		"ip6tables -w -A " + egressFloorChain + " -d fd00:ec2::254/128 -j REJECT",
		"iptables -w -C OUTPUT -m owner --uid-owner 10002 -j " + egressFloorChain,
	} {
		if !slices.Contains(calls, want) {
			t.Errorf("missing %q in %v", want, calls)
		}
	}
	// Ipv6 refused: ipv4 still counts, the problem says so.
	no6 := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "ip6tables" {
			return []byte("can't initialize ip6tables"), errors.New("exit 3")
		}
		return nil, nil
	}
	if st := installAgentEgressFloor("linux", p, no6); len(st.Installed) != 1 || !strings.Contains(st.String(), "ipv6 rules not installed") {
		t.Fatalf("status %+v", st)
	}
	fail := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Permission denied"), errors.New("exit 4")
	}
	if st := installAgentEgressFloor("linux", p, fail); len(st.Installed) != 0 || !strings.Contains(st.String(), "CAP_NET_ADMIN") {
		t.Fatalf("status %+v", st)
	}
	never := func(context.Context, string, ...string) ([]byte, error) { t.Fatal("iptables ran"); return nil, nil }
	for _, c := range []struct {
		goos string
		p    ExecPrivilege
		env  string
		want string
	}{
		{"darwin", p, "", "needs Linux"},
		{"linux", ExecPrivilege{}, "", "privilege drop is off"},
		{"linux", p, "0", egressFilterEnv + "=0"},
	} {
		t.Setenv(egressFilterEnv, c.env)
		if st := installAgentEgressFloor(c.goos, c.p, never); len(st.Installed) != 0 || !strings.Contains(st.Problem, c.want) {
			t.Errorf("%s: %+v", c.want, st)
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
		IsolationTargets{MetadataDial: []string{"169.254.169.254:80"}, EgressFloor: "the agent egress floor is not installed: x"})
	if !slices.Equal(spec.NoDial, []string{"169.254.169.254:80"}) || !strings.Contains(spec.NoDialHint, AllowMetadataEgressEnv) ||
		!strings.Contains(spec.NoDialHint, "not installed: x") {
		t.Fatalf("spec %+v", spec)
	}
}
