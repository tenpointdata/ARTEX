package transcriptsync

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// memBackend is an in-process Backend with the same fencing contract as the
// PostgreSQL one.
type memBackend struct {
	mu     sync.Mutex
	holder map[string]string
	lines  map[string][][]byte
}

func newMem() *memBackend {
	return &memBackend{holder: map[string]string{}, lines: map[string][][]byte{}}
}

func (b *memBackend) Acquire(_ context.Context, s, h string) ([][]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.holder[s] = h
	return append([][]byte(nil), b.lines[s]...), nil
}

func (b *memBackend) Append(_ context.Context, s, h string, from int, lines [][]byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.holder[s] != h || len(b.lines[s]) != from {
		return ErrFenced
	}
	b.lines[s] = append(b.lines[s], lines...)
	return nil
}

func (b *memBackend) stored(s string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(bytes.Join(b.lines[s], []byte{'|'}))
}

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(b)
}

// Host A writes, is taken over by host B (A's lease fenced), B resumes from
// A's lines and continues; A's later lines never reach the store, and when A
// runs the session again it gets B's history.
func TestTakeoverResumesAcrossHosts(t *testing.T) {
	ctx := context.Background()
	be := newMem()
	m := &Mirror{Backend: be, Interval: 1 << 40} // pushes only when asked
	pathA := filepath.Join(t.TempDir(), "s.jsonl")
	pathB := filepath.Join(t.TempDir(), "sub", "s.jsonl")

	a, err := m.Begin(ctx, pathA, "s")
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, pathA, "a1\na2\na3-torn")
	if err := a.Push(ctx); err != nil {
		t.Fatal(err)
	}
	if got := be.stored("s"); got != "a1|a2" {
		t.Fatalf("stored after A push = %q", got)
	}

	b, err := m.Begin(ctx, pathB, "s")
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, pathB); got != "a1\na2\n" {
		t.Fatalf("B local after Begin = %q", got)
	}
	appendFile(t, pathA, "\na4\n") // A is a zombie now
	if err := a.Push(ctx); !errors.Is(err, ErrFenced) {
		t.Fatalf("A push after takeover = %v, want ErrFenced", err)
	}
	if err := a.Close(); !errors.Is(err, ErrFenced) {
		t.Fatalf("A close = %v, want ErrFenced", err)
	}
	appendFile(t, pathB, "b1\n")
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if got := be.stored("s"); got != "a1|a2|b1" {
		t.Fatalf("stored after B = %q", got)
	}

	// A runs the session again: its diverged local file is replaced.
	a2, err := m.Begin(ctx, pathA, "s")
	if err != nil {
		t.Fatal(err)
	}
	defer a2.Close()
	if got := readFile(t, pathA); got != "a1\na2\nb1\n" {
		t.Fatalf("A local after re-Begin = %q", got)
	}
}

// A local file the store has never seen (written before mirroring, or
// restored from an archive) seeds the store; a local file longer than the
// store with the same prefix keeps its extra lines.
func TestSeedAndKeepLocalSuperset(t *testing.T) {
	ctx := context.Background()
	be := newMem()
	m := &Mirror{Backend: be, Interval: 1 << 40}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	appendFile(t, path, "x1\nx2\n")
	l, err := m.Begin(ctx, path, "s")
	if err != nil {
		t.Fatal(err)
	}
	if got := be.stored("s"); got != "x1|x2" {
		t.Fatalf("seeded = %q", got)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	appendFile(t, path, "x3\n") // written after the last push (e.g. a crash)
	l, err = m.Begin(ctx, path, "s")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if got := be.stored("s"); got != "x1|x2|x3" {
		t.Fatalf("superset pushed = %q", got)
	}
}

func TestMaterialize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.jsonl")
	if err := Materialize(path, [][]byte{[]byte("r1"), []byte("r2")}); err != nil {
		t.Fatal(err)
	}
	appendFile(t, path, "r3\n")
	if err := Materialize(path, [][]byte{[]byte("r1")}); err != nil { // prefix: kept
		t.Fatal(err)
	}
	if got := readFile(t, path); !strings.HasSuffix(got, "r3\n") {
		t.Fatalf("materialize clobbered a superset: %q", got)
	}
}
