//go:build windows

package pipeline

import (
	"errors"
	"os"
	"syscall"
)

var (
	indexLockFile   = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFile")
	indexUnlockFile = syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFile")
)

func lockIndexGuard(f *os.File) error {
	ok, _, err := indexLockFile.Call(f.Fd(), 0, 0, 1, 0)
	if ok == 0 {
		if err == nil || errors.Is(err, syscall.Errno(0)) {
			return errors.New("unable to acquire index guard")
		}
		return err
	}
	return nil
}

func unlockIndexGuard(f *os.File) { _, _, _ = indexUnlockFile.Call(f.Fd(), 0, 0, 1, 0) }

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Failure to open a PID is not normally proof of death (access may be
	// denied); only the explicit invalid-PID result permits stale recovery.
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		const errorInvalidParameter syscall.Errno = 87
		return !errors.Is(err, errorInvalidParameter)
	}
	defer syscall.CloseHandle(h)
	status, err := syscall.WaitForSingleObject(h, 0)
	return err != nil || status != syscall.WAIT_OBJECT_0
}
