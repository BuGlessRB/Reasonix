//go:build linux

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// Dup3 rather than Dup2: linux/arm64 has no dup2 syscall at all, so the pair
// that exists everywhere Linux runs is this one.
const canRedirectStderr = true

func redirectStderr(f *os.File) error {
	return unix.Dup3(int(f.Fd()), int(os.Stderr.Fd()), 0)
}

func dupStderr() (*os.File, error) {
	fd, err := unix.Dup(int(os.Stderr.Fd()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "stderr"), nil
}
