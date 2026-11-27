//go:build unix

package server

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestTightenVolumeRootLeavesMissingAndSafeDirs(t *testing.T) {
	if err := tightenVolumeRoot(filepath.Join(t.TempDir(), "absent"), 0o755); err != nil {
		t.Fatalf("missing dir: %v", err)
	}
	if err := tightenVolumeRoot("", 0o755); err != nil {
		t.Fatalf("empty path: %v", err)
	}
	if os.Geteuid() != 0 {
		t.Skip("the rest changes ownership to root")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := tightenVolumeRoot(dir, 0o755); err != nil {
		t.Fatalf("safe dir: %v", err)
	}
}

// A Kubernetes emptyDir or local-path volume is 0777: the root must become
// root-owned and not writable by group or other.
func TestTightenVolumeRootStripsWorldWrite(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to chown")
	}
	for _, tc := range []struct {
		max, want os.FileMode
	}{{0o755, 0o755}, {0o700, 0o700}} {
		dir := filepath.Join(t.TempDir(), "vol")
		if err := os.Mkdir(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o777|os.ModeSticky); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(dir, 10001, 10001); err != nil {
			t.Fatal(err)
		}
		if err := tightenVolumeRoot(dir, tc.max); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode() & (os.ModePerm | os.ModeSticky); got != tc.want {
			t.Fatalf("mode %v, want %v", got, tc.want)
		}
		if uid, gid := statOwner(t, fi); uid != 0 || gid != 0 {
			t.Fatalf("owner %d:%d, want 0:0", uid, gid)
		}
	}
}

// A symlink at the volume root is refused, never followed.
func TestTightenVolumeRootRefusesSymlink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o777); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_ = tightenVolumeRoot(link, 0o755)
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o777 {
		t.Fatalf("the link's target changed to %v", fi.Mode().Perm())
	}
}

func statOwner(t *testing.T, fi os.FileInfo) (uint32, uint32) {
	t.Helper()
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("no stat for %s", fi.Name())
	}
	return st.Uid, st.Gid
}
