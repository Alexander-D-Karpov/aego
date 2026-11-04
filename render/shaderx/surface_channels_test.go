package shaderx

import (
	"fmt"
	"strings"
	"testing"

	"aego/render/shader"
)

func TestSurfaceChannelsPreserveSixUserBindingsAndRejectAmbiguity(t *testing.T) {
	base := "#pragma aego abi 1\n#pragma domain sprite\n#pragma lighting forward2d\n#pragma material\n%s#pragma end\nvoid fragment(){aego_out=vec4(1.0);}\n"
	params := "uniform sampler2D Normal; // @normal\nuniform sampler2D Emission; // @emission\n"
	for i := 0; i < 6; i++ {
		params += fmt.Sprintf("uniform sampler2D User%d;\n", i)
	}
	u, diags := Parse([]byte(fmt.Sprintf(base, params)), "surface.aeshader", nil)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	if u.Features&shader.FeatureSurfaceChannels == 0 || u.Params[0].Slot != shader.TexNormal || u.Params[1].Slot != shader.TexEmission {
		t.Fatal("surface resources missing from ABI")
	}
	for i := 0; i < 6; i++ {
		if u.Params[i+2].Slot != shader.TexUser+i {
			t.Fatal("user sampler budget was reduced")
		}
	}
	for _, extra := range []string{
		"uniform sampler2D Extra;\n",
		"uniform sampler2D SecondEmission; // @emission\n",
		"uniform sampler2D Both; // @normal @emission\n",
		"uniform vec4 Wrong; // @emission\n",
	} {
		if _, issues := Parse([]byte(fmt.Sprintf(base, params+extra)), "bad.aeshader", nil); len(issues) == 0 {
			t.Fatalf("accepted invalid declarations: %s", extra)
		}
	}
	noLighting := strings.ReplaceAll(fmt.Sprintf(base, "uniform sampler2D Emission; // @emission\n"), "#pragma lighting forward2d\n", "")
	if _, issues := Parse([]byte(noLighting), "unlit.aeshader", nil); len(issues) == 0 {
		t.Fatal("Unlit silently consumed lighting reservations")
	}
}
