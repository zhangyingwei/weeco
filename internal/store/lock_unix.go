//go:build !windows

package store

import (
	"os"
	"syscall"
)

func lockRuntimeFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockRuntimeFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
