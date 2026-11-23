package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// toctouFixture is a workspace root with a directory "run/sub" that an agent
// swaps for a symlink to an outside directory after the server checked a path
// below it. outside holds a file the server must neither change nor remove.
type toctouFixture struct {
	s                     *Server
	root, outside, secret string
}

func newTOCTOUFixture(t *testing.T) *toctouFixture {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "data")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{filepath.Join(root, "run", "sub"), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(secret, []byte("ORIGINAL"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "run", "sub", "victim.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &toctouFixture{s: &Server{m: &Manager{dir: root}}, root: root, outside: outside, secret: secret}
}

// resolve checks rel the way the handlers do, then swaps run/sub for a symlink
// that leads out of the root: the race the agent wins between check and act.
func (f *toctouFixture) resolveThenSwap(t *testing.T, rel string) confinedPath {
	t.Helper()
	cp, ok := f.s.wsResolveConfined(rel)
	if !ok {
		t.Fatalf("%q refused before the swap", rel)
	}
	sub := filepath.Join(f.root, "run", "sub")
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, f.outside, sub)
	return cp
}

func (f *toctouFixture) assertOutsideIntact(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(f.secret)
	if err != nil || string(data) != "ORIGINAL" {
		t.Fatalf("outside file changed: %q %v", data, err)
	}
	entries, err := os.ReadDir(f.outside)
	if err != nil || len(entries) != 1 {
		t.Fatalf("outside directory holds %v (%v), want only victim.txt", entries, err)
	}
}

func TestWorkspaceWriteRefusesSwappedSymlink(t *testing.T) {
	f := newTOCTOUFixture(t)
	cp := f.resolveThenSwap(t, "run/sub/victim.txt")
	if err := wsWriteConfined(cp, []byte("PWNED")); err == nil {
		t.Fatal("write through a swapped symlink succeeded")
	}
	f.assertOutsideIntact(t)

	// A new file below the swapped directory, parents to be created.
	f = newTOCTOUFixture(t)
	cp = f.resolveThenSwap(t, "run/sub/new/planted.txt")
	if err := wsWriteConfined(cp, []byte("PWNED")); err == nil {
		t.Fatal("write of a new file through a swapped symlink succeeded")
	}
	f.assertOutsideIntact(t)
}

func TestWorkspaceMkdirRefusesSwappedSymlink(t *testing.T) {
	f := newTOCTOUFixture(t)
	cp := f.resolveThenSwap(t, "run/sub/newdir")
	if err := wsMkdirConfined(cp); err == nil {
		t.Fatal("mkdir through a swapped symlink succeeded")
	}
	f.assertOutsideIntact(t)
}

func TestWorkspaceDeleteRefusesSwappedSymlink(t *testing.T) {
	f := newTOCTOUFixture(t)
	cp := f.resolveThenSwap(t, "run/sub/victim.txt")
	if err := wsRemoveConfined(cp); err == nil {
		t.Fatal("delete through a swapped symlink succeeded")
	}
	f.assertOutsideIntact(t)

	// The swapped directory itself is removed as a link; its target stays.
	f = newTOCTOUFixture(t)
	cp = f.resolveThenSwap(t, "run/sub")
	if err := wsRemoveConfined(cp); err != nil {
		t.Fatalf("removing the link itself: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(f.root, "run", "sub")); !os.IsNotExist(err) {
		t.Fatalf("the link is still there: %v", err)
	}
	f.assertOutsideIntact(t)
}

func TestWorkspaceReadRefusesSwappedSymlink(t *testing.T) {
	f := newTOCTOUFixture(t)
	cp := f.resolveThenSwap(t, "run/sub/victim.txt")
	if fh, err := wsOpenConfined(cp); err == nil {
		fh.Close()
		t.Fatal("open through a swapped symlink succeeded")
	}
}

func TestWorkspaceUploadRefusesSwappedSymlink(t *testing.T) {
	f := newTOCTOUFixture(t)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "victim.txt")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(fw, "PWNED")
	mw.Close()
	req := httptest.NewRequest("POST", "/", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	cp := f.resolveThenSwap(t, "run/sub/victim.txt")
	if err := wsSaveUploadConfined(req.MultipartForm.File["file"][0], cp); err == nil {
		t.Fatal("upload through a swapped symlink succeeded")
	}
	f.assertOutsideIntact(t)
}

// Without a swap the confined operations act as before.
func TestWorkspaceConfinedOpsWork(t *testing.T) {
	f := newTOCTOUFixture(t)
	cp, _ := f.s.wsResolveConfined("run/new/dir/file.txt")
	if err := wsWriteConfined(cp, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(f.root, "run", "new", "dir", "file.txt")); string(data) != "hello" {
		t.Fatalf("content = %q", data)
	}
	dir, _ := f.s.wsResolveConfined("run/new")
	if err := wsWriteConfined(dir, []byte("x")); err != errWorkspaceIsDir {
		t.Fatalf("write to a directory: %v, want errWorkspaceIsDir", err)
	}
	if err := wsRemoveConfined(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "run", "new")); !os.IsNotExist(err) {
		t.Fatalf("directory not removed: %v", err)
	}
	rootCP, _ := f.s.wsResolveConfined("")
	if err := wsRemoveConfined(rootCP); err == nil {
		t.Fatal("removing the workspace root succeeded")
	}
}
