package docstool

import (
	"bytes"
	"testing"
)

func TestPreparedLightingTryAndRealDiagnosticExamples(t *testing.T) {
	reg, err := Registry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if bad := CheckExamples(reg, &out); bad != 0 {
		t.Fatalf("diagnostic examples: %s", out.String())
	}
	out.Reset()
	if bad := CheckTry(reg, &out); bad != 0 {
		t.Fatalf("Try scenes: %s", out.String())
	}
}
