package server

import (
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
	"os"
	"path/filepath"
)

// Root-confined, no-follow file operations of the workspace file manager.
// wsResolveConfined checks a path (resolving every symlink on it) and
// workspacePathDenied hides the platform-internal subtrees, but the server acts
// on the path afterwards, as root, in a tree an agent process can write: between
// the check and the act the agent can replace a directory on the path with a
// symlink, to /app/keys or /etc, or to traffic/_ca inside the workspace.
//
// Every act therefore starts from an os.Root opened at the workspace root and
// walks the already-resolved path with the no-follow helpers (nofollow_root.go):
// each directory component must still be a directory and is pinned with
// os.SameFile, the leaf is handled by name in the pinned parent, and a symlink
// met anywhere on the path is refused, since the check resolved them all. Writes
// and uploads go to an O_EXCL temporary file renamed over the leaf; reads and
// downloads compare the opened file with the Lstat'ed entry; removal unlinks the
// leaf in the pinned parent (os.Root.RemoveAll unlinks a symlink and descends
// with no-follow opens). A swap can make an operation fail, never redirect it,
// neither out of the root nor into another place inside it.
//
// Residual: the path check does not see hard links. A hard link in a run
// directory to a file in a hidden subtree could be read or downloaded through
// the file manager (writes replace the entry and never touch the linked inode).
// Creating one needs the agent to own the file or have read and write access to
// it under Linux's default fs.protected_hardlinks=1, which root-owned
// traffic/_ca files do not grant. A directory moved bodily into the path is
// followed as the same directory, which needs write access to its old parent.

// errWorkspaceIsDir answers a write to a directory.
var errWorkspaceIsDir = errors.New("target is a directory")

// wsRootRel returns path relative to the root of cp, for an os.Root opened there.
func wsRootRel(cp confinedPath, path string) (string, error) {
	rel, err := filepath.Rel(cp.realRoot, path)
	if err != nil || (rel != "." && !filepath.IsLocal(rel)) {
		return "", errPathEscape
	}
	return rel, nil
}

// withWorkspaceRoot opens an os.Root at cp's root and runs fn with it and the path
// of target relative to it.
func withWorkspaceRoot(cp confinedPath, target string, fn func(root *os.Root, rel string) error) error {
	rel, err := wsRootRel(cp, target)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(cp.realRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	return fn(root, rel)
}

// withWorkspaceParent opens target's parent directory without following any
// symlink (creating it when create is set) and runs fn with it and target's
// final name. The workspace root itself has no parent here: errPathEscape.
func withWorkspaceParent(cp confinedPath, target string, create bool, fn func(dir *os.Root, leaf string) error) error {
	return withWorkspaceRoot(cp, target, func(root *os.Root, rel string) error {
		if rel == "." {
			return errPathEscape
		}
		dir, err := openDirNoFollow(root, filepath.Dir(rel), create, 0o755)
		if err != nil {
			return err
		}
		defer dir.Close()
		return fn(dir, filepath.Base(rel))
	})
}

// wsWriteConfined creates or replaces the file at cp with content, creating its
// parent directories, all without following a symlink.
func wsWriteConfined(cp confinedPath, content []byte) error {
	if cp.realPath == cp.realRoot {
		return errWorkspaceIsDir
	}
	return withWorkspaceParent(cp, cp.realPath, true, func(dir *os.Root, leaf string) error {
		return replaceFileNoFollow(dir, leaf, 0o644, errWorkspaceIsDir, func(w io.Writer) error {
			_, err := w.Write(content)
			return err
		})
	})
}

// wsMkdirConfined creates the directory at cp and its parents, following no symlink.
func wsMkdirConfined(cp confinedPath) error {
	return withWorkspaceRoot(cp, cp.realPath, func(root *os.Root, rel string) error {
		dir, err := openDirNoFollow(root, rel, true, 0o755)
		if err != nil {
			return err
		}
		return dir.Close()
	})
}

// wsRemoveConfined removes the entry at cp (a symlink as a link, a directory with
// everything beneath it) by name in its pinned parent.
func wsRemoveConfined(cp confinedPath) error {
	return withWorkspaceParent(cp, cp.entryPath, false, func(dir *os.Root, leaf string) error {
		return dir.RemoveAll(leaf)
	})
}

// wsOpenConfined opens the file or directory at cp for reading, following no
// symlink. The caller closes it.
func wsOpenConfined(cp confinedPath) (*os.File, error) {
	var f *os.File
	err := withWorkspaceRoot(cp, cp.realPath, func(root *os.Root, rel string) error {
		var err error
		if rel == "." {
			f, err = root.Open(".")
			return err
		}
		dir, err := openDirNoFollow(root, filepath.Dir(rel), false, 0)
		if err != nil {
			return err
		}
		defer dir.Close()
		f, err = openFileNoFollow(dir, filepath.Base(rel))
		return err
	})
	return f, err
}

// wsReadDirConfined lists the directory at cp, following no symlink. Each entry's
// metadata is read by name in the pinned directory (Lstat: a link is reported as
// a link), so it cannot come from elsewhere.
func wsReadDirConfined(cp confinedPath) ([]fs.FileInfo, error) {
	var out []fs.FileInfo
	err := withWorkspaceRoot(cp, cp.realPath, func(root *os.Root, rel string) error {
		dir, err := openDirNoFollow(root, rel, false, 0)
		if err != nil {
			return err
		}
		defer dir.Close()
		f, err := dir.Open(".")
		if err != nil {
			return err
		}
		names, err := f.Readdirnames(-1)
		f.Close()
		if err != nil {
			return err
		}
		for _, name := range names {
			if fi, err := dir.Lstat(name); err == nil {
				out = append(out, fi)
			}
		}
		return nil
	})
	return out, err
}

// wsSaveUploadConfined writes an uploaded file to cp, replacing what is there,
// in its existing parent directory. A symlink at the destination is refused,
// like saveUpload does for chat uploads.
func wsSaveUploadConfined(hdr *multipart.FileHeader, cp confinedPath) error {
	return withWorkspaceParent(cp, cp.realPath, false, func(dir *os.Root, leaf string) error {
		src, err := hdr.Open()
		if err != nil {
			return err
		}
		defer src.Close()
		return replaceFileNoFollow(dir, leaf, 0o644, errWorkspaceIsDir, func(w io.Writer) error {
			_, err := io.Copy(w, src)
			return err
		})
	})
}
