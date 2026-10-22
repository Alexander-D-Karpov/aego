package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

var boot struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

func DefaultBootPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "aego", "logs", "boot.log")
}

func StartBoot(path string, lines ...string) error {
	boot.mu.Lock()
	defer boot.mu.Unlock()
	if boot.f != nil {
		boot.f.Close()
		boot.f, boot.path = nil, ""
	}
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_ = os.Rename(path, strings.TrimSuffix(path, ".log")+".prev.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	boot.f, boot.path = f, path
	for _, l := range lines {
		writeBoot("start", l)
	}
	return nil
}

func BootPath() string {
	boot.mu.Lock()
	defer boot.mu.Unlock()
	return boot.path
}

func Boot(stage, detail string) {
	boot.mu.Lock()
	defer boot.mu.Unlock()
	writeBoot(stage, detail)
}

func writeBoot(stage, detail string) {
	if boot.f == nil {
		return
	}
	boot.f.WriteString(time.Now().Format("15:04:05.000") + " " + stage + ": " + detail + "\n")
	boot.f.Sync()
}

func FatalText(title, msg string) string {
	text := title + " cannot start: " + msg
	if p := BootPath(); p != "" {
		text += "\n\nDetails: " + p
	}
	return text
}

func RecoverFatal(title string) {
	r := recover()
	if r == nil {
		return
	}
	msg := fmt.Sprint(r)
	Boot("panic", msg)
	Boot("stack", string(debug.Stack()))
	text := FatalText(title, msg)
	if !ShowFatal(title, text) {
		fmt.Fprintln(os.Stderr, text)
	}
	os.Exit(2)
}
