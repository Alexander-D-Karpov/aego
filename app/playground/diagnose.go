package playground

import (
	"errors"
	"fmt"
	"strings"

	"aego/script/diag"
)

const exampleFrames = 3

// Reproduce runs the diagnostics example of code and returns what its broken version reports.
func (p *Playground) Reproduce(code string) (Diagnostic, error) {
	ex, ok := diag.Example(code)
	if !ok {
		return Diagnostic{}, errors.New("no example in script/diag/examples.go")
	}
	if ex.Why != "" {
		return Diagnostic{}, nil
	}
	if ex.Bad == "" || ex.Good == "" {
		return Diagnostic{}, errors.New("the example needs both a broken and a fixed version")
	}
	bad := reports(p.Run(ex.Sources(false), exampleFrames))
	var found *Diagnostic
	for i := range bad {
		if bad[i].Code == code {
			found = &bad[i]
			break
		}
	}
	if found == nil {
		return Diagnostic{}, fmt.Errorf("the broken example does not report %s, it reports %s", code, list(bad))
	}
	var wrong []Diagnostic
	for _, d := range reports(p.Run(ex.Sources(true), exampleFrames)) {
		if d.Code == code || d.Severity == "error" {
			wrong = append(wrong, d)
		}
	}
	if len(wrong) > 0 {
		return *found, fmt.Errorf("the fixed example still reports %s", list(wrong))
	}
	return *found, nil
}

func reports(res Result) []Diagnostic {
	out := append([]Diagnostic(nil), res.Diagnostics...)
	for _, f := range res.Faults {
		out = append(out, Diagnostic{File: f.File, Line: f.Line, Code: f.Code, Severity: "error", Message: f.Message})
	}
	return out
}

func list(ds []Diagnostic) string {
	if len(ds) == 0 {
		return "nothing"
	}
	parts := make([]string, len(ds))
	for i, d := range ds {
		parts[i] = strings.TrimSpace(d.Code + " " + d.Message)
	}
	return strings.Join(parts, "; ")
}
