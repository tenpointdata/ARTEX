package server

import (
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
	"os"
	"path/filepath"
)

// Root-confined file operations of the workspace file manager. wsResolveConfined
// checks a path, but the server acts on it afterwards, as root, in a tree an agent
// process can write: between the check and the act the agent can replace a
// directory on the path with a symlink to /app/keys or /etc, and a plain
// os.WriteFile or os.RemoveAll would follow it. Every act therefore goes through an
// os.Root opened at the workspace root, with the path relative to it: the kernel
// walk (openat, no following of a symlink that leaves the root) refuses an escape
// that appears after the check, whatever component it is in.
//
// Residual: os.Root follows a symlink that stays inside the root, so a swap within
// the workspace can still redirect an operation to another place in the workspace,
// including the platform-internal subtrees workspacePathDenied hides
// (traffic/_ca, traffic/_index, traffic/_blobs). Nothing outside the workspace root
// is reachable.

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

// wsWriteConfined creates or overwrites the file at cp with content, creating its
// parent directories, all inside the workspace root.
func wsWriteConfined(cp confinedPath, content []byte) error {
	return withWorkspaceRoot(cp, cp.realPath, func(root *os.Root, rel string) error {
		if fi, err := root.Stat(rel); err == nil && fi.IsDir() {
			return errWorkspaceIsDir
		}
		if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return err
		}
		return root.WriteFile(rel, content, 0o644)
	})
}

// wsMkdirConfined creates the directory at cp and its parents inside the root.
func wsMkdirConfined(cp confinedPath) error {
	return withWorkspaceRoot(cp, cp.realPath, func(root *os.Root, rel string) error {
		return root.MkdirAll(rel, 0o755)
	})
}

// wsRemoveConfined removes the entry at cp (a symlink as a link, a directory with
// everything beneath it) inside the root.
func wsRemoveConfined(cp confinedPath) error {
	return withWorkspaceRoot(cp, cp.entryPath, func(root *os.Root, rel string) error {
		if rel == "." {
			return errPathEscape
		}
		return root.RemoveAll(rel)
	})
}

// wsOpenConfined opens the file or directory at cp for reading inside the root.
// The caller closes it.
func wsOpenConfined(cp confinedPath) (*os.File, error) {
	var f *os.File
	err := withWorkspaceRoot(cp, cp.realPath, func(root *os.Root, rel string) error {
		var err error
		f, err = root.Open(rel)
		return err
	})
	return f, err
}

// wsReadDirConfined lists the directory at cp inside the root.
func wsReadDirConfined(cp confinedPath) ([]fs.DirEntry, error) {
	f, err := wsOpenConfined(cp)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

// wsSaveUploadConfined writes an uploaded file to cp inside the root. A symlink
// already at the destination is refused, like saveUpload does for chat uploads.
func wsSaveUploadConfined(hdr *multipart.FileHeader, cp confinedPath) error {
	return withWorkspaceRoot(cp, cp.realPath, func(root *os.Root, rel string) error {
		if fi, err := root.Lstat(rel); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return errors.New("refusing to write through a symlink")
		}
		src, err := hdr.Open()
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, src); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
