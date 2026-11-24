package ownerfw

import (
	"context"
	"errors"
	"net/netip"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func testRules() Rules {
	return Rules{
		Chain:     "T_FLOOR",
		Owners:    []string{"10001", "20000-29999"},
		Pass:      []netip.Prefix{netip.MustParsePrefix("169.254.10.0/24"), netip.MustParsePrefix("fe80::1/128")},
		PassPorts: []Port{{Proto: "udp", Port: 53}},
		Refuse:    []netip.Prefix{netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("fd00:ec2::254/128")},
	}
}

func joined(p Plan) []string {
	var out []string
	for _, s := range p.Steps {
		line := strings.Join(s.Args, " ")
		if s.Fallback != nil {
			line += " || " + strings.Join(s.Fallback, " ")
		}
		out = append(out, line)
	}
	return out
}

func TestPlanPerFamily(t *testing.T) {
	got := joined(testRules().Plan(IPv4))
	want := []string{
		"-w -N T_FLOOR || -w -F T_FLOOR",
		"-w -C OUTPUT -m owner --uid-owner 10001 -j T_FLOOR || -w -I OUTPUT 1 -m owner --uid-owner 10001 -j T_FLOOR",
		"-w -C OUTPUT -m owner --uid-owner 20000-29999 -j T_FLOOR || -w -I OUTPUT 1 -m owner --uid-owner 20000-29999 -j T_FLOOR",
		"-w -A T_FLOOR -p udp --dport 53 -j RETURN",
		"-w -A T_FLOOR -d 169.254.10.0/24 -j RETURN",
		"-w -A T_FLOOR -d 169.254.0.0/16 -j REJECT",
		"-w -A T_FLOOR -m conntrack --ctorigdst 169.254.0.0/16 -j REJECT",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ipv4 plan:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	v6 := joined(testRules().Plan(IPv6))
	if v6[len(v6)-2] != "-w -A T_FLOOR -d fd00:ec2::254/128 -j REJECT" || v6[len(v6)-3] != "-w -A T_FLOOR -d fe80::1/128 -j RETURN" {
		t.Fatalf("ipv6 plan: %v", v6)
	}
	if testRules().Plan(IPv6).Family.Binary() != "ip6tables" || IPv4.Binary() != "iptables" {
		t.Fatal("binaries")
	}
}

func TestValidate(t *testing.T) {
	if err := testRules().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Rules{
		{Chain: "", Owners: []string{"1"}},
		{Chain: strings.Repeat("X", 29), Owners: []string{"1"}},
		{Chain: "C"},
		{Chain: "C", Owners: []string{"010001"}}, // octal to iptables
		{Chain: "C", Owners: []string{"20-10"}},
		{Chain: "C", Owners: []string{"-5"}},
		{Chain: "C", Owners: []string{"a"}},
		{Chain: "C", Owners: []string{"1"}, PassPorts: []Port{{Proto: "icmp", Port: 1}}},
	} {
		if bad.Validate() == nil {
			t.Errorf("%+v validated", bad)
		}
	}
}

func TestApplyFallbackAndStop(t *testing.T) {
	var calls []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		line := name + " " + strings.Join(args, " ")
		calls = append(calls, line)
		switch {
		case slices.Contains(args, "-N"): // the chain exists: flush instead
			return []byte("chain exists"), errors.New("exit 1")
		case slices.Contains(args, "-C") && slices.Contains(args, "10001"): // jump present
			return nil, nil
		case slices.Contains(args, "-C"):
			return nil, errors.New("exit 1")
		case slices.Contains(args, "fd00:ec2::254/128"):
			return []byte("no such target"), errors.New("exit 2")
		}
		return nil, nil
	}
	if err := Apply(context.Background(), run, testRules().Plan(IPv4)); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "iptables -w -F T_FLOOR") || slices.Contains(calls, "iptables -w -I OUTPUT 1 -m owner --uid-owner 10001 -j T_FLOOR") ||
		!slices.Contains(calls, "iptables -w -I OUTPUT 1 -m owner --uid-owner 20000-29999 -j T_FLOOR") {
		t.Fatalf("calls: %v", calls)
	}
	err := Apply(context.Background(), run, testRules().Plan(IPv6))
	if err == nil || !strings.Contains(err.Error(), "no such target") {
		t.Fatalf("ipv6 err = %v", err)
	}
}

func TestApplyNoBinary(t *testing.T) {
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, exec.ErrNotFound }
	if err := Apply(context.Background(), run, testRules().Plan(IPv4)); !errors.Is(err, ErrNoBinary) {
		t.Fatalf("err = %v", err)
	}
}
