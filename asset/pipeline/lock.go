package pipeline

import (
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
			fmt.Fprintf(f, "%d\n%s\n", os.Getpid(), host)
			f.Close()
			return func() { os.Remove(path) }, true
		}
		if !os.IsExist(err) {
			return func() {}, true
		}
		if !staleLock(path) {
			return func() {}, false
		}
		os.Remove(path)
	}
	return func() {}, false
}

func staleLock(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return os.IsNotExist(err)
	}
	pid, host, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	n, err := strconv.Atoi(strings.TrimSpace(pid))
	if err != nil {
		return true
	}
	me, _ := os.Hostname()
	if strings.TrimSpace(host) != me {
		return false
	}
	return n != os.Getpid() && !processAlive(n)
}
