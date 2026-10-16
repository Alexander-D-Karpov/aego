package source

import (
	"sort"
	"sync"
	"unicode/utf8"
)

type FileID uint32

const NoFile FileID = 0

type Span struct {
	File       FileID
	Start, End uint32
}

func (s Span) Valid() bool {
	return s.File != NoFile
}

func (s Span) Len() uint32 {
	return s.End - s.Start
}

func (s Span) Contains(off uint32) bool {
	return off >= s.Start && off < s.End
}

func (s Span) To(o Span) Span {
	if s.File != o.File {
		return s
	}
	return Span{File: s.File, Start: min(s.Start, o.Start), End: max(s.End, o.End)}
}

type file struct {
	path  string
	src   []byte
	lines []uint32
}

func (f *file) lineEnd(i int) uint32 {
	if i+1 < len(f.lines) {
		return f.lines[i+1] - 1
	}
	return uint32(len(f.src))
}

type Manager struct {
	mu       sync.RWMutex
	files    []*file
	byPath   map[string]FileID
	interner *Interner
}

func NewManager() *Manager {
	return &Manager{byPath: map[string]FileID{}, interner: NewInterner()}
}

// Interner returns the project-wide identifier interner for lexer.New.
func (m *Manager) Interner() *Interner {
	return m.interner
}

func lineStarts(src []byte) []uint32 {
	lines := make([]uint32, 1, 1+len(src)/32)
	for i, c := range src {
		if c == '\n' {
			lines = append(lines, uint32(i+1))
		}
	}
	return lines
}

func (m *Manager) Add(path string, src []byte) FileID {
	f := &file{path: path, src: src, lines: lineStarts(src)}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.byPath[path]; ok {
		m.files[id-1] = f
		return id
	}
	m.files = append(m.files, f)
	id := FileID(len(m.files))
	m.byPath[path] = id
	return id
}

func (m *Manager) get(id FileID) *file {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id == NoFile || int(id) > len(m.files) {
		return nil
	}
	return m.files[id-1]
}

func (m *Manager) Lookup(path string) (FileID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.byPath[path]
	return id, ok
}

func (m *Manager) Path(id FileID) string {
	if f := m.get(id); f != nil {
		return f.path
	}
	return ""
}

func (m *Manager) Source(id FileID) []byte {
	if f := m.get(id); f != nil {
		return f.src
	}
	return nil
}

func (m *Manager) Files() []FileID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]FileID, len(m.files))
	for i := range m.files {
		out[i] = FileID(i + 1)
	}
	return out
}

func (m *Manager) LineCount(id FileID) int {
	if f := m.get(id); f != nil {
		return len(f.lines)
	}
	return 0
}

func (m *Manager) LineCol(id FileID, off uint32) (int, int) {
	f := m.get(id)
	if f == nil {
		return 0, 0
	}
	if int(off) > len(f.src) {
		off = uint32(len(f.src))
	}
	i := sort.Search(len(f.lines), func(i int) bool { return f.lines[i] > off }) - 1
	start := f.lines[i]
	for off > start && int(off) < len(f.src) && !utf8.RuneStart(f.src[off]) {
		off--
	}
	return i + 1, utf8.RuneCount(f.src[start:off]) + 1
}

func (m *Manager) Offset(id FileID, line, col int) uint32 {
	f := m.get(id)
	if f == nil || line < 1 {
		return 0
	}
	if line > len(f.lines) {
		return uint32(len(f.src))
	}
	off := f.lines[line-1]
	end := f.lineEnd(line - 1)
	for c := 1; c < col && off < end; c++ {
		_, size := utf8.DecodeRune(f.src[off:end])
		off += uint32(size)
	}
	return off
}

func (m *Manager) Line(id FileID, line int) string {
	f := m.get(id)
	if f == nil || line < 1 || line > len(f.lines) {
		return ""
	}
	text := f.src[f.lines[line-1]:f.lineEnd(line-1)]
	if n := len(text); n > 0 && text[n-1] == '\r' {
		text = text[:n-1]
	}
	return string(text)
}
