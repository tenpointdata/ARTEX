//go:build unix

package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Autumn-27/artex/agent"
	"golang.org/x/sys/unix"
)

// Home directory of agent processes. A HOME shared by every task lets one
// prompt-injected task plant what later tasks load without asking: a .gitconfig
// (core.fsPathMonitor, aliases), a Python .pth file in ~/.local, an npx or pip
// cache entry, shell rc files. So no home outlives its run:
//
//   - Bash, the interactive shell and custom tools get <run dir>/.artex-home, the
//     home of that run directory alone (tasks/<id>/main, tasks/<id>/i<N> or
//     sessions/<id>). It is deleted and archived with the task or the session. A
//     later task has another run directory, which the executor cannot create in
//     advance: tasks/ and tasks/<id> are root-only.
//   - Anything else (an MCP stdio server, a process whose working directory is not a
//     run directory) gets a fresh directory of its own under <data>/exec-home.
//
// <data>/exec-home is root:root 0711 and emptied at every start, so the executor
// cannot list it or plant an entry there, and nothing in it survives a restart.
// Each home is created by root without following a symlink and handed to its uid
// with mode 0700 through a descriptor. Nothing the image installs lives in these
// homes: tool data is at fixed paths (PLAYWRIGHT_BROWSERS_PATH,
// NUCLEI_TEMPLATES_DIR) that the environment allowlist keeps. bbot has no such
// override, so the bbot on PATH copies a read-only seed into the caller's home
// itself, as the caller, before it starts (docker/bbot-home.sh).
//
// With a single executor uid this is not isolation between live runs: every agent
// process has the same uid, so one can still write into another run directory that
// is live and that it knows the path of. What it removes is the shared place that
// every later task trusts. With -exec-uid-range each task and chat session has a
// uid of its own and its run directories are closed to the others (execuid.go).

// runHomeName is the home directory inside a run directory.
const runHomeName = ".artex-home"

// homeEnv returns HOME and the XDG base directories for a home directory.
func homeEnv(home string) []string {
	return []string{
		"HOME=" + home,
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
	}
}

// prepareExecHome empties and recreates <data>/exec-home, root:root 0711, and
// records its resolved path for the shims and applyExecCredential. It removes the
// shared, executor-owned home of earlier versions too.
func prepareExecHome(dataDir string) error {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return err
	}
	home := filepath.Join(abs, execHomeDir)
	// os.RemoveAll does not follow a symlink, at the top or below it.
	if err := os.RemoveAll(home); err != nil {
		return err
	}
	if err := os.Mkdir(home, 0o711); err != nil {
		return err
	}
	if err := setDirOwner(home, 0, 0o711); err != nil {
		return err
	}
	return os.Setenv(envExecHome, home)
}

// execHomeFor returns the home of a process that runs in dir as uid and gid,
// created when missing (see above). runDirHome selects the run-directory home when
// dir is a run directory; otherwise, or when that home cannot be used (the agent
// replaced it with a symlink or a file), a fresh home is made under exec-home. It
// returns "" without an error when PrepareExecPrivilege has not run.
func execHomeFor(dir string, uid, gid uint32, runDirHome bool) (string, error) {
	base := os.Getenv(envExecHome)
	if base == "" {
		return "", nil
	}
	if runDirHome && dir != "" {
		if real, err := filepath.EvalSymlinks(dir); err == nil && isRunDir(filepath.Dir(base), real) {
			if err := makeOwnedDir(real, runHomeName, uid, gid); err == nil {
				return filepath.Join(real, runHomeName), nil
			}
		}
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	name := fmt.Sprintf("p%d-%s", uid, hex.EncodeToString(b[:]))
	if err := makeOwnedDir(base, name, uid, gid); err != nil {
		return "", fmt.Errorf("create a home under %s: %w", base, err)
	}
	return filepath.Join(base, name), nil
}

// isRunDir reports whether dir (resolved) is a run directory under dataDir:
// tasks/<id>/main, tasks/<id>/i<N> or sessions/<id>.
func isRunDir(dataDir, dir string) bool {
	rel, err := filepath.Rel(dataDir, dir)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	switch {
	case len(parts) == 3 && parts[0] == "tasks":
		return isDigits(parts[1]) && agent.IsRunDirName(parts[2])
	case len(parts) == 2 && parts[0] == "sessions":
		return parts[1] != ""
	}
	return false
}

// makeOwnedDir makes parent/name a directory of uid:gid, mode 0700. The parent and
// the entry are opened without following a symlink, and the entry is changed
// through its descriptor: a symlink or a file planted at the name is refused and
// never redirects the change. An entry already owned by uid is used as it is; one
// root owns (just created) is handed over; any other owner is refused.
func makeOwnedDir(parent, name string, uid, gid uint32) error {
	pfd, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(pfd)
	if err := unix.Mkdirat(pfd, name, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
		return err
	}
	fd, err := unix.Openat(pfd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	switch st.Uid {
	case uid:
		return nil
	case 0:
		if err := unix.Fchown(fd, int(uid), int(gid)); err != nil {
			return err
		}
		return unix.Fchmod(fd, 0o700)
	}
	return fmt.Errorf("%s is owned by uid %d, not %d", filepath.Join(parent, name), st.Uid, uid)
}
