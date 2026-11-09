package pipeline

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const indexLock = "index.lock"

// index.guard is persistent: unlinking a held advisory lock would let another
// process lock a different inode. Retain index.lock as the PID marker used by
// older editors. These checks reject a visible replacement, but cannot provide
// atomic compare-and-open against a hostile process changing the directory.
func openIndexGuard(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if os.IsExist(err) {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("index guard is not a regular file")
		}
		f, err = os.OpenFile(path, os.O_RDWR, 0)
	}
	if err != nil {
		return nil, err
	}
	if err := checkIndexGuard(path, f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func checkIndexGuard(path string, f *os.File) error {
	held, err := f.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !held.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(held, current) {
		return fmt.Errorf("index guard changed or is not a regular file")
	}
	return nil
}

func acquireIndexLock(dir string) (func(), bool) {
	guardPath := filepath.Join(dir, "index.guard")
	guard, err := openIndexGuard(guardPath)
	if err != nil {
		return func() {}, false
	}
	if err := lockIndexGuard(guard); err != nil {
		guard.Close()
		return func() {}, false
	}
	retained := false
	defer func() {
		if !retained {
			unlockIndexGuard(guard)
			guard.Close()
		}
	}()
	if err := checkIndexGuard(guardPath, guard); err != nil {
		return func() {}, false
	}
	path := filepath.Join(dir, indexLock)
	for range 2 {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			host, _ := os.Hostname()
			data := []byte(fmt.Sprintf("%d\n%s\n", os.Getpid(), host))
			_, writeErr := f.Write(data)
			info, statErr := f.Stat()
			closeErr := f.Close()
			if writeErr != nil || statErr != nil || closeErr != nil {
				// A partially written lock is not proof of ownership and may need
				// manual inspection; never remove an unknown replacement here.
				return func() {}, false
			}
			retained = true
			var once sync.Once
			return func() {
				once.Do(func() {
					removeUnchangedLock(path, info, data)
					unlockIndexGuard(guard)
					guard.Close()
				})
			}, true
		}
		if !os.IsExist(err) {
			// Failure to create a lock is not ownership. In particular, recovery
			// must never write sources while another owner may be active.
			return func() {}, false
		}
		info, data, stale := staleLock(path)
		if !stale {
			if _, err := os.Lstat(path); os.IsNotExist(err) {
				// The owner may have released it between O_EXCL and inspection.
				continue
			}
			return func() {}, false
		}
		if !removeUnchangedLock(path, info, data) {
			return func() {}, false
		}
	}
	return func() {}, false
}

func staleLock(path string) (os.FileInfo, []byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false
	}
	pid, host, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	n, err := strconv.Atoi(strings.TrimSpace(pid))
	if err != nil || n <= 0 {
		// Another process may have created this file but not yet written its
		// PID. An invalid or partial lock must not be assumed stale.
		return nil, nil, false
	}
	me, _ := os.Hostname()
	if strings.TrimSpace(host) != me {
		return nil, nil, false
	}
	return info, data, n != os.Getpid() && !processAlive(n)
}

// The inode and byte checks are best-effort protection against replacing an
// active lock. The final remove is not an atomic compare-and-delete.
func removeUnchangedLock(path string, expected os.FileInfo, data []byte) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(expected, info) {
		return false
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, data) {
		return false
	}
	after, err := os.Lstat(path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(info, after) {
		return false
	}
	return os.Remove(path) == nil
}
