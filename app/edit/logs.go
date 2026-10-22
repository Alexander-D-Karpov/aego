package edit

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"aego/core/log"
)

const keepLogs = 10

type logFile struct {
	f *os.File
}

func LogsDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "aego", "logs")
}

func attachLogs(cfg *Config) *logFile {
	if cfg.Logger == nil {
		cfg.Logger = log.New(log.NewTextSink(os.Stderr))
		if cfg.Debug {
			cfg.Logger.SetLevel(log.Debug)
		}
	}
	dir := LogsDir()
	if dir == "" || os.MkdirAll(dir, 0o755) != nil {
		return &logFile{}
	}
	name := "editor-" + time.Now().Format("20060102-150405") + "-" + strconv.Itoa(os.Getpid()) + ".log"
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return &logFile{}
	}
	cfg.Logger.AddSink(log.NewTextSink(f))
	pruneLogs(dir)
	return &logFile{f: f}
}

func (l *logFile) writer(w io.Writer) io.Writer {
	if l.f == nil {
		return w
	}
	return io.MultiWriter(w, l.f)
}

func (l *logFile) close() {
	if l.f != nil {
		l.f.Close()
	}
}

func pruneLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for i := 0; i+keepLogs < len(names); i++ {
		os.Remove(filepath.Join(dir, names[i]))
	}
}
