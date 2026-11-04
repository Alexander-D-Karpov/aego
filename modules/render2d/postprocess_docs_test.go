package render2d

import (
	"os"
	"slices"
	"testing"

	"aego/engine/module"
	"aego/engine/problems"
	"aego/scene/sceneio"
)

func TestPostProcessDiagnosticExample(t *testing.T) {
	f := lightingTestWorld(t)
	data, err := os.ReadFile("res/docs/diagnostics/PFX001.aescene")
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := sceneio.Load(data, "PFX001.aescene", f.s.Types(), nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	ds := module.ValidateScene(s, nil)
	if !slices.ContainsFunc(ds, func(d problems.Diagnostic) bool { return d.Code == "PFX001" }) {
		t.Fatalf("documented example did not produce PFX001: %+v", ds)
	}
}
