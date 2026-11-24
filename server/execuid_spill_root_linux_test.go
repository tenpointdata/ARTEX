//go:build linux

package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Autumn-27/artex/agent"
)

// Root test of the spill directories under per-run uids: a spill directory belongs
// to its key's uid group alone, so task A cannot read task B's command output even
// though its shell keeps the exec gid; a release gives it back to root, a new
// acquire regroups it, and a start-up closes a spill directory of the single-uid
// layout.
func TestExecUIDRangeSpillIsolationAsRoot(t *testing.T) {
	f := newExecRangeFixture(t)
	a := agent.EnsureRunDir(f.data, 1, 0)
	b := agent.EnsureRunDir(f.data, 2, 1)
	ua, _, _ := ownerOf(t, a)
	ub, _, _ := ownerOf(t, b)
	spillA := agent.TaskToolOutputDir(f.data, 1, a)
	spillB := agent.TaskToolOutputDir(f.data, 2, b)
	if spillB != agent.TaskSpillDir(f.data, "2") {
		t.Fatalf("task 2's ToolOutputDir is %q", spillB)
	}
	checkDir(t, spillA, ua, spillDirMode)
	checkDir(t, spillB, ub, spillDirMode)

	// A spill file as Norma writes it (root, 0600), shared by the post-tool hook.
	file := filepath.Join(spillB, "out.txt")
	if err := os.WriteFile(file, []byte("secret-of-b"), 0o600); err != nil {
		t.Fatal(err)
	}
	agent.SpillHooks(nil, spillB).PostToolUse(context.Background(), "Bash", nil, []byte("<persisted-output>"), false)
	if u, g, m := ownerOf(t, file); u != 0 || g != ub || m.Perm() != 0o640 {
		t.Fatalf("shared spill file: %d:%d %v, want root:%d 0640", u, g, m, ub)
	}

	readProbe := fmt.Sprintf(`if out=$(cat '%s' 2>&1); then echo "read=$out"; else echo "read=denied:$out"; fi
echo "groups=$(id -G)"
`, file)
	if out := f.bash(t, b, readProbe); !strings.Contains(out, "read=secret-of-b") {
		t.Errorf("task 2 cannot read its own spill:\n%s", out)
	}
	outA := f.bash(t, a, readProbe)
	if !strings.Contains(outA, "read=denied:") || !strings.Contains(outA, "Permission denied") ||
		!strings.Contains(outA, fmt.Sprint(DefaultExecGID)) {
		t.Errorf("task 1's shell (with the exec gid) reading task 2's spill: want EACCES:\n%s", outA)
	}
	var rerr error
	if err := agent.RunAsFileToolIdentity(a, func() { _, rerr = os.ReadFile(file) }); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(rerr, syscall.EACCES) {
		t.Errorf("task 1's file tool reading task 2's spill: %v, want EACCES", rerr)
	}

	// Release: the spill directory and its files go back to root.
	releaseTaskExecIdentity(2)
	checkDir(t, spillB, 0, releasedSpillDirMode)
	if u, g, _ := ownerOf(t, file); u != 0 || g != 0 {
		t.Errorf("spill file after the release: %d:%d, want root", u, g)
	}

	// The task runs again: its new uid gets the spill directory and its files.
	b = agent.EnsureRunDir(f.data, 2, 1)
	ub2, _, _ := ownerOf(t, b)
	checkDir(t, spillB, ub2, spillDirMode)
	if _, g, _ := ownerOf(t, file); g != ub2 {
		t.Errorf("spill file after the re-acquire: gid %d, want %d", g, ub2)
	}

	// A spill directory of the single-uid layout with no live key is closed at
	// start-up; a live key keeps its group.
	legacy := agent.TaskSpillDir(f.data, "9")
	if err := os.Mkdir(legacy, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(legacy, 0, int(DefaultExecGID)); err != nil {
		t.Fatal(err)
	}
	if err := currentExecRuns().adoptAll(); err != nil {
		t.Fatal(err)
	}
	checkDir(t, legacy, 0, releasedSpillDirMode)
	checkDir(t, spillA, ua, spillDirMode)
	checkDir(t, spillB, ub2, spillDirMode)
}
