//go:build unix

package main

import (
	"fmt"
	"os"
	"syscall"
)

// openRegular opens path without following a trailing symbolic link and
// without blocking on a FIFO, then verifies that the open file is a
// regular file.
func openRegular(path string, flag int) (*os.File, os.FileInfo, error) {
	f, err := os.OpenFile(path, flag|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, nil, fmt.Errorf("%s: not a regular file", path)
	}
	return f, info, nil
}
