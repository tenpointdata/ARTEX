//go:build unix

package server

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A FIFO at the leaf is refused before open, so a read cannot block.
func TestOpenFileNoFollowRefusesFIFO(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := openFileNoFollow(root, "pipe"); !errors.Is(err, errNotRegularInPath) {
		t.Fatalf("got %v, want errNotRegularInPath", err)
	}
}
