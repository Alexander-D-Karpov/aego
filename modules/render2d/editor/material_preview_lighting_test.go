package editor

import (
	"testing"

	"aego/render/lighting"
)

func TestMaterialPreviewLightsAreValidAndSessionLocal(t *testing.T) {
	a, b := &materialPreview{}, &materialPreview{}
	in := a.lightingInput()
	if !in.HasEnvironment || in.Environment.ColorMode != lighting.LDR || in.Environment.AmbientIntensity >= 1 || len(in.Lights) != 2 {
		t.Fatal("Lit preview still has only white ambient")
	}
	if err := in.Environment.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, light := range in.Lights {
		if err := light.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	b.lightingInput()
	if a.viewID == b.viewID || a.viewID.Scope == 0 {
		t.Fatal("material previews share mutable output identity")
	}
	id := a.viewID
	for range 5 {
		a.lightingInput()
	}
	if id != a.viewID || &a.lighting.Lights[0] == &b.lighting.Lights[0] {
		t.Fatal("lighting state changed identity or aliases another preview")
	}
}
