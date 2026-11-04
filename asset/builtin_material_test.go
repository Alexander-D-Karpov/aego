package asset_test

import (
	"bytes"
	"slices"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/gpu"
	"aego/render/material"
	"aego/render/shader"
)

func TestBuiltinMaterialsArePreparedUserPipelineArtifacts(t *testing.T) {
	names := asset.BuiltinMaterialNames()
	if !slices.Equal(names, []string{"Unlit", "Additive", "Flash", "Dissolve", "Outline", "PixelSnap", "Lit", "LitMesh2D", "PostTint"}) {
		t.Fatal(names)
	}
	seen := map[ids.AssetID]bool{}
	for _, name := range names {
		mid, _ := asset.BuiltinMaterialNamed(name)
		sid, _ := asset.BuiltinShaderNamed(name)
		for _, id := range []ids.AssetID{mid, sid} {
			if seen[id] {
				t.Fatal("duplicate builtin ID")
			}
			seen[id] = true
			if _, ok := asset.LookupBuiltin(id); !ok {
				t.Fatalf("%s is not in the common asset registry", id.String())
			}
		}
		src, ok := asset.BuiltinShader(sid)
		if !ok {
			t.Fatalf("%s missing AESH", name)
		}
		sh, err := asset.DecodeShader(src)
		if err != nil {
			t.Fatal(err)
		}
		if sh.Validation != (shader.SchemaChecked | shader.ReferenceChecked) {
			t.Fatalf("%s falsely claims GPU validation or skipped reference checks: %b", name, sh.Validation)
		}
		if sh.Vertex != (name == "PixelSnap") {
			t.Fatalf("%s custom vertex policy", name)
		}
		lit := name == "Lit" || name == "LitMesh2D"
		if (sh.Features&shader.FeatureLighting != 0) != lit {
			t.Fatalf("%s lighting feature: %v", name, sh.Features)
		}
		if (sh.Domain == shader.DomainMesh2D) != (name == "LitMesh2D") || (sh.Domain == shader.DomainPostFX) != (name == "PostTint") {
			t.Fatalf("%s has an incompatible domain", name)
		}
		blob, ok := asset.BuiltinMaterial(mid)
		if !ok {
			t.Fatalf("%s missing AEMT", name)
		}
		doc, err := asset.DecodeMaterial(blob, name)
		if err != nil {
			t.Fatal(err)
		}
		if doc.Shader != sid {
			t.Fatalf("%s references another shader", name)
		}
		m, problems := doc.Build(name, sh.Runtime(sid, name), func(id ids.AssetID) (material.Tex, bool) {
			_, ok := asset.BuiltinTexture(id)
			return material.Tex{}, ok
		})
		if m == nil || len(problems) != 0 {
			t.Fatalf("%s: %v", name, problems)
		}
		if name == "Dissolve" && doc.Textures["Noise"] != asset.BuiltinMaterialNoise {
			t.Fatal("dissolve lost its data texture reference")
		}
		if name == "Additive" && m.Blend() != shader.BlendAdditive {
			t.Fatal("additive did not use the declared blend")
		}
	}
	if id, ok := asset.BuiltinMaterialNamed("Default"); !ok || id != asset.BuiltinMaterialUnlit {
		t.Fatal("Default must alias Unlit")
	}
}

func TestBuiltinNoiseIsDeterministicLinearData(t *testing.T) {
	a, ok := asset.BuiltinTexture(asset.BuiltinMaterialNoise)
	if !ok {
		t.Fatal("missing noise")
	}
	b, _ := asset.BuiltinTexture(asset.BuiltinMaterialNoise)
	if !bytes.Equal(a.Pixels, b.Pixels) || a.Width != 64 || a.Height != 64 || a.Format != gpu.TexRGBA8 || !a.NonColor {
		t.Fatal("noise uses color conversion or changes between reads")
	}
	seen := [256]bool{}
	for i := 0; i < len(a.Pixels); i += 4 {
		seen[a.Pixels[i]] = true
		if a.Pixels[i+3] != 255 {
			t.Fatal("noise alpha is not opaque")
		}
	}
	n := 0
	for _, v := range seen {
		if v {
			n++
		}
	}
	if n < 250 {
		t.Fatalf("noise only contains %d grayscale levels", n)
	}
}
