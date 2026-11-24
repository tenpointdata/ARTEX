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
		Chain:  "T_FLOOR",
		Tag:    "t-floor",
		Owners: []string{"10001", "20000-29999"},
		Pass:   []netip.Prefix{netip.MustParsePrefix("169.254.10.0/24"), netip.MustParsePrefix("fe80::1/128")},
		PassPorts: []Port{
			{Proto: "udp", Port: 53, To: netip.MustParsePrefix("169.254.169.253/32")},
			{Proto: "tcp", Port: 853},
		},
		Refuse: []netip.Prefix{netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("fd00:ec2::254/128")},
	}
}

func TestRestoreInput(t *testing.T) {
	output := []string{
		"-P OUTPUT ACCEPT",
		"-A OUTPUT -j ACCEPT",
		"-A OUTPUT -m owner --uid-owner 30000-30009 -j T_FLOOR", // stale, untagged
		"-A OUTPUT -m owner --uid-owner 10001 -m comment --comment \"t-floor\" -j T_FLOOR",
		"-A OUTPUT -m owner --uid-owner 10001 -j OTHER",
	}
	got := strings.Split(strings.TrimSpace(testRules().restoreInput(IPv4, output)), "\n")
	want := []string{
		"*filter",
		":T_FLOOR - [0:0]",
		"-D OUTPUT -m owner --uid-owner 30000-30009 -j T_FLOOR",
		"-D OUTPUT -m owner --uid-owner 10001 -m comment --comment \"t-floor\" -j T_FLOOR",
		"-I OUTPUT 1 -m owner --uid-owner 20000-29999 -m comment --comment t-floor -j T_FLOOR",
		"-I OUTPUT 1 -m owner --uid-owner 10001 -m comment --comment t-floor -j T_FLOOR",
		"-A T_FLOOR -d 169.254.169.253/32 -p udp --dport 53 -j RETURN",
		"-A T_FLOOR -p tcp --dport 853 -j RETURN",
		"-A T_FLOOR -d 169.254.10.0/24 -j RETURN",
		"-A T_FLOOR -d 169.254.0.0/16 -j REJECT",
		"-A T_FLOOR -m conntrack --ctorigdst 169.254.0.0/16 -j REJECT",
		"COMMIT",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ipv4:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	v6 := testRules().restoreInput(IPv6, nil)
	for _, l := range []string{"-A T_FLOOR -d fe80::1/128 -j RETURN", "-A T_FLOOR -d fd00:ec2::254/128 -j REJECT", "-A T_FLOOR -p tcp --dport 853 -j RETURN"} {
		if !strings.Contains(v6, l+"\n") {
			t.Errorf("ipv6 lacks %q:\n%s", l, v6)
		}
	}
	if strings.Contains(v6, "169.254") {
		t.Errorf("ipv6 carries ipv4 rules:\n%s", v6)
	}
	if rm := removeInput("T_FLOOR", output, true); rm != "*filter\n-D OUTPUT -m owner --uid-owner 30000-30009 -j T_FLOOR\n"+
		"-D OUTPUT -m owner --uid-owner 10001 -m comment --comment \"t-floor\" -j T_FLOOR\n-F T_FLOOR\n-X T_FLOOR\nCOMMIT\n" {
		t.Fatalf("remove:\n%s", rm)
	}
	if IPv6.RestoreBinary() != "ip6tables-restore" || IPv4.Binary() != "iptables" || IPv6.String() != "ipv6" {
		t.Fatal("binaries")
	}
}

func TestJumpDrift(t *testing.T) {
	r := testRules()
	j := func(o string) string { return "-A OUTPUT " + r.jumpSpec(o) }
	for _, c := range []struct {
		output []string
		want   string
	}{
		{[]string{"-P OUTPUT ACCEPT", j("10001"), j("20000-29999"), "-A OUTPUT -j ACCEPT"}, ""},
		{[]string{"-P OUTPUT ACCEPT", "-A OUTPUT -j ACCEPT", j("10001"), j("20000-29999")}, "rule 1 is not the jump"},
		{[]string{j("20000-29999"), j("10001")}, "for uid \"20000-29999\", not 10001"},
		{[]string{j("10001")}, "rule 2 is not the jump"},
		{[]string{j("10001"), j("20000-29999"), j("30000")}, "rule 3 is an extra jump"},
		{nil, "rule 1 is not the jump"},
	} {
		if got := jumpDrift(r, c.output); c.want == "" && got != "" || !strings.Contains(got, c.want) {
			t.Errorf("%v: drift %q, want %q", c.output, got, c.want)
		}
	}
}

func TestFields(t *testing.T) {
	got := fields(`-A OUTPUT -m comment --comment "a b \"c\"" -j X`)
	if !slices.Equal(got, []string{"-A", "OUTPUT", "-m", "comment", "--comment", `a b "c"`, "-j", "X"}) {
		t.Fatalf("%q", got)
	}
	if !isJumpTo("-A OUTPUT --jump X", "X") || isJumpTo("-A X -j X", "X") || isJumpTo("-A OUTPUT -j XY", "X") {
		t.Fatal("isJumpTo")
	}
}

func TestValidate(t *testing.T) {
	if err := testRules().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Rules{
		{Chain: "", Tag: "t", Owners: []string{"1"}},
		{Chain: strings.Repeat("X", 29), Tag: "t", Owners: []string{"1"}},
		{Chain: "C D", Tag: "t", Owners: []string{"1"}},
		{Chain: "C", Owners: []string{"1"}},
		{Chain: "C", Tag: "a b", Owners: []string{"1"}},
		{Chain: "C", Tag: "t"},
		{Chain: "C", Tag: "t", Owners: []string{"010001"}}, // octal to iptables
		{Chain: "C", Tag: "t", Owners: []string{"20-10"}},
		{Chain: "C", Tag: "t", Owners: []string{"-5"}},
		{Chain: "C", Tag: "t", Owners: []string{"a"}},
		{Chain: "C", Tag: "t", Owners: []string{"1"}, PassPorts: []Port{{Proto: "icmp", Port: 1}}},
	} {
		if bad.Validate() == nil {
			t.Errorf("%+v validated", bad)
		}
	}
}

// fakeTables keeps OUTPUT and one chain as -S lines and applies restore input
// naively, enough to drive a Keeper.
type fakeTables struct {
	output   []string
	chain    []string // nil: no chain
	restores []string
	fail     error
}

func (ft *fakeTables) run(_ context.Context, stdin, name string, args ...string) ([]byte, error) {
	if ft.fail != nil {
		return []byte("Permission denied"), ft.fail
	}
	if strings.HasSuffix(name, "-restore") {
		ft.restores = append(ft.restores, stdin)
		var top []string
		for _, l := range strings.Split(stdin, "\n") {
			switch {
			case strings.HasPrefix(l, ":"):
				ft.chain = []string{"-N " + strings.Fields(l[1:])[0]}
			case strings.HasPrefix(l, "-D OUTPUT"):
				ft.output = slices.DeleteFunc(ft.output, func(o string) bool { return o == "-A OUTPUT"+strings.TrimPrefix(l, "-D OUTPUT") })
			case strings.HasPrefix(l, "-I OUTPUT 1"):
				top = append([]string{"-A OUTPUT" + strings.TrimPrefix(l, "-I OUTPUT 1")}, top...)
			case strings.HasPrefix(l, "-X"):
				ft.chain = nil
			case strings.HasPrefix(l, "-A "):
				ft.chain = append(ft.chain, l)
			}
		}
		ft.output = append(top, ft.output...)
		return nil, nil
	}
	if args[len(args)-1] == "OUTPUT" {
		return []byte(strings.Join(append([]string{"-P OUTPUT ACCEPT"}, ft.output...), "\n")), nil
	}
	if ft.chain == nil {
		return []byte("No chain/target/match by that name."), errors.New("exit 1")
	}
	return []byte(strings.Join(ft.chain, "\n")), nil
}

func TestKeeperDriftAndRemove(t *testing.T) {
	ft := &fakeTables{output: []string{"-A OUTPUT -m owner --uid-owner 4242 -m comment --comment t-floor -j T_FLOOR"}}
	k := &Keeper{Rules: testRules(), Family: IPv4, Run: ft.run}
	ctx := context.Background()
	if d, _ := k.Drift(ctx); d == "" {
		t.Fatal("no drift before the first Apply")
	}
	if err := k.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if len(ft.output) != 2 || strings.Contains(strings.Join(ft.output, " "), "4242") {
		t.Fatalf("stale jump kept: %v", ft.output)
	}
	if d, err := k.Ensure(ctx); d != "" || err != nil || len(ft.restores) != 1 {
		t.Fatalf("drift %q err %v restores %d right after Apply", d, err, len(ft.restores))
	}
	ft.output = append([]string{"-A OUTPUT -j ACCEPT"}, ft.output...) // a rule above the jumps
	if d, err := k.Ensure(ctx); !strings.Contains(d, "rule 1") || err != nil || ft.output[0] == "-A OUTPUT -j ACCEPT" {
		t.Fatalf("drift %q err %v output %v", d, err, ft.output)
	}
	ft.output[0] = strings.Replace(ft.output[0], "-m owner --uid-owner", "-m owner ! --uid-owner", 1) // same uid, negated
	if d, _ := k.Ensure(ctx); !strings.Contains(d, "rule 1 differs") || strings.Contains(ft.output[0], "!") {
		t.Fatalf("drift %q output %v", d, ft.output)
	}
	ft.chain = ft.chain[:1] // flushed by a reload
	if d, _ := k.Ensure(ctx); !strings.Contains(d, "differs") || len(ft.chain) < 3 {
		t.Fatalf("drift %q chain %v", d, ft.chain)
	}
	ft.chain = nil
	if d, _ := k.Ensure(ctx); !strings.Contains(d, "gone") {
		t.Fatalf("drift %q", d)
	}
	k.Rules.Owners = []string{"10001"}
	if d, _ := k.Ensure(ctx); d != DriftRulesChanged || len(ft.output) != 2 {
		t.Fatalf("drift %q output %v", d, ft.output)
	}
	n, existed, err := Remove(ctx, ft.run, IPv4, "T_FLOOR")
	if n != 1 || !existed || err != nil || ft.chain != nil || len(ft.output) != 1 {
		t.Fatalf("remove: %d %v %v; output %v chain %v", n, existed, err, ft.output, ft.chain)
	}
	if n, existed, err := Remove(ctx, ft.run, IPv4, "T_FLOOR"); n != 0 || existed || err != nil {
		t.Fatalf("second remove: %d %v %v", n, existed, err)
	}
	ft.fail = errors.New("exit 4")
	if err := k.Apply(ctx); err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("err %v", err)
	}
	ft.fail = exec.ErrNotFound
	if err := k.Apply(ctx); !errors.Is(err, ErrNoBinary) {
		t.Fatalf("err %v", err)
	}
}
