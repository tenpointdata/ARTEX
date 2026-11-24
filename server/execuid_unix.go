//go:build unix

package server

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/Autumn-27/artex/agent"
)

// Per-run executor identities on disk (see execuid.go for the model).
//
// A live run key ("tasks/<id>", "sessions/<id>") holds a uid X of the range. Its
// run directories are X:X 2770, and every entry below them, and the older entries
// directly in tasks/<id>, belong to X:X with group read/write (setgid directories).
// A released key's entries belong to root:root and its run directories are 0700,
// so a uid that goes back to the pool owns nothing on disk. Its processes are
// killed when it is released (reapExecUID). tasks/, sessions/ and tasks/<id> stay
// root:root 0755 as in the single-uid layout.

// envExecUIDRange carries the range to the bash shim.
const envExecUIDRange = "ARTEX_EXEC_UID_RANGE"

// execReapArg is the argv marker of the reaper: artex __artex-exec-reap <uid>,
// started as root; it drops to uid before it signals.
const execReapArg = "__artex-exec-reap"

// rangedRunDirMode is the mode of a run directory of a live run key: its uid and
// its gid (the same number) alone, setgid so what is created there keeps the gid.
const rangedRunDirMode = 0o770 | os.ModeSetgid

// releasedRunDirMode is the mode of a run directory of a released key: root only.
const releasedRunDirMode = 0o700

// processIdentity is the identity an agent process or a file tool call runs as.
type processIdentity struct {
	UID, GID uint32
	Groups   []uint32
}

// identityIn returns the identity of an agent process that works in dir. With a
// range, a run directory of a live key gives that key's uid (the owner of the
// directory) with the executor group as a supplementary group, for the script and
// spill directories. Any other directory gives the executor identity.
func (p ExecPrivilege) identityIn(dataDir, dir string) processIdentity {
	if uid, ok := runDirIdentity(dataDir, p.Range, dir); ok {
		return processIdentity{UID: uid, GID: uid, Groups: []uint32{uid, p.GID}}
	}
	return processIdentity{UID: p.UID, GID: p.GID, Groups: []uint32{p.GID}}
}

