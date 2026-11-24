//go:build !unix

package server

import (
	"io/fs"
	"os"
)

// keepOwner is a no-op where file ownership is not a uid/gid pair.
func keepOwner(*os.File, fs.FileInfo) {}
