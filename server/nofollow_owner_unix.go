//go:build unix

package server

import (
	"io/fs"
	"os"
	"syscall"
)

// keepOwner gives f the owner and group of the entry fi describes, so a file
// replaced by replaceFileNoFollow stays writable by whoever owned it (an agent's
// file in its run directory). Best effort: a failure keeps the server's owner.
func keepOwner(f *os.File, fi fs.FileInfo) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		_ = f.Chown(int(st.Uid), int(st.Gid))
	}
}
