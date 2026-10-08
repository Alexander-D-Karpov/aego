package diag

import (
	"bytes"
	"fmt"
	"io"
	"sort"

	"aego/script/source"
)

type Severity uint8

const (
	Error Severity = iota
	Warning
	Info
)

func (s Severity) String() string {
	switch s {
	case Warning:
		return "warning"
	case Info:
		return "info"
	}
	return "error"
}

type Note struct {
	Span    source.Span
	Message string
}

type Diagnostic struct {
	Code     string
	Severity Severity
	Message  string
	Primary  source.Span
	Notes    []Note
}

func sprintf(format string, args []any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

func Errorf(code string, span source.Span, format string, args ...any) Diagnostic {
	return Diagnostic{Code: code, Severity: Error, Message: sprintf(format, args), Primary: span}
}

func Warnf(code string, span source.Span, format string, args ...any) Diagnostic {
	return Diagnostic{Code: code, Severity: Warning, Message: sprintf(format, args), Primary: span}
}

func (d Diagnostic) WithNote(span source.Span, format string, args ...any) Diagnostic {
	notes := make([]Note, len(d.Notes), len(d.Notes)+1)
	copy(notes, d.Notes)
	d.Notes = append(notes, Note{Span: span, Message: sprintf(format, args)})
	return d
}

func HasErrors(ds []Diagnostic) bool {
	for i := range ds {
		if ds[i].Severity == Error {
			return true
		}
	}
	return false
}

func Count(ds []Diagnostic) (errors, warnings int) {
	for i := range ds {
		switch ds[i].Severity {
		case Error:
			errors++
		case Warning:
			warnings++
		}
	}
	return errors, warnings
}

func Sort(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i].Primary, ds[j].Primary
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return ds[i].Severity < ds[j].Severity
	})
}

func Print(w io.Writer, sm *source.Manager, ds []Diagnostic) {
	var b bytes.Buffer
	for i := range ds {
		write(&b, sm, &ds[i])
	}
	w.Write(b.Bytes())
}

func Format(sm *source.Manager, d Diagnostic) string {
	var b bytes.Buffer
	write(&b, sm, &d)
	return b.String()
}

func location(sm *source.Manager, sp source.Span) (string, int, int, bool) {
	if sm == nil || !sp.Valid() {
		return "", 0, 0, false
	}
	line, col := sm.LineCol(sp.File, sp.Start)
	return sm.Path(sp.File), line, col, line > 0
}

func write(b *bytes.Buffer, sm *source.Manager, d *Diagnostic) {
	path, line, col, ok := location(sm, d.Primary)
	if ok {
		fmt.Fprintf(b, "%s:%d:%d: ", path, line, col)
	}
	if d.Code != "" {
		fmt.Fprintf(b, "%s %s: %s\n", d.Severity, d.Code, d.Message)
	} else {
		fmt.Fprintf(b, "%s: %s\n", d.Severity, d.Message)
	}
	if ok {
		snippet(b, sm, d.Primary, line, col)
	}
	for _, n := range d.Notes {
		np, nl, nc, nok := location(sm, n.Span)
		if !nok {
			fmt.Fprintf(b, "  note: %s\n", n.Message)
			continue
		}
		fmt.Fprintf(b, "  %s:%d:%d: note: %s\n", np, nl, nc, n.Message)
		snippet(b, sm, n.Span, nl, nc)
	}
}

func snippet(b *bytes.Buffer, sm *source.Manager, sp source.Span, line, col int) {
	text := sm.Line(sp.File, line)
	if len(bytes.TrimSpace([]byte(text))) == 0 {
		return
	}
	b.WriteString("    ")
	b.WriteString(text)
	b.WriteString("\n    ")
	runes := []rune(text)
	for i := 0; i < col-1 && i < len(runes); i++ {
		if runes[i] == '\t' {
			b.WriteByte('\t')
		} else {
			b.WriteByte(' ')
		}
	}
	width := 1
	if endLine, endCol := sm.LineCol(sp.File, sp.End); endLine == line && endCol > col {
		width = endCol - col
	}
	b.WriteByte('^')
	for i := 1; i < width; i++ {
		b.WriteByte('~')
	}
	b.WriteByte('\n')
}
