package shaderx

import (
	"aego/render/shader"
	"strings"
	"testing"
)

func TestPostFXSourceABI(t *testing.T) {
	for _, decl := range []string{"#pragma postfx", "#pragma domain postfx"} {
		u, diag := Parse([]byte(head+decl+"\n#pragma material\nuniform float Strength;\n#pragma end\nvoid fragment(){ aego_out=aego_sample(aego_uv)*Strength; }\n"), "fx.aeshader", nil)
		if len(diag) > 0 || u.Domain != shader.DomainPostFX || u.Features != 0 {
			t.Fatalf("%s: %v", decl, diag)
		}
		for _, es := range []bool{false, true} {
			vert, _ := u.Source(false, es, 0)
			frag, locs := u.Source(true, es, 0)
			if strings.Contains(vert, "aego_i_pos_size") || strings.Count(frag, "uniform sampler2D tex0;") != 1 || !strings.Contains(frag, "float Strength;") || len(locs) != strings.Count(frag, "\n") {
				t.Fatal("invalid fullscreen shader source/map")
			}
		}
	}
}
