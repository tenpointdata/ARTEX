//go:build unix

package server

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Spill directories under per-run uids. In the single-uid layout a spill directory
// (<data>/spill/tasks/<id>, <data>/spill/sessions/<id>) is root:<exec gid> 0750.
// With a range every agent shell keeps the exec gid as a supplementary group, so
// that layout would let task A read task B's command output. With a range the
// spill directory of a live key is root:<key uid> 0750 and its files root:<key uid>
// 0640: the key's uid reads them, no other uid can traverse the directory, and none
// can change it. A released key's spill directory is root:root 0700.

// releasedSpillDirMode is the mode of the spill directory of a released key.
const releasedSpillDirMode = 0o700

// spillKeyOf returns the run key of a spill directory below base: "tasks/<id>" for
// spill/tasks/<id>, "sessions/<id>" for spill/sessions/<id>.
func spillKeyOf(base, dir string) (string, error) {
	rel, err := filepath.Rel(base, dir)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%s is not below %s", dir, base)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	switch {
	case len(parts) == 3 && parts[0] == "spill" && parts[1] == "tasks" && isDigits(parts[2]):
		return "tasks/" + parts[2], nil
	case len(parts) == 3 && parts[0] == "spill" && parts[1] == "sessions" && parts[2] != "":
		return "sessions/" + parts[2], nil
	}
	return "", fmt.Errorf("%s is not a spill directory", dir)
}

// spillDirOf is the spill directory of key.
func (e *execRuns) spillDirOf(key string) string {
	return filepath.Join(e.base, "spill", filepath.FromSlash(key))
}

// makeSpillDir creates the spill directory dir for its key's uid, allocating one
// when the key has none (normally its run directory was made first and holds it).
func (e *execRuns) makeSpillDir(base, dir string) error {
	key, err := spillKeyOf(base, dir)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	uid, fresh, err := e.pool.acquire(key)
	if err != nil {
		return err
	}
	if fresh {
		if err := reapExecUID(uid); err != nil {
			e.pool.quarantine(uid, key)
			return fmt.Errorf("stop the earlier processes of uid %d: %w", uid, err)
		}
		// The key may have no run directory yet; what it has becomes uid's.
		if err := e.ownKey(key, uid); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if err := makeLeafDirAs(base, dir, 0, int(uid), spillDirMode); err != nil {
		return err
	}
	return shareSpillFiles(dir, int(uid))
}

// shareSpill lets the uid of dir's key read the spill files in dir. A key that
// holds no uid (released meanwhile) gets nothing: its files stay root's.
func (e *execRuns) shareSpill(dir string) error {
	key, err := spillKeyOf(e.base, dir)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if uid, ok := e.pool.held(key); ok {
		return shareSpillFiles(dir, int(uid))
	}
	return shareSpillFiles(dir, 0)
}

// ownSpill gives key's spill directory, when there is one, to uid's group.
func (e *execRuns) ownSpill(key string, uid uint32) error {
	dir := e.spillDirOf(key)
	if err := setDirOwnerTo(dir, 0, int(uid), spillDirMode); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return shareSpillFiles(dir, int(uid))
}

// closeSpill makes key's spill directory root:root 0700 without touching its files.
func (e *execRuns) closeSpill(key string) error {
	err := setDirOwnerTo(e.spillDirOf(key), 0, 0, releasedSpillDirMode)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// disownSpill closes key's spill directory and gives its files to root.
func (e *execRuns) disownSpill(key string) error {
	if err := e.closeSpill(key); err != nil {
		return err
	}
	err := shareSpillFiles(e.spillDirOf(key), 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// adoptSpills applies the layout to the spill directories on disk at start-up: a
// key that holds a uid (adoptAll ran first) gives its spill directory to that uid,
// every other one (left root:<exec gid> by the single-uid layout, or with no run
// directory left) is closed to root.
func (e *execRuns) adoptSpills() error {
	var errs []error
	for _, sub := range []string{"tasks", "sessions"} {
		entries, err := os.ReadDir(filepath.Join(e.base, "spill", sub))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, ent := range entries {
			if !ent.IsDir() || (sub == "tasks" && !isDigits(ent.Name())) {
				continue
			}
			key := sub + "/" + ent.Name()
			if uid, ok := e.pool.held(key); ok {
				errs = append(errs, e.ownSpill(key, uid))
			} else {
				errs = append(errs, e.disownSpill(key))
			}
		}
	}
	return errors.Join(errs...)
}

// restoredSpillDir handles a spill directory restored from an archive: it follows
// its key, root's unless the key holds a uid.
func (e *execRuns) restoredSpillDir(dir string) error {
	key, err := spillKeyOf(e.base, dir)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if uid, ok := e.pool.held(key); ok {
		return e.ownSpill(key, uid)
	}
	return e.disownSpill(key)
}
