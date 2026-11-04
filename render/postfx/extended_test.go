package postfx_test

import (
	"aego/core/ids"
	"aego/render/material"
	"aego/render/postfx"
	"aego/render/shader"
	"testing"
)

func TestExtendedEffectContracts(t *testing.T) {
	r := postfx.NewRegistry()
	if len(r.Descriptors()) != 7 {
		t.Fatal("seven built-ins expected")
	}
	var p postfx.Plan
	build := func(e postfx.Instance, profile postfx.Profile, want int) {
		t.Helper()
		if err := r.Build(&p, true, []postfx.Instance{e}, profile); err != nil || len(p.Passes) != want {
			t.Fatalf("%s %v %v", e.TypeID, p, err)
		}
	}
	b := newInstance(t, r, postfx.BloomID, "lamp")
	build(b, postfx.Profile{}, 1)
	if len(p.Passes[0].Descriptor.Auxiliary()) != 3 {
		t.Fatal("bloom filters")
	}
	b.Parameters[1].Value.X = 0
	build(b, postfx.Profile{}, 0)
	g := newInstance(t, r, postfx.ColorGradeID, "grade")
	build(g, postfx.Profile{}, 0)
	g.LUT.ID = ids.AssetID{Lo: 17}
	build(g, postfx.Profile{}, 1)
	g.Parameters[2].Value.X = 0
	build(g, postfx.Profile{}, 0)
	a := newInstance(t, r, postfx.ChromaticID, "colour")
	build(a, postfx.Profile{ReducedMotion: true}, 0)
	build(a, postfx.Profile{}, 1)
	build(a, postfx.Profile{Transparent: true}, 0)
	c := newInstance(t, r, postfx.CustomID, "user")
	build(c, postfx.Profile{}, 0)
	c.Enabled = true
	if err := r.Validate([]postfx.Instance{c}); err == nil {
		t.Fatal("missing custom material accepted")
	}
	c.Material.ID = ids.AssetID{Lo: 29}
	build(c, postfx.Profile{}, 1)
}

type testMaterials map[ids.AssetID]*material.Material

func (s testMaterials) Material(id ids.AssetID) (*material.Material, bool) {
	m, ok := s[id]
	return m, ok
}

func TestCustomPhaseComesFromPreparedMaterial(t *testing.T) {
	r := postfx.NewRegistry()
	c := newInstance(t, r, postfx.CustomID, "custom")
	c.Enabled = true
	c.Material.ID = ids.AssetID{Lo: 3}
	sh := &material.Shader{Domain: shader.DomainPostFX, Schema: material.NewSchema(nil, 0, 0), Variants: []material.Variant{{}}}
	m, ok := material.New("custom", sh, 0, shader.BlendOpaque)
	if !ok {
		t.Fatal("material")
	}
	src := testMaterials{c.Material.ID: m}
	grade := newInstance(t, r, postfx.VignetteID, "v")
	var p postfx.Plan
	sh.Features = shader.FeaturePostSceneLinear
	if err := r.BuildWithMaterials(&p, true, []postfx.Instance{c, grade}, postfx.Profile{}, src); err != nil || len(p.Passes) != 2 || p.Passes[0].Phase() != postfx.SceneLinear {
		t.Fatal(err, p)
	}
	if err := r.BuildWithMaterials(&p, true, []postfx.Instance{grade, c}, postfx.Profile{}, src); err == nil || len(p.Passes) != 0 {
		t.Fatal("phase conflict should reject the whole stack")
	}
	for _, feature := range []shader.Features{0, shader.FeaturePostSceneLinear} {
		sh.Features = feature
		if err := r.BuildWithMaterials(&p, true, []postfx.Instance{c}, postfx.Profile{LinearOutput: true}, src); err != nil {
			t.Fatal(err)
		}
		if allowed := p.CheckOutput() == nil; allowed != (feature == shader.FeaturePostSceneLinear) {
			t.Fatal("wrong linear-output policy", p)
		}
	}
}

func TestPlanRebuildReleasesRemovedMaterial(t *testing.T) {
	reg := postfx.NewRegistry()
	grade := newInstance(t, reg, postfx.VignetteID, "grade")
	custom := newInstance(t, reg, postfx.CustomID, "custom")
	custom.Enabled, custom.Material.ID = true, ids.AssetID{Lo: 4}
	sh := &material.Shader{Domain: shader.DomainPostFX, Schema: material.NewSchema(nil, 0, 0), Variants: []material.Variant{{}}}
	m, ok := material.New("removed", sh, 0, shader.BlendOpaque)
	if !ok {
		t.Fatal("material")
	}
	src := testMaterials{custom.Material.ID: m}
	var p postfx.Plan
	build := func(items ...postfx.Instance) error {
		return reg.BuildWithMaterials(&p, true, items, postfx.Profile{}, src)
	}
	if err := build(grade, custom); err != nil || len(p.Passes) != 2 || p.Passes[1].Material != m {
		t.Fatalf("custom material was not active: %v", err)
	}
	if err := build(grade); err != nil || len(p.Passes) != 1 {
		t.Fatal(err, p.Passes)
	}
	if p.Passes[:cap(p.Passes)][1].Material != nil || p.States[:cap(p.States)][1].InstanceID != "" {
		t.Fatal("shortened plan retains deleted material or instance")
	}
	broken := postfx.Instance{TypeID: "unavailable", InstanceID: "invalid", Enabled: true}
	if err := build(custom, broken); err == nil || len(p.Passes) != 0 {
		t.Fatal("invalid stack was accepted")
	}
	if p.Passes[:cap(p.Passes)][0].Material != nil {
		t.Fatal("rejected plan retains material from the discarded pass")
	}
}
