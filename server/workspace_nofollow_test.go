package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// inRootFixture is a workspace root whose run/sub directory an agent swaps,
// after the server checked a path below it, for a symlink that stays inside the
// root: to the hidden traffic/_ca subtree or to another workspace directory.
// Both targets hold a victim.txt the server must neither read, change nor remove.
type inRootFixture struct {
	s         *Server
	root      string
	ca, other string // in-root swap targets
}

const victimContent = "ORIGINAL"

func newInRootFixture(t *testing.T) *inRootFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "data")
	f := &inRootFixture{
		s:     &Server{m: &Manager{dir: root}},
		root:  root,
		ca:    filepath.Join(root, "traffic", "_ca"),
		other: filepath.Join(root, "other"),
	}
	for _, d := range []string{filepath.Join(root, "run", "sub"), f.ca, f.other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{f.ca, f.other} {
		if err := os.WriteFile(filepath.Join(d, "victim.txt"), []byte(victimContent), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "run", "sub", "victim.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// resolveThenSwap checks rel the way the handlers do, then replaces run/sub
// with a symlink to target.
func (f *inRootFixture) resolveThenSwap(t *testing.T, rel, target string) confinedPath {
	t.Helper()
	cp, ok := f.s.wsResolveConfined(rel)
	if !ok {
		t.Fatalf("%q refused before the swap", rel)
	}
	sub := filepath.Join(f.root, "run", "sub")
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, target, sub)
	return cp
}

// assertIntact checks that target still holds only the original victim.txt.
func assertIntact(t *testing.T, target string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(target, "victim.txt"))
	if err != nil || string(data) != victimContent {
		t.Fatalf("%s/victim.txt changed: %q %v", target, data, err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 1 {
		t.Fatalf("%s holds %v (%v), want only victim.txt", target, entries, err)
	}
}

// forEachTarget runs fn on a fresh fixture for each in-root swap target.
func forEachTarget(t *testing.T, fn func(t *testing.T, f *inRootFixture, target string)) {
	for _, name := range []string{"traffic/_ca", "other"} {
		t.Run(name, func(t *testing.T) {
			f := newInRootFixture(t)
			target := f.ca
			if name == "other" {
				target = f.other
			}
			fn(t, f, target)
			assertIntact(t, target)
		})
	}
}

func uploadHeader(t *testing.T, name, content string) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(fw, content)
	mw.Close()
	req := httptest.NewRequest("POST", "/", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	return req.MultipartForm.File["file"][0]
}

func TestWorkspaceInRootSwapWrite(t *testing.T) {
	forEachTarget(t, func(t *testing.T, f *inRootFixture, target string) {
		if err := wsWriteConfined(f.resolveThenSwap(t, "run/sub/victim.txt", target), []byte("PWNED")); err == nil {
			t.Fatal("write through an in-root swapped symlink succeeded")
		}
	})
	forEachTarget(t, func(t *testing.T, f *inRootFixture, target string) {
		if err := wsWriteConfined(f.resolveThenSwap(t, "run/sub/new/planted.txt", target), []byte("PWNED")); err == nil {
			t.Fatal("write of a new nested file through an in-root swapped symlink succeeded")
		}
	})
}

func TestWorkspaceInRootSwapRead(t *testing.T) {
	forEachTarget(t, func(t *testing.T, f *inRootFixture, target string) {
		if fh, err := wsOpenConfined(f.resolveThenSwap(t, "run/sub/victim.txt", target)); err == nil {
			fh.Close()
			t.Fatal("read through an in-root swapped symlink succeeded")
		}
	})
}

func TestWorkspaceInRootSwapMkdir(t *testing.T) {
	forEachTarget(t, func(t *testing.T, f *inRootFixture, target string) {
		if err := wsMkdirConfined(f.resolveThenSwap(t, "run/sub/newdir", target)); err == nil {
			t.Fatal("mkdir through an in-root swapped symlink succeeded")
		}
	})
}

func TestWorkspaceInRootSwapRemove(t *testing.T) {
	forEachTarget(t, func(t *testing.T, f *inRootFixture, target string) {
		if err := wsRemoveConfined(f.resolveThenSwap(t, "run/sub/victim.txt", target)); err == nil {
			t.Fatal("remove through an in-root swapped symlink succeeded")
		}
	})
	// The swapped directory itself is removed as a link; its target stays.
	forEachTarget(t, func(t *testing.T, f *inRootFixture, target string) {
		if err := wsRemoveConfined(f.resolveThenSwap(t, "run/sub", target)); err != nil {
			t.Fatalf("removing the link itself: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(f.root, "run", "sub")); !os.IsNotExist(err) {
			t.Fatalf("the link is still there: %v", err)
		}
	})
}

func TestWorkspaceInRootSwapList(t *testing.T) {
	forEachTarget(t, func(t *testing.T, f *inRootFixture, target string) {
		if ents, err := wsReadDirConfined(f.resolveThenSwap(t, "run/sub", target)); err == nil {
			t.Fatalf("list through an in-root swapped symlink succeeded: %d entries", len(ents))
		}
	})
}

func TestWorkspaceInRootSwapUpload(t *testing.T) {
	forEachTarget(t, func(t *testing.T, f *inRootFixture, target string) {
		hdr := uploadHeader(t, "victim.txt", "PWNED")
		if err := wsSaveUploadConfined(hdr, f.resolveThenSwap(t, "run/sub/victim.txt", target)); err == nil {
			t.Fatal("upload through an in-root swapped symlink succeeded")
		}
	})
}

// A leaf swapped for a symlink after the check is refused, not followed.
func TestWorkspaceLeafSwapRefused(t *testing.T) {
	forEachTarget(t, func(t *testing.T, f *inRootFixture, target string) {
		cp, _ := f.s.wsResolveConfined("run/sub/victim.txt")
		leaf := filepath.Join(f.root, "run", "sub", "victim.txt")
		if err := os.Remove(leaf); err != nil {
			t.Fatal(err)
		}
		mustSymlink(t, filepath.Join(target, "victim.txt"), leaf)
		if err := wsWriteConfined(cp, []byte("PWNED")); err == nil {
			t.Fatal("write through a swapped leaf symlink succeeded")
		}
		if fh, err := wsOpenConfined(cp); err == nil {
			fh.Close()
			t.Fatal("read through a swapped leaf symlink succeeded")
		}
		if err := wsSaveUploadConfined(uploadHeader(t, "victim.txt", "PWNED"), cp); err == nil {
			t.Fatal("upload through a swapped leaf symlink succeeded")
		}
	})
}

// Without a swap, nested paths work for every operation.
func TestWorkspaceNoFollowNestedOpsWork(t *testing.T) {
	f := newInRootFixture(t)
	cp, _ := f.s.wsResolveConfined("run/a/b/c/file.txt")
	if err := wsWriteConfined(cp, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := wsWriteConfined(cp, []byte("two")); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	fh, err := wsOpenConfined(cp)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(fh)
	fh.Close()
	if string(data) != "two" {
		t.Fatalf("content = %q", data)
	}
	mk, _ := f.s.wsResolveConfined("run/a/b/d/e")
	if err := wsMkdirConfined(mk); err != nil {
		t.Fatal(err)
	}
	if err := wsMkdirConfined(mk); err != nil {
		t.Fatalf("mkdir of an existing directory: %v", err)
	}
	up, _ := f.s.wsResolveConfined("run/a/b/up.txt")
	if err := wsSaveUploadConfined(uploadHeader(t, "up.txt", "uploaded"), up); err != nil {
		t.Fatal(err)
	}
	dir, _ := f.s.wsResolveConfined("run/a/b")
	ents, err := wsReadDirConfined(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if got := filepath.Join(names...); got != filepath.Join("c", "d", "up.txt") {
		t.Fatalf("entries = %v (temporary files must not remain)", names)
	}
	rootCP, _ := f.s.wsResolveConfined("")
	if _, err := wsReadDirConfined(rootCP); err != nil {
		t.Fatalf("listing the root: %v", err)
	}
	if err := wsRemoveConfined(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "run", "a", "b")); !os.IsNotExist(err) {
		t.Fatalf("directory not removed: %v", err)
	}
	// A symlink that was there at check time is still served through its real
	// path, which has no link on it.
	mustSymlink(t, filepath.Join(f.root, "other"), filepath.Join(f.root, "run", "alias"))
	alias, ok := f.s.wsResolveConfined("run/alias/victim.txt")
	if !ok {
		t.Fatal("in-root alias refused")
	}
	if fh, err := wsOpenConfined(alias); err != nil {
		t.Fatalf("read through a pre-existing alias: %v", err)
	} else {
		fh.Close()
	}
}
