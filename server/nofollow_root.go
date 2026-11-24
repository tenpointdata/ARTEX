package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// No-follow walks beneath an os.Root. os.Root refuses a symlink that leaves the
// root but follows one that stays inside it. For a path whose symlinks were
// already resolved when it was checked, a symlink met while acting on it means
// the tree changed between check and act, and following it would redirect the
// act to wherever the link points inside the root. These helpers follow no
// symlink at all: every directory component is Lstat'ed (must be a directory,
// not a link), opened as a child os.Root, and pinned by comparing the opened
// directory's identity (os.SameFile) with the Lstat result, so a swap between
// the Lstat and the open is detected. The leaf is handled in the pinned parent
// by name. Product-agnostic: nothing here knows about workspaces.

// errSymlinkInPath answers a path on which a symlink appeared, or an entry was
// replaced, between the check and the act.
var errSymlinkInPath = errors.New("path changed under the operation: symlink or replaced entry")

// errNotRegularInPath answers a read of an entry that is not a regular file.
var errNotRegularInPath = errors.New("not a regular file")

// errNotDirInPath answers a directory component that is not a directory.
var errNotDirInPath = errors.New("path component is not a directory")

// splitRootRel splits a root-relative path into its components; "." and ""
// give none. The caller has checked the path is local (filepath.IsLocal).
func splitRootRel(rel string) []string {
	var out []string
	for _, c := range strings.Split(filepath.ToSlash(rel), "/") {
		if c != "" && c != "." {
			out = append(out, c)
		}
	}
	return out
}

// openDirNoFollow opens the directory at rel beneath root without following any
// symlink, creating missing components with perm when create is set. The
// caller closes the returned root; rel "." gives a fresh handle on root itself.
func openDirNoFollow(root *os.Root, rel string, create bool, perm fs.FileMode) (*os.Root, error) {
	cur, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	for _, comp := range splitRootRel(rel) {
		next, err := openChildDirNoFollow(cur, comp, create, perm)
		cur.Close()
		if err != nil {
			return nil, err
		}
		cur = next
	}
	return cur, nil
}

// openChildDirNoFollow opens the directory name inside dir, refusing a symlink
// and pinning the opened handle to the entry Lstat saw.
func openChildDirNoFollow(dir *os.Root, name string, create bool, perm fs.FileMode) (*os.Root, error) {
	fi, err := dir.Lstat(name)
	if create && errors.Is(err, fs.ErrNotExist) {
		// Mkdir does not follow a link planted at name: it fails with EEXIST.
		if err = dir.Mkdir(name, perm); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		fi, err = dir.Lstat(name)
	}
	if err != nil {
		return nil, err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return nil, errSymlinkInPath
	}
	if !fi.IsDir() {
		return nil, errNotDirInPath
	}
	child, err := dir.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	if st, err := child.Stat("."); err != nil || !os.SameFile(fi, st) {
		child.Close()
		return nil, errSymlinkInPath
	}
	return child, nil
}

// openFileNoFollow opens name inside dir for reading, refusing a symlink and
// any entry swapped between the Lstat and the open. The caller closes it.
func openFileNoFollow(dir *os.Root, name string) (*os.File, error) {
	fi, err := dir.Lstat(name)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return nil, errSymlinkInPath
	}
	if !fi.Mode().IsRegular() {
		return nil, errNotRegularInPath // a FIFO or device would block or misbehave on open
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err != nil || !os.SameFile(fi, st) {
		f.Close()
		return nil, errSymlinkInPath
	}
	return f, nil
}

// replaceFileNoFollow writes a new file at name inside dir with the content
// produced by write: the data goes to a fresh O_EXCL temporary entry in dir,
// which is then renamed over name. A rename replaces a symlink at name rather
// than following it, and never writes into an existing inode, so neither a
// swapped link nor a hard link at name redirects the write. An existing
// directory at name is refused with isDir; an existing regular file keeps its
// permission bits and (on unix) its owner, otherwise perm applies.
func replaceFileNoFollow(dir *os.Root, name string, perm fs.FileMode, isDir error, write func(io.Writer) error) error {
	var prev fs.FileInfo
	if fi, err := dir.Lstat(name); err == nil {
		switch {
		case fi.IsDir():
			return isDir
		case fi.Mode()&fs.ModeSymlink != 0:
			return errSymlinkInPath
		case fi.Mode().IsRegular():
			perm, prev = fi.Mode().Perm(), fi
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	tmp, f, err := createTempNoFollow(dir, perm)
	if err != nil {
		return err
	}
	if prev != nil {
		keepOwner(f, prev)
	}
	werr := write(f)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = dir.Rename(tmp, name)
	}
	if werr != nil {
		dir.Remove(tmp)
	}
	return werr
}

// createTempNoFollow creates a new, randomly named hidden file in dir. O_EXCL
// fails on any existing entry, a symlink included.
func createTempNoFollow(dir *os.Root, perm fs.FileMode) (string, *os.File, error) {
	for range 8 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", nil, err
		}
		tmp := ".tmp-" + hex.EncodeToString(b[:])
		f, err := dir.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if err == nil {
			return tmp, f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, errors.New("could not create a temporary file")
}
