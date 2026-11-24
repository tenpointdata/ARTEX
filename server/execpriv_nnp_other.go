//go:build unix && !linux

package server

// setNoNewPrivs: no_new_privs is a Linux prctl; the drop is supported on Linux only.
func setNoNewPrivs() error { return nil }
