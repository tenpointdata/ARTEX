package agent

import "sync/atomic"

// File tool identity. The confined file tools (filetools.go) and the ingest tools
// open the paths the agent names inside the server process, which is root in the
// Docker image. With the executor privilege drop active (server/execpriv.go) the
// server sets the executor identity here, and each of those calls then reaches the
// file system as that uid and gid, with that gid as its only group: the kernel
// checks every open, create and directory read against the executor's rights, the
// rights of the agent's shell, on top of the os.Root confinement. Linux only; see
// filetools_identity_linux.go.

// FileToolIdentity is the uid and gid the file tools access files as.
type FileToolIdentity struct {
	UID, GID uint32
	// Groups are the supplementary groups; empty means GID alone.
	Groups []uint32
	// For, when set, gives the identity for a call in the working directory dir
	// (one uid per task with server -exec-uid-range). It runs in the server.
	For func(dir string) FileToolIdentity
}

// groups returns the supplementary groups of id.
func (id FileToolIdentity) groups() []uint32 {
	if len(id.Groups) == 0 {
		return []uint32{id.GID}
	}
	return id.Groups
}

var fileToolIdentity atomic.Pointer[FileToolIdentity]

// SetFileToolIdentity sets the identity of the file tools; nil restores the
// server's own identity.
func SetFileToolIdentity(id *FileToolIdentity) { fileToolIdentity.Store(id) }

// FileToolIdentitySupported reports whether the file tools can run as another
// identity on this platform.
func FileToolIdentitySupported() bool { return fileIdentitySupported }

// RunAsFileToolIdentity runs fn with the file tool identity for a call in the
// working directory dir, or directly when none is set. When it returns an error, fn
// did not run.
func RunAsFileToolIdentity(dir string, fn func()) error {
	id := fileToolIdentity.Load()
	if id == nil {
		fn()
		return nil
	}
	eff := *id
	if id.For != nil {
		eff = id.For(dir)
	}
	return runWithFileIdentity(eff, fn)
}
