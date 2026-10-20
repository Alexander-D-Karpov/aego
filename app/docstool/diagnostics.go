package docstool

import (
	"aego/app/playground"
	"aego/engine/docs"
	"aego/script/diag"
)

func diagnoser(pg *playground.Playground) func(string, *docs.DiagInfo) {
	return func(code string, d *docs.DiagInfo) {
		got, err := pg.Reproduce(code)
		if err != nil || got.Code == "" {
			return
		}
		d.Severity, d.At, d.Message = got.Severity, got.File, got.Message
		d.Line, d.Col, d.EndLine, d.EndCol = got.Line, got.Col, got.EndLine, got.EndCol
	}
}

// DiagnosticProblems lists codes whose example is missing or no longer produces the code.
func DiagnosticProblems() []docs.Problem {
	pg, err := playground.New()
	if err != nil {
		return []docs.Problem{{ID: docs.DiagnosticsID, Error: true, Message: "cannot start the docs runtime: " + err.Error()}}
	}
	var out []docs.Problem
	for _, c := range diag.Codes() {
		if _, err := pg.Reproduce(c); err != nil {
			out = append(out, docs.Problem{ID: c, Error: true, Message: err.Error()})
		}
	}
	return out
}
