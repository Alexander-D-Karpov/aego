package document

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const LockFile = ".aego/editor.lock"

type LockInfo struct {
	PID  int
	Host string
	Time time.Time
}

func lockPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(LockFile))
}

func ReadLock(root string) (*LockInfo, bool) {
	data, err := os.ReadFile(lockPath(root))
	if err != nil {
		return nil, false
	}
	info := &LockInfo{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch k {
		case "pid":
			info.PID, _ = strconv.Atoi(v)
		case "host":
			info.Host = v
		case "time":
			info.Time, _ = time.Parse(time.RFC3339Nano, v)
		}
	}
	return info, info.PID != 0
}

func AcquireLock(root string) (func(), *LockInfo, error) {
	var other *LockInfo
	if info, ok := ReadLock(root); ok && info.PID != os.Getpid() && time.Since(info.Time) < 7*24*time.Hour {
		other = info
	}
	host, _ := os.Hostname()
	me := LockInfo{PID: os.Getpid(), Host: host, Time: time.Now()}
	data := "pid=" + strconv.Itoa(me.PID) + "\nhost=" + me.Host + "\ntime=" + me.Time.Format(time.RFC3339Nano) + "\n"
	if err := WriteAtomic(lockPath(root), []byte(data)); err != nil {
		return func() {}, other, err
	}
	release := func() {
		if cur, ok := ReadLock(root); ok && cur.PID == me.PID && cur.Host == me.Host {
			os.Remove(lockPath(root))
		}
	}
	return release, other, nil
}
