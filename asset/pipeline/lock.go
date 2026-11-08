package pipeline

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const indexLock = "index.lock"

func acquireIndexLock(dir string) (func(), bool) {
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
			return func() { removeUnchangedLock(path, info, data) }, true
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