// runDirIdentity returns the uid of the run directory dir: its owner, when dir
// (resolved) is a run directory under dataDir, owned by a uid of r with the gid of
// the same number. Only root sets the owner of a run directory.
func runDirIdentity(dataDir string, r ExecUIDRange, dir string) (uint32, bool) {
	if !r.Enabled() || dataDir == "" || dir == "" {
		return 0, false
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil || !isRunDir(dataDir, real) {
		return 0, false
	}
	fi, err := os.Lstat(real)
	if err != nil || !fi.IsDir() {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != st.Gid || !r.Contains(st.Uid) {
		return 0, false
	}
	return st.Uid, true
}

// execDataDir is the resolved data directory, the parent of exec-home; "" before
// PrepareExecPrivilege.
func execDataDir() string {
	if home := os.Getenv(envExecHome); home != "" {
		return filepath.Dir(home)
	}
	return ""
}

// execRuns is the live state of the per-run identities: the pool and the data
// directory. It is set by PrepareExecPrivilege when a range is configured.
type execRuns struct {
	mu   sync.Mutex // serializes the ownership changes of run keys
	pool *execUIDPool
	base string
}

var activeExecRuns struct {
	sync.Mutex
	r *execRuns
}

func currentExecRuns() *execRuns {
	activeExecRuns.Lock()
	defer activeExecRuns.Unlock()
	return activeExecRuns.r
}

func setExecRuns(r *execRuns) {
	activeExecRuns.Lock()
	activeExecRuns.r = r
	activeExecRuns.Unlock()
}

// keyDir is the directory a run key's entries are in, and whether the key's
// directory is itself the run directory (sessions/<id>).
func (e *execRuns) keyDir(key string) (dir string, isLeaf bool) {
	return filepath.Join(e.base, filepath.FromSlash(key)), filepath.Dir(filepath.FromSlash(key)) == "sessions"
}

// makeRunDir creates the run directory leaf for its key's uid, allocating one when
// the key has none; a fresh uid takes over every entry of the key.
func (e *execRuns) makeRunDir(base, leaf string) error {
	key, err := runKeyOf(base, leaf)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	uid, fresh, err := e.pool.acquire(key)
	if err != nil {
		return err
	}
	_, statErr := os.Lstat(leaf)
	if err := makeLeafDirAs(base, leaf, int(uid), int(uid), rangedRunDirMode); err != nil {
		return err
	}
	switch {
	case fresh:
		// A process of an earlier holder of uid that outlived its release (a
		// custom tool started during it) must not get this key's files.
		if err := reapExecUID(uid); err != nil {
			e.pool.quarantine(uid, key)
			return fmt.Errorf("stop the earlier processes of uid %d: %w", uid, err)
		}
		return e.ownKey(key, uid)
	case statErr == nil:
		return ownRunDir(leaf, uid)
	}
	return nil
}

// ownKey gives every entry of key to uid.
func (e *execRuns) ownKey(key string, uid uint32) error {
	dir, isLeaf := e.keyDir(key)
	if isLeaf {
		return ownRunDir(dir, uid)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var errs []error
	for _, ent := range entries {
		path := filepath.Join(dir, ent.Name())
		if agent.IsRunDirName(ent.Name()) && ent.IsDir() {
			errs = append(errs, ownRunDir(path, uid))
		} else {
			errs = append(errs, changeTreeOwner(path, entryOwner{uid: int(uid), gid: int(uid), share: true}))
		}
	}
	return errors.Join(errs...)
}

// disownKey gives every entry of key back to root and closes its run directories.
func (e *execRuns) disownKey(key string) error {
	dir, isLeaf := e.keyDir(key)
	root := entryOwner{uid: 0, gid: 0}
	if isLeaf {
		return disownRunDir(dir)
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, ent := range entries {
		path := filepath.Join(dir, ent.Name())
		if agent.IsRunDirName(ent.Name()) && ent.IsDir() {
			errs = append(errs, disownRunDir(path))
		} else {
			errs = append(errs, changeTreeOwner(path, root))
		}
	}
	return errors.Join(errs...)
}

// ownRunDir gives a run directory and everything below it to uid:uid.
func ownRunDir(dir string, uid uint32) error {
	if err := changeTreeOwner(dir, entryOwner{uid: int(uid), gid: int(uid), share: true}); err != nil {
		return err
	}
	return setDirOwnerTo(dir, int(uid), int(uid), rangedRunDirMode)
}

// disownRunDir gives a run directory and everything below it to root and closes it.
func disownRunDir(dir string) error {
	if err := changeTreeOwner(dir, entryOwner{uid: 0, gid: 0}); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	err := setDirOwnerTo(dir, 0, 0, releasedRunDirMode)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// release frees key's uid. Its run directories are closed first (root 0700), so
// no new process resolves to uid (the shims check again after the drop), then its
// processes are killed and its entries and fallback homes go back to root. A uid
// whose release failed is quarantined, never handed out again in this run. Errors
// are logged; a release never fails the caller.
func (e *execRuns) release(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	uid, ok := e.pool.release(key)
	if !ok {
		return
	}
	var errs []error
	errs = append(errs, e.closeKey(key))
	if err := reapExecUID(uid); err != nil {
		errs = append(errs, fmt.Errorf("stop the processes of uid %d: %w", uid, err))
	}
	errs = append(errs, e.disownKey(key), removeFallbackHomes(uid))
	if err := errors.Join(errs...); err != nil {
		e.pool.quarantine(uid, key)
		log.Printf("[exec] release of uid %d (%s) incomplete, uid quarantined: %v", uid, key, err)
	}
}

// closeKey makes every run directory of key root:root 0700 without walking it.
func (e *execRuns) closeKey(key string) error {
	dir, isLeaf := e.keyDir(key)
	leaves := []string{dir}
	if !isLeaf {
		leaves = nil
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, ent := range entries {
			if agent.IsRunDirName(ent.Name()) && ent.IsDir() {
				leaves = append(leaves, filepath.Join(dir, ent.Name()))
			}
		}
	}
	var errs []error
	for _, leaf := range leaves {
		if err := setDirOwnerTo(leaf, 0, 0, releasedRunDirMode); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// removeFallbackHomes removes the homes under exec-home made for uid (execHomeFor
// names them p<uid>-<hex>), so a later holder of uid finds none of them.
func removeFallbackHomes(uid uint32) error {
	base := os.Getenv(envExecHome)
	if base == "" {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(base, "p"+execIDString(uid)+"-*"))
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range matches {
		errs = append(errs, os.RemoveAll(m)) // does not follow symlinks
	}
	return errors.Join(errs...)
}

// checkRunDirStill fails when the working directory of a process that dropped to a
// per-run uid no longer belongs to that uid: its key was released between the
// lookup and the drop. Any error counts as a mismatch.
func checkRunDirStill(id processIdentity, r ExecUIDRange) error {
	if !r.Contains(id.UID) {
		return nil
	}
	fi, err := os.Stat(".")
	if err != nil {
		return fmt.Errorf("the run directory was released: %w", err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != id.UID {
		return errors.New("the run directory was released")
	}
	return nil
}

// adoptAll applies the layout to what is on disk at start-up: a key whose run
// directory is owned by a uid of the range keeps that uid (a task that runs on
// after a restart) and its entries are given to it again; every other key is
// released on disk. tasks/, sessions/ and the task directories become root:root
// 0755.
func (e *execRuns) adoptAll() error {
	if err := os.MkdirAll(e.base, 0o755); err != nil {
		return err
	}
	for _, sub := range []string{"tasks", "sessions"} {
		root := filepath.Join(e.base, sub)
		if err := os.Mkdir(root, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		if err := setDirOwnerTo(root, 0, 0, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		for _, ent := range entries {
			if !ent.IsDir() || (sub == "tasks" && !isDigits(ent.Name())) {
				continue
			}
			key := sub + "/" + ent.Name()
			if sub == "tasks" {
				if err := setDirOwnerTo(filepath.Join(root, ent.Name()), 0, 0, 0o755); err != nil {
					return err
				}
			}
			if err := e.adoptKey(key); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
	}
	return nil
}

func (e *execRuns) adoptKey(key string) error {
	dir, isLeaf := e.keyDir(key)
	leaves := []string{dir}
	if !isLeaf {
		leaves = nil
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, ent := range entries {
			if agent.IsRunDirName(ent.Name()) && ent.IsDir() {
				leaves = append(leaves, filepath.Join(dir, ent.Name()))
			}
		}
	}
	for _, leaf := range leaves {
		if uid, ok := ownerInRange(leaf, e.pool.r); ok && e.pool.adopt(key, uid) {
			return e.ownKey(key, uid)
		}
	}
	return e.disownKey(key)
}

// ownerInRange returns the owner of dir when it is a directory of a uid of r with
// the gid of the same number.
func ownerInRange(dir string, r ExecUIDRange) (uint32, bool) {
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != st.Gid || !r.Contains(st.Uid) {
		return 0, false
	}
	return st.Uid, true
}

// releaseTaskExecIdentity frees the uid of a task that ended or is deleted.
func releaseTaskExecIdentity(taskID int64) {
	if e := currentExecRuns(); e != nil {
		e.release("tasks/" + strconv.FormatInt(taskID, 10))
	}
}

// releaseSessionExecIdentity frees the uid of a chat session that is deleted.
func releaseSessionExecIdentity(sessionID string) {
	if e := currentExecRuns(); e != nil && sessionID != "" && filepath.Base(sessionID) == sessionID {
		e.release("sessions/" + sessionID)
	}
}

// restoredTaskDir handles a task directory restored from an archive: the task is
// not live, so its key is released and its entries belong to root.
func (e *execRuns) restoredTaskDir(taskDir string) error {
	key := "tasks/" + filepath.Base(taskDir)
	e.release(key)
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := setDirOwnerTo(taskDir, 0, 0, 0o755); err != nil {
		return err
	}
	return e.disownKey(key)
}

// reapExecUID kills every process of uid: the reaper drops to uid and sends SIGKILL
// to -1, which reaches the processes of its own uid only (and not itself). It is
// started as root, so the uid needs no access to the binary, and runs twice so that
// a process forked during the first pass is caught by the second.
func reapExecUID(uid uint32) error {
	if uid == 0 {
		return errors.New("refusing to reap uid 0")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		c := exec.Command(self, execReapArg, execIDString(uid))
		c.Dir = "/"
		c.Env = []string{}
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
	}
	return nil
}

// runReaper is the reaper process: <uid>. It drops to uid for good and checks that
// it is no longer root before it signals: kill(-1) as root would reach every
// process of the host.
func runReaper(args []string) {
	if len(args) != 1 {
		shimFail(errors.New("reaper needs <uid>"))
	}
	uid, err := strconv.ParseUint(args[0], 10, 31)
	if err != nil || uid == 0 {
		shimFail(fmt.Errorf("reaper: bad uid %q", args[0]))
	}
	id := uint32(uid)
	if err := dropTo(processIdentity{UID: id, GID: id, Groups: []uint32{id}}); err != nil {
		shimFail(err)
	}
	if os.Geteuid() == 0 || os.Getuid() == 0 {
		shimFail(errors.New("the reaper must not signal as root"))
	}
	if err := syscall.Kill(-1, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		shimFail(fmt.Errorf("kill: %w", err))
	}
	os.Exit(0)
}

// prepareExecRuns sets up the per-run identities for p.Range: it applies the layout
// to the data directory, installs the run directory maker and the file tools'
// per-directory identity, and tells the shims the range.
func prepareExecRuns(p ExecPrivilege, dataDir string) error {
	e := &execRuns{pool: newExecUIDPool(p.Range), base: dataDir}
	if err := e.adoptAll(); err != nil {
		return fmt.Errorf("apply the per-run uid layout: %w", err)
	}
	setExecRuns(e)
	agent.SetRunDirMaker(e.makeRunDir)
	return os.Setenv(envExecUIDRange, p.Range.String())
}

// fileToolIdentity is the identity of the file tools: per working directory with a
// range, the executor's otherwise.
func fileToolIdentity(p ExecPrivilege) *agent.FileToolIdentity {
	id := &agent.FileToolIdentity{UID: p.UID, GID: p.GID}
	if p.Range.Enabled() {
		id.For = func(dir string) agent.FileToolIdentity {
			pi := p.identityIn(execDataDir(), dir)
			return agent.FileToolIdentity{UID: pi.UID, GID: pi.GID, Groups: pi.Groups}
		}
	}
	return id
}
