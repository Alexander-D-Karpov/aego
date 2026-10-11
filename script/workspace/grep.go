package workspace

import (
	"bytes"
	"regexp"
	"unicode/utf8"
)

type GrepOptions struct {
	Regexp bool
	Case   bool
	Word   bool
}

type Match struct {
	Path   string
	Line   int
	Col    int
	Offset int
	Len    int
	Text   string
}

func Pattern(q string, o GrepOptions) (*regexp.Regexp, error) {
	if !o.Regexp {
		q = regexp.QuoteMeta(q)
	}
	if o.Word {
		q = `\b(?:` + q + `)\b`
	}
	if !o.Case {
		q = "(?i)" + q
	}
	return regexp.Compile(q)
}

type content struct {
	path string
	src  []byte
}

func (w *Workspace) contents() []content {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]content, 0, len(w.order))
	for _, e := range w.order {
		switch {
		case e.hasOver:
			out = append(out, content{path: e.path, src: e.over})
		case e.onDisk:
			out = append(out, content{path: e.path, src: e.disk})
		}
	}
	return out
}

func (w *Workspace) Source(p string) ([]byte, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e, ok := w.files[w.rel(p)]
	switch {
	case !ok:
		return nil, false
	case e.hasOver:
		return e.over, true
	case e.onDisk:
		return e.disk, true
	}
	return nil, false
}

func (w *Workspace) Grep(q string, o GrepOptions, limit int) ([]Match, bool, error) {
	if q == "" {
		return nil, false, nil
	}
	re, err := Pattern(q, o)
	if err != nil {
		return nil, false, err
	}
	files := w.contents()
	var out []Match
	for _, f := range files {
		src := f.src
		for line, start := 1, 0; start <= len(src); line++ {
			end := bytes.IndexByte(src[start:], '\n')
			if end < 0 {
				end = len(src)
			} else {
				end += start
			}
			text := bytes.TrimSuffix(src[start:end], []byte{'\r'})
			for _, m := range re.FindAllIndex(text, -1) {
				if m[1] == m[0] {
					continue
				}
				out = append(out, Match{
					Path:   f.path,
					Line:   line,
					Col:    utf8.RuneCount(text[:m[0]]) + 1,
					Offset: m[0],
					Len:    m[1] - m[0],
					Text:   string(text),
				})
				if limit > 0 && len(out) >= limit {
					return out, true, nil
				}
			}
			start = end + 1
		}
	}
	return out, false, nil
}
