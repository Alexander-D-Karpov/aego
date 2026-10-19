package crash

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"aego/core/ids"
	"aego/core/log"
	"aego/core/trace"
	"aego/engine"
	"aego/engine/recorder"
	"aego/format/aetext"
)

const Dir = ".aego/crash"

type Ring struct {
	mu    sync.Mutex
	lines []string
	head  int
	n     int
}

func NewRing(n int) *Ring {
	return &Ring{lines: make([]string, n)}
}

func (r *Ring) Write(rec log.Record) {
	var b strings.Builder
	b.WriteString(strconv.FormatFloat(rec.Time, 'f', 3, 64) + " " + rec.Level.String() + " [" + string(rec.Channel) + "] " + rec.Msg)
	for _, f := range rec.Fields {
		b.WriteString(" " + f.Key + "=" + fmt.Sprint(f.Val))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.n < len(r.lines) {
		r.lines[(r.head+r.n)%len(r.lines)] = b.String()
		r.n++
		return
	}
	r.lines[r.head] = b.String()
	r.head = (r.head + 1) % len(r.lines)
}

func (r *Ring) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, r.n)
	for i := range out {
		out[i] = r.lines[(r.head+i)%len(r.lines)]
	}
	return out
}

type Bundle struct {
	Root   string
	Dir    string
	Kind   string
	Where  string
	Panic  any
	Stack  []byte
	Script string
	Trace  *trace.Tracer
	Events *recorder.Recorder
	Names  func(ids.NodeID) string
	Scene  []byte
	Log    *Ring
	Info   [][2]string
}

func sanitizer() func(string) string {
	home, _ := os.UserHomeDir()
	return func(s string) string {
		if home != "" {
			s = strings.ReplaceAll(s, home, "~")
		}
		return s
	}
}

func Write(b Bundle) (string, error) {
	clean := sanitizer()
	parent := b.Dir
	if parent == "" {
		parent = filepath.Join(b.Root, filepath.FromSlash(Dir))
	}
	dir := filepath.Join(parent, time.Now().Format("20060102-150405.000"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	put := func(name string, data []byte) {
		if len(data) > 0 {
			os.WriteFile(filepath.Join(dir, name), []byte(clean(string(data))), 0o644)
		}
	}
	info := &aetext.Node{Head: "crash"}
	field := func(k, v string) {
		info.Fields = append(info.Fields, aetext.Field{Name: k, Value: aetext.String(clean(v))})
	}
	field("kind", b.Kind)
	field("where", b.Where)
	field("time", time.Now().Format(time.RFC3339))
	if b.Panic != nil {
		field("message", fmt.Sprint(b.Panic))
	}
	field("engine", engine.Version)
	field("go", runtime.Version())
	field("os", runtime.GOOS+"/"+runtime.GOARCH)
	for _, kv := range b.Info {
		field(kv[0], kv[1])
	}
	put("info.aetext", aetext.Write(info))
	put("go.stack", b.Stack)
	put("script.stack", []byte(b.Script))
	if b.Trace != nil && b.Trace.Frames() > 0 {
		var buf bytes.Buffer
		if trace.WriteChrome(&buf, b.Trace, b.Trace.Snapshot(b.Trace.Frames())) == nil {
			put("trace.json", buf.Bytes())
		}
	}
	if b.Events != nil {
		var buf bytes.Buffer
		for _, r := range b.Events.Snapshot(nil) {
			buf.WriteString(r.Line(b.Names) + "\n")
		}
		put("events.log", buf.Bytes())
	}
	put("scene.aescene", b.Scene)
	if b.Log != nil {
		put("log.txt", []byte(strings.Join(b.Log.Lines(), "\n")+"\n"))
	}
	return dir, nil
}
