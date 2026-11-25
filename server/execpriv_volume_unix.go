//go:build unix

package server

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"syscall"
)

// tightenVolumeRoot strips group and other write from dir, the root of a volume
// the server was given (the data or key directory), and makes it owned by root.
// Container runtimes create fresh volumes world-writable: a Kubernetes emptyDir is
// 0777, and so is a PersistentVolume from many provisioners (kind's local-path,
// hostPath). The executor could then plant files at the top of the store, which
// the isolation self-test refuses. A Docker named volume copies the image's
// root-owned directory, so this is a no-op there. A missing dir is left alone; a
// symlink is refused, so a planted link never redirects the change.
func tightenVolumeRoot(dir string, maxPerm fs.FileMode) error {
	if dir == "" {
		return nil
	}
	fi, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return nil
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	perm := fi.Mode().Perm()
	want := perm & maxPerm
	if ok && st.Uid == 0 && perm == want && fi.Mode()&fs.ModeSticky == 0 {
		return nil
	}
	gid := -1
	if ok {
		gid = int(st.Gid)
		if perm&0o020 != 0 {
			gid = 0 // a writable group is not trusted; root's group takes over
		}
	}
	f, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chown(0, gid); err != nil {
		return err
	}
	if err := f.Chmod(want); err != nil {
		return err
	}
	log.Printf("[exec] %s was mode %v; it is now root-owned %v so agent processes cannot write in it", dir, fi.Mode(), want)
	return nil
}
