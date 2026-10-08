package console

import (
	"fmt"
	"strings"
	"sync"

	"aego/core/log"
)

type Entry struct {
	Time    float64
	Level   log.Level
	Channel log.Channel
	Text    string
	Path    string
	Count   int
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

func pathOf(r log.Record) string {
	for _, f := range r.Fields {
		if f.Key == "path" {
			if s, ok := f.Val.(string); ok {
				return s
			}
		}
	}
	return ""
}

func (s *Sink) Write(r log.Record) {
	text := format(r)
	p := pathOf(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(r.Level) < len(s.counts) {
		s.counts[r.Level]++
	}
	s.rev++
	if s.n > 0 {
		last := &s.ring[(s.start+s.n-1)%len(s.ring)]
		if last.Level == r.Level && last.Channel == r.Channel && last.Text == text {
			last.Count++
			last.Time = r.Time
			return
		}
	}
	e := Entry{Time: r.Time, Level: r.Level, Channel: r.Channel, Text: text, Path: p, Count: 1}
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
	dst = dst[:0]
	for i := 0; i < s.n; i++ {
		dst = append(dst, s.ring[(s.start+i)%len(s.ring)])
	}
	return dst
}

func (s *Sink) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.start, s.n = 0, 0
	s.counts = [log.Fatal + 1]int{}
	s.rev++
}
