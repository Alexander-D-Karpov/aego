//go:build !windows && !js

package pipeline

import (
	"errors"
	"os"
	"syscall"
)

func lockIndexGuard(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockIndexGuard(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
