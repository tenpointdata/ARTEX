package server

import (
	"path/filepath"
	"testing"
)

func TestParseExecUIDRange(t *testing.T) {
	for _, c := range []struct {
		in   string
		want ExecUIDRange
		ok   bool
	}{
		{"", ExecUIDRange{}, true},
		{"20000-29999", ExecUIDRange{20000, 29999}, true},
		{" 5000 - 5000 ", ExecUIDRange{5000, 5000}, true},
		{"42-2000", ExecUIDRange{}, false},
		{"0-10", ExecUIDRange{}, false},
		{"10-5", ExecUIDRange{}, false},
		{"20000", ExecUIDRange{}, false},
		{"a-b", ExecUIDRange{}, false},
		{"1-2147483648", ExecUIDRange{}, false},
	} {
		got, err := ParseExecUIDRange(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("ParseExecUIDRange(%q) = %v, %v; want %v ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
	if s := (ExecUIDRange{20000, 29999}).String(); s != "20000-29999" {
		t.Errorf("String() = %q", s)
	}
}

func TestWithUIDRangeRefusesOverlaps(t *testing.T) {
	p := ExecPrivilege{Enabled: true, UID: 10001, GID: 10001}
	for _, r := range []ExecUIDRange{{10001, 10001}, {10002, 10010}, {9000, 10001}} {
		if _, err := p.WithUIDRange(r); err == nil {
			t.Errorf("range %s overlapping the executor or MCP ids was accepted", r)
		}
	}
	g := ExecPrivilege{Enabled: true, UID: 10001, GID: 25000}
	if _, err := g.WithUIDRange(ExecUIDRange{20000, 29999}); err == nil {
		t.Error("a range holding the executor gid was accepted: its task would share the executor group")
	}
	got, err := p.WithUIDRange(ExecUIDRange{20000, 29999})
	if err != nil || got.Range != (ExecUIDRange{20000, 29999}) {
		t.Fatalf("WithUIDRange = %+v, %v", got, err)
	}
	if off, err := (ExecPrivilege{}).WithUIDRange(ExecUIDRange{20000, 29999}); err != nil || off.Range.Enabled() {
		t.Errorf("a disabled privilege took a range: %+v, %v", off, err)
	}
}

func TestExecUIDPool(t *testing.T) {
	p := newExecUIDPool(ExecUIDRange{100, 102})
	a, fresh, err := p.acquire("tasks/1")
	if err != nil || !fresh || a != 100 {
		t.Fatalf("first acquire = %d %v %v", a, fresh, err)
	}
	if again, fresh, _ := p.acquire("tasks/1"); again != a || fresh {
		t.Fatalf("acquire is not idempotent: %d %v", again, fresh)
	}
	b, _, _ := p.acquire("sessions/x")
	c, _, _ := p.acquire("tasks/2")
	if b != 101 || c != 102 {
		t.Fatalf("round robin: %d %d", b, c)
	}
	if _, _, err := p.acquire("tasks/3"); err == nil {
		t.Fatal("an exhausted range handed out a uid")
	}
	if uid, ok := p.release("tasks/1"); !ok || uid != 100 {
		t.Fatalf("release = %d %v", uid, ok)
	}
	if _, ok := p.release("tasks/1"); ok {
		t.Fatal("double release")
	}
	if d, _, _ := p.acquire("tasks/3"); d != 100 {
		t.Fatalf("the freed uid was not reused: %d", d)
	}

	q := newExecUIDPool(ExecUIDRange{100, 109})
	if !q.adopt("tasks/7", 105) || !q.adopt("tasks/7", 105) {
		t.Fatal("adopt of a free uid failed")
	}
	if q.adopt("tasks/8", 105) || q.adopt("tasks/7", 106) || q.adopt("tasks/9", 99) {
		t.Fatal("adopt accepted a taken uid, a second uid for a key or a uid outside the range")
	}
	if next, _, _ := q.acquire("tasks/10"); next != 106 {
		t.Fatalf("allocation after adopt starts at %d, want 106", next)
	}
}

func TestRunKeyOf(t *testing.T) {
	base := filepath.FromSlash("/data")
	for leaf, want := range map[string]string{
		"/data/tasks/12/main":   "tasks/12",
		"/data/tasks/12/i3":     "tasks/12",
		"/data/sessions/conv-4": "sessions/conv-4",
	} {
		if got, err := runKeyOf(base, filepath.FromSlash(leaf)); err != nil || got != want {
			t.Errorf("runKeyOf(%s) = %q, %v; want %q", leaf, got, err, want)
		}
	}
	for _, leaf := range []string{"/data/tasks/12", "/data/tasks/x/main", "/other/tasks/1/main", "/data/sessions"} {
		if got, err := runKeyOf(base, filepath.FromSlash(leaf)); err == nil {
			t.Errorf("runKeyOf(%s) = %q, want an error", leaf, got)
		}
	}
}
