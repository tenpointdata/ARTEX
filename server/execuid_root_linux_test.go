//go:build linux

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	actool "github.com/Autumn-27/norma/tool"
)

// Root tests of the per-run executor uids (-exec-uid-range, execuid.go): a shell,
// a custom tool and a file tool call of task A cannot write a run directory of task
// B or its home, a released uid owns nothing and its processes stop, and a restart
// keeps the uid of a live task.

var testExecRange = ExecUIDRange{Lo: 20000, Hi: 20009}

type execRangeFixture struct {
	priv ExecPrivilege
	data string
}

func newExecRangeFixture(t *testing.T) *execRangeFixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	old := syscall.Umask(0o022)
	syscall.Umask(old)
	t.Cleanup(func() { syscall.Umask(old) })
	for _, k := range []string{"PATH", envExecBash, envExecUID, envExecGID, envExecHome, envExecUIDRange} {
		t.Setenv(k, os.Getenv(k))
	}
	tmp, err := os.MkdirTemp("", "artex-exec-tmp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	if err := os.Chmod(tmp, 0o777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	data, err := os.MkdirTemp("", "artex-exec-range-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(data) })
	if err := os.Chmod(data, 0o755); err != nil {
		t.Fatal(err)
	}
	priv, err := ResolveExecPrivilege(DefaultExecUID, DefaultExecGID, true)
	if err != nil {
		t.Fatal(err)
	}
	if priv, err = priv.WithUIDRange(testExecRange); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		agent.SetRunDirMaker(nil)
		agent.SetSpillDirMaker(nil, nil)
		agent.SetFileToolIdentity(nil)
		setExecRuns(nil)
	})
	if err := PrepareExecPrivilege(&priv, data, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := InstallBashShim(priv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.SplitList(os.Getenv("PATH"))[0]) })
	return &execRangeFixture{priv: priv, data: data}
}

