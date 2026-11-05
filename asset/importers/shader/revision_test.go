package shader

import (
	"testing"

	"aego/render/shaderx"
)

func TestDiagnosticRevisionDoesNotAllocate(t *testing.T) {
	i := New()
	i.record("assets/a.aeshader", []shaderx.Diag{{Code: shaderx.SHD001}})
	_, before := i.Diagnostics()
	if before == 0 || i.Revision() != before {
		t.Fatal("diagnostic revision was not reported")
	}
	if n := testing.AllocsPerRun(100, func() { _ = i.Revision() }); n != 0 {
		t.Fatalf("checking the revision allocates %v times", n)
	}
	i.record("assets/a.aeshader", nil)
	if i.Revision() == before {
		t.Fatal("clearing a diagnostic did not advance the revision")
	}
}
