//go:build linux

package server

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// setNoNewPrivs keeps a dropped process from gaining privileges through execve
// (setuid or file-capability programs), even without Docker's no-new-privileges.
func setNoNewPrivs() error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}
	return nil
}
