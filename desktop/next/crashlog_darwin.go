//go:build darwin

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

const canRedirectStderr = true

func redirectStderr(f *os.File) error {
	return unix.Dup2(int(f.Fd()), int(os.Stderr.Fd()))
}

func dupStderr() (*os.File, error) {
	fd, err := unix.Dup(int(os.Stderr.Fd()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "stderr"), nil
}
