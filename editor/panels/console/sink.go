package console

import (
	"aego/editor/core/services"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"aego/core/log"
)

type Entry struct {
	Code, Cause, Asset string
	ID                 uint64
	Time               float64
	Level              log.Level
	Channel            log.Channel
	Text               string
	Path               string
	Line               int
	Count              int
}

type Sink struct {
	mu      sync.Mutex
	ring    []Entry
	start   int
	n       int
	rev     uint64
	counts  [log.Fatal + 1]int
	collect bool
}

func NewSink(capacity int) *Sink {
	if capacity < 16 {
		capacity = 16
	}
	return &Sink{ring: make([]Entry, capacity), collect: true}
}

func format(r log.Record) string {
	if len(r.Fields) == 0 {
		return r.Msg
	}
	var b strings.Builder
	b.WriteString(r.Msg)
	for _, f := range r.Fields {
		b.WriteByte(' ')
		b.WriteString(f.Key)
		b.WriteByte('=')
		b.WriteString(fmt.Sprint(f.Val))
	}
	return b.String()
}

var diagnosticCode = regexp.MustCompile(`\b[A-Z]{2,8}[0-9]{3,5}\b`)

func entryOf(r log.Record) Entry {
	e := Entry{Time: r.Time, Level: r.Level, Channel: r.Channel, Text: format(r), Count: 1}
	for _, f := range r.Fields {
		switch f.Key {
		case "path":
			if value, ok := f.Val.(string); ok && value != "" {
				e.Path = value
			}
		case "file":
			if e.Path == "" {
				e.Path, _ = f.Val.(string)
			}
		case "line":
			e.Line, _ = f.Val.(int)
		case "asset":
			e.Asset = fmt.Sprint(f.Val)
		case "code":
			e.Code = fmt.Sprint(f.Val)
		case "err":
			e.Cause = diagnosticCode.FindString(fmt.Sprint(f.Val))
		}
	}
	return e
}

func (e Entry) Message() services.ConsoleMessage {
	return services.ConsoleMessage{Time: e.Time, Level: e.Level.String(), Channel: string(e.Channel), Text: e.Text,
		Code: e.Code, Cause: e.Cause, Path: e.Path, Asset: e.Asset, Line: e.Line, Count: e.Count}
}

func (e Entry) details() string {
	data, _ := json.MarshalIndent(e.Message(), "", "  ")
	return string(data)
}
func (s *Sink) Write(r log.Record) {
	e := entryOf(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(r.Level) < len(s.counts) {
		s.counts[r.Level]++
	}
	s.rev++
	if s.n > 0 {
		last := &s.ring[(s.start+s.n-1)%len(s.ring)]
		if last.Level == r.Level && last.Channel == r.Channel && last.Text == e.Text {
			last.Count++
			last.Time = r.Time
			return
		}
	}
	e.ID = s.rev
	if s.n < len(s.ring) {
		s.ring[(s.start+s.n)%len(s.ring)] = e
		s.n++
		return
	}
	s.ring[s.start] = e
	s.start = (s.start + 1) % len(s.ring)
}

func (s *Sink) Revision() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rev
}

func (s *Sink) Counts() (info, warn, errors int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[log.Trace] + s.counts[log.Debug] + s.counts[log.Info], s.counts[log.Warn], s.counts[log.Error] + s.counts[log.Fatal]
}

func (s *Sink) Snapshot(dst []Entry) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(dst)
	dst = dst[:0]
	for i := 0; i < s.n; i++ {
		dst = append(dst, s.ring[(s.start+i)%len(s.ring)])
	}
	return dst
}

func (s *Sink) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.ring)
	s.start, s.n = 0, 0
	s.counts = [log.Fatal + 1]int{}
	s.rev++
}
