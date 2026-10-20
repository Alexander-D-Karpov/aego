package playground

import (
	"testing"

	"aego/script/diag"
)

func TestDiagnosticExamples(t *testing.T) {
	p := open(t)
	var skipped []string
	for _, code := range diag.Codes() {
		d, err := p.Reproduce(code)
		if err != nil {
			t.Errorf("%s: %v", code, err)
			continue
		}
		if d.Code == "" {
			skipped = append(skipped, code)
			continue
		}
		t.Logf("%s %s:%d:%d-%d:%d %s %s", code, d.File, d.Line, d.Col, d.EndLine, d.EndCol, d.Severity, d.Message)
	}
	if len(skipped) > 10 {
		t.Errorf("%d codes are not reproducible: %v", len(skipped), skipped)
	}
}