// bash runs script through Norma's Bash tool in dir, as the agent does.
func (f *execRangeFixture) bash(t *testing.T, dir, script string) string {
	t.Helper()
	in, _ := json.Marshal(map[string]string{"command": script})
	res, err := actool.NewBash().Call(context.Background(), in, &actool.ToolContext{WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return res.Flatten()
}

func ownerOf(t *testing.T, path string) (uid, gid uint32, mode os.FileMode) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := fi.Sys().(*syscall.Stat_t)
	return st.Uid, st.Gid, fi.Mode()
}

// writeProbe is the shell probe of a write: "<label>=ok" or "<label>=denied:<msg>".
func writeProbe(label, path string) string {
	return fmt.Sprintf(`if out=$( (echo x > '%s') 2>&1 ); then echo %s=ok; else echo "%s=denied:$out"; fi
`, path, label, label)
}

func TestExecUIDRangeIsolatesTasksAsRoot(t *testing.T) {
	f := newExecRangeFixture(t)
	a := agent.EnsureRunDir(f.data, 1, 0)
	b := agent.EnsureRunDir(f.data, 2, 1)
	ua, _, ma := ownerOf(t, a)
	ub, gb, mb := ownerOf(t, b)
	if ua == ub || !testExecRange.Contains(ua) || !testExecRange.Contains(ub) || gb != ub {
		t.Fatalf("run directories: task 1 uid %d, task 2 uid %d gid %d; want two uids of %s", ua, ub, gb, testExecRange)
	}
	if mb.Perm() != 0o770 || mb&os.ModeSetgid == 0 || ma.Perm() != 0o770 {
		t.Fatalf("run directory modes %v %v, want 2770", ma, mb)
	}
	// Task 2's shell runs once, so its home exists, and runs as task 2's uid.
	outB := f.bash(t, b, "id -u; echo b > own")
	if !strings.Contains(outB, fmt.Sprint(ub)) {
		t.Fatalf("task 2's shell is not uid %d:\n%s", ub, outB)
	}
	homeB := filepath.Join(b, runHomeName)
	if u, _, _ := ownerOf(t, homeB); u != ub {
		t.Fatalf("task 2's home is uid %d, want %d", u, ub)
	}

	outA := f.bash(t, a, `echo "uid=$(id -u) groups=$(id -G)"
`+writeProbe("own", "own.txt")+writeProbe("other", filepath.Join(b, "x"))+writeProbe("otherhome", filepath.Join(homeB, "x"))+
		`if ls '`+b+`' >/dev/null 2>&1; then echo list=ok; else echo list=denied; fi
`)
	for _, want := range []string{
		fmt.Sprintf("uid=%d groups=%d %d", ua, ua, DefaultExecGID),
		"own=ok",
		"other=denied:", "otherhome=denied:", "list=denied",
	} {
		if !strings.Contains(outA, want) {
			t.Errorf("task 1's shell: want %q in:\n%s", want, outA)
		}
	}
	for _, line := range strings.Split(outA, "\n") {
		if strings.Contains(line, "=denied:") && !strings.Contains(line, "Permission denied") {
			t.Errorf("want EACCES (Permission denied): %s", line)
		}
	}

	// A custom tool of task 1 (SysProcAttr.Credential).
	c := exec.Command("sh", "-c", writeProbe("tool", filepath.Join(b, "y")))
	c.Dir = a
	applyExecCredential(c, f.priv)
	out, _ := c.CombinedOutput()
	if !strings.Contains(string(out), "tool=denied:") {
		t.Errorf("task 1's custom tool wrote into task 2's run directory: %s", out)
	}

	// The file tools of task 1 (per-thread file system ids).
	var werr error
	if err := agent.RunAsFileToolIdentity(a, func() { werr = os.WriteFile(filepath.Join(b, "z"), []byte("x"), 0o644) }); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(werr, syscall.EACCES) {
		t.Errorf("task 1's file tool writing into task 2: %v, want EACCES", werr)
	}
	if err := agent.RunAsFileToolIdentity(a, func() { werr = os.WriteFile(filepath.Join(a, "z"), []byte("x"), 0o644) }); err != nil || werr != nil {
		t.Errorf("task 1's file tool in its own run directory: %v %v", err, werr)
	}
	t.Logf("task 1 shell:\n%s", outA)
}

func TestExecUIDRangeReleaseAsRoot(t *testing.T) {
	f := newExecRangeFixture(t)
	b := agent.EnsureRunDir(f.data, 2, 1)
	ub, _, _ := ownerOf(t, b)
	// A socket in the run directory (a stale ssh ControlMaster) cannot be opened:
	// the release must still close the directory and give it to root.
	f.bash(t, b, `echo b > own; python3 -c "import socket; socket.socket(socket.AF_UNIX).bind('sock')"`)
	// A process of task 2 that outlives its tool call.
	c := exec.Command("sleep", "300")
	c.Dir = b
	applyExecCredential(c, f.priv)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()

	releaseTaskExecIdentity(2)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = c.Process.Kill()
		t.Fatal("the process of the released uid is still running")
	}
	for _, p := range []string{b, filepath.Join(b, "own"), filepath.Join(b, "sock"), filepath.Join(b, runHomeName)} {
		if u, g, _ := ownerOf(t, p); u != 0 || g != 0 {
			t.Errorf("%s is %d:%d after the release, want root", p, u, g)
		}
	}
	if _, _, m := ownerOf(t, b); m.Perm() != releasedRunDirMode {
		t.Errorf("released run directory mode %v", m)
	}

	// The task runs again (a task chat): a new uid takes over its files.
	agent.EnsureRunDir(f.data, 2, 1)
	ub2, _, _ := ownerOf(t, b)
	if ub2 == ub || !testExecRange.Contains(ub2) {
		t.Fatalf("re-acquired uid %d (was %d)", ub2, ub)
	}
	for _, p := range []string{filepath.Join(b, "own"), filepath.Join(b, runHomeName)} {
		if u, _, _ := ownerOf(t, p); u != ub2 {
			t.Errorf("%s is uid %d, want %d", p, u, ub2)
		}
	}
}

func TestExecUIDRangeRestartKeepsLiveUIDAsRoot(t *testing.T) {
	f := newExecRangeFixture(t)
	a := agent.EnsureRunDir(f.data, 1, 0)
	ua, _, _ := ownerOf(t, a)
	// A run directory of the single-uid layout, from before the range.
	legacy := filepath.Join(f.data, "sessions", "conv-9")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	setExecRuns(nil)
	p := f.priv
	if err := PrepareExecPrivilege(&p, f.data, "", ""); err != nil {
		t.Fatal(err)
	}
	if u, _, _ := ownerOf(t, a); u != ua {
		t.Errorf("task 1 changed uid over a restart: %d, want %d", u, ua)
	}
	if u, _, m := ownerOf(t, legacy); u != 0 || m.Perm() != releasedRunDirMode {
		t.Errorf("a run directory without a uid is %d %v at start-up, want root 0700", u, m)
	}
	if b := agent.EnsureRunDir(f.data, 2, 0); func() bool { u, _, _ := ownerOf(t, b); return u == ua }() {
		t.Error("a new task got the uid of a live one")
	}
}
