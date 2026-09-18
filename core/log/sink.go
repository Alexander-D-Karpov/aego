package log

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

type TextSink struct {
	mu  sync.Mutex
	w   io.Writer
	buf []byte
}

func NewTextSink(w io.Writer) *TextSink {
	return &TextSink{w: w, buf: make([]byte, 0, 512)}
}

func (s *TextSink) Write(r Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.buf[:0]
	b = strconv.AppendFloat(b, r.Time, 'f', 3, 64)
	b = append(b, ' ')
	lv := r.Level.String()
	b = append(b, lv...)
	for i := len(lv); i < 5; i++ {
		b = append(b, ' ')
	}
	b = append(b, " ["...)
	b = append(b, r.Channel...)
	b = append(b, "] "...)
	b = append(b, r.Msg...)
	for _, f := range r.Fields {
		b = append(b, ' ')
		b = append(b, f.Key...)
		b = append(b, '=')
		b = appendValue(b, f.Val)
	}
	b = append(b, '\n')
	s.buf = b
	_, _ = s.w.Write(b)
}

func appendValue(b []byte, v any) []byte {
	switch x := v.(type) {
	case string:
		return appendString(b, x)
	case error:
		return appendString(b, x.Error())
	case fmt.Stringer:
		return appendString(b, x.String())
	case bool:
		return strconv.AppendBool(b, x)
	case int:
		return strconv.AppendInt(b, int64(x), 10)
	case int64:
		return strconv.AppendInt(b, x, 10)
	case uint32:
		return strconv.AppendUint(b, uint64(x), 10)
	case uint64:
		return strconv.AppendUint(b, x, 10)
	case float32:
		return strconv.AppendFloat(b, float64(x), 'g', 6, 32)
	case float64:
		return strconv.AppendFloat(b, x, 'g', 6, 64)
	}
	return fmt.Append(b, v)
}

func appendString(b []byte, s string) []byte {
	if s == "" || strings.ContainsAny(s, " \t\n\"=") {
		return strconv.AppendQuote(b, s)
	}
	return append(b, s...)
}

type FuncSink func(Record)

func (f FuncSink) Write(r Record) {
	f(r)
}

type MemorySink struct {
	mu      sync.Mutex
	records []Record
}

func (m *MemorySink) Write(r Record) {
	r.Fields = append([]Field(nil), r.Fields...)
	m.mu.Lock()
	m.records = append(m.records, r)
	m.mu.Unlock()
}

func (m *MemorySink) Records() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Record(nil), m.records...)
}
