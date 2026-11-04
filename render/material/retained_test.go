package material

import (
	"math"
	"strings"
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/shader"
)

func TestInitiallyUnknownValuesResurrectWithoutLosingLiveState(t *testing.T) {
	old := testShader("old", nil, []uint32{0}, pspec{"Speed", shader.KindFloat, [4]float32{1}, false})
	m, _ := New("material", old, 0, shader.BlendPremultiplied)
	m.Set("Speed", [4]float32{7})
	values := []Value{
		{Name: "FutureFloat", Kind: shader.KindFloat, Typed: true, N: 1, Lanes: [4]float32{0.25}},
		{Name: "FutureInt", Kind: shader.KindInt, Typed: true, N: 1, Int: math.MaxInt32},
		{Name: "FutureBool", Kind: shader.KindBool, Typed: true, N: 1, Lanes: [4]float32{1}},
		{Name: "FutureColor", Kind: shader.KindVec4, Typed: true, N: 4, Color: true, Lanes: [4]float32{0.5, 0.5, 0.5, 0.5}},
		{Name: "FutureEnum", Kind: shader.KindInt, Typed: true, N: 1, Symbol: "second"},
		{Name: "FutureTexture", Kind: shader.KindTexture, Typed: true, N: 0, Asset: true, Reference: ids.AssetID{Lo: 42}},
	}
	for _, v := range values {
		if !m.RetainUnknown(v, Tex{}, true) {
			t.Fatal(v.Name)
		}
	}
	next := testShader("next", nil, []uint32{0},
		pspec{"Speed", shader.KindFloat, [4]float32{1}, false},
		pspec{"FutureFloat", shader.KindFloat, [4]float32{}, false},
		pspec{"FutureInt", shader.KindInt, [4]float32{}, true},
		pspec{"FutureBool", shader.KindBool, [4]float32{}, true},
		pspec{"FutureColor", shader.KindVec4, [4]float32{}, true},
		pspec{"FutureEnum", shader.KindInt, [4]float32{}, false},
		pspec{"FutureTexture", shader.KindTexture, [4]float32{}, false})
	next.Schema.Params[4].Color = true
	next.Schema.Params[5].Enum = []string{"first", "second"}
	candidate, err := m.prepare(next)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := candidate.Get("Speed"); v[0] != 7 {
		t.Fatal("a source resurrection reset a live shared value")
	}
	if v, _ := candidate.GetInt("FutureInt"); v != math.MaxInt32 {
		t.Fatal("unknown integer lost precision")
	}
	if v, _ := candidate.Get("FutureBool"); v[0] != 1 {
		t.Fatal("unknown bool was lost")
	}
	if v, _ := candidate.Get("FutureColor"); v[0] != mathx.SRGBToLinear(0.5) || v[3] != 0.5 {
		t.Fatalf("color semantics %v", v)
	}
	if v, _ := candidate.GetInt("FutureEnum"); v != 1 {
		t.Fatal("enum name was not resolved")
	}
	if m.Shader() != old {
		t.Fatal("preparation changed the last-good shader")
	}
}

func TestUnknownMismatchAndUnresolvedReferenceRejectPreparation(t *testing.T) {
	old := testShader("old", nil, []uint32{0})
	m, _ := New("material", old, 0, shader.BlendPremultiplied)
	m.RetainUnknown(Value{Name: "Added", Kind: shader.KindVec2, Typed: true, N: 2, Lanes: [4]float32{1, 2}}, Tex{}, true)
	next := testShader("next", nil, []uint32{0}, pspec{"Added", shader.KindFloat, [4]float32{}, false})
	if _, err := m.prepare(next); err == nil {
		t.Fatal("incompatible explicit unknown value was discarded")
	}
	if m.Shader() != old || !m.Explicit("Added") {
		t.Fatal("rejected preparation modified last-good state")
	}

	tex, _ := New("texture", old, 0, shader.BlendPremultiplied)
	ref := ids.AssetID{Lo: 42}
	tex.RetainUnknown(Value{Name: "Map", Kind: shader.KindTexture, Typed: true, Asset: true, N: 0, Reference: ref}, Tex{}, false)
	withTex := testShader("with texture", nil, []uint32{0}, pspec{"Map", shader.KindTexture, [4]float32{}, false})
	if _, err := tex.prepare(withTex); err == nil || !strings.Contains(err.Error(), ref.String()) {
		t.Fatal("unresolved explicit texture silently became white")
	}
	if got, ok := tex.RetainedReference("Map"); !ok || got != ref {
		t.Fatal("reference identity lost")
	}
	staged, err := tex.CloneDefinition()
	if err != nil {
		t.Fatal(err)
	}
	if !staged.ResolveRetainedTexture("Map", Tex{Texture: gpu.TextureHandle{}}) {
		t.Fatal("cannot resolve retained dependency")
	}
	if _, err := staged.prepare(withTex); err != nil {
		t.Fatal(err)
	}
	if _, err := tex.prepare(withTex); err == nil {
		t.Fatal("staging changed the live unresolved definition")
	}
}
