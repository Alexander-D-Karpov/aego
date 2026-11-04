package materials_test

import (
	"aego/anim"
	"aego/asset"
	"aego/core/ids"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/gpu"
	"aego/gpu/null"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/modules/tween"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/shader"
	"aego/render/texture"
	"aego/scene"
	"aego/scene/sceneio"
	"math"
	"testing"
)

type materialResolver struct {
	mat  *material.Material
	clip []byte
}

func (*materialResolver) Decoded(_ ids.AssetID, _ string, build func() (any, error)) (any, error) {
	return build()
}
func (r *materialResolver) Anim(ids.AssetID) ([]byte, bool) { return r.clip, len(r.clip) > 0 }

func (r *materialResolver) Material(id ids.AssetID) (*material.Material, bool) {
	return r.mat, r.mat != nil
}
func (*materialResolver) Texture(ids.AssetID) (texture.Region, bool) {
	return texture.Region{W: 16, H: 16}, true
}
func (*materialResolver) Mesh(ids.AssetID) (*mesh.Mesh, bool)   { return nil, false }
func (*materialResolver) Scene(ids.AssetID) ([]byte, bool)      { return nil, false }
func (*materialResolver) Prefab(ids.AssetID) ([]byte, bool)     { return nil, false }
func (*materialResolver) Generation() uint32                    { return 1 }
func (*materialResolver) Subscribe(func(asset.Reloaded)) func() { return func() {} }

func fixtureMaterial(t *testing.T) (*scene.Scene, *material.Material, *materialResolver) {
	t.Helper()
	params := []shader.Param{
		{Name: "Flash", Kind: shader.KindFloat, Instance: true, Default: [4]float32{.2}, Lane: 0, Comp: 0},
		{Name: "Index", Kind: shader.KindInt, Instance: true, IntDefault: 3, Lane: 0, Comp: 1},
		{Name: "Enabled", Kind: shader.KindBool, Instance: true, Lane: 0, Comp: 2},
		{Name: "Offset", Kind: shader.KindVec2, Instance: true, Lane: 1, Comp: 0},
		{Name: "Uniform", Kind: shader.KindFloat, Default: [4]float32{.7}, Offset: 0},
	}
	sh := &material.Shader{Name: "fixture", Schema: material.NewSchema(params, 16, 2), Variants: []material.Variant{{Sources: [4]string{"void main(){}", "void main(){}", "void main(){}", "void main(){}"}}}}
	mat, ok := material.New("fixture", sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("material")
	}
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if err = render2d.New().Register(module.New(types, nil)); err != nil {
		t.Fatal(err)
	}
	if err = animation.New().Register(module.New(types, nil)); err != nil {
		t.Fatal(err)
	}
	if err = types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	res := &materialResolver{mat: mat}
	return scene.New(types, res, scene.Options{}), mat, res
}
func fixtureNode(s *scene.Scene, name string) scene.NodeHandle {
	h := s.Create(s.Root(), name)
	sr := scene.Add[render2d.SpriteRenderer](s, h)
	*sr = render2d.DefaultSpriteRenderer()
	sr.Material.ID = ids.AssetID{Lo: 1}
	scene.Add[render2d.MaterialOverride](s, h)
	s.Flush()
	return h
}

func commitFixtureShader(t *testing.T, m *material.Material) {
	t.Helper()
	old := m.Shader()
	next := *old
	next.Schema = material.NewSchema(append([]shader.Param(nil), old.Schema.Params...), old.Schema.Size, old.Schema.Lanes)
	dev := null.New()
	defer dev.Close()
	cache := material.NewCache(dev, false, func(shader.Domain, int, shader.Blend) gpu.PipelineDesc { return gpu.PipelineDesc{} })
	defer cache.Close()
	cache.Track(m)
	cache.QueueReload(old, &next, []*material.Material{m}, nil)
	cache.Begin()
	cache.End()
	if m.Shader() != &next {
		t.Fatal("fixture reload was not committed")
	}
}

func TestMaterialTweenSurvivesSchemaGenerationAndKeepsEditScene(t *testing.T) {
	edit, m, _ := fixtureMaterial(t)
	a := fixtureNode(edit, "animated")
	b := fixtureNode(edit, "shared")
	saved := sceneio.Save(edit)
	play, _, err := sceneio.Load(saved, "play.aescene", edit.Types(), edit.Resolver(), sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	pa := play.Handle(edit.ID(a))
	svc := tween.NewService(play, engine.NewWorld("play"))
	defer svc.Close()
	h, err := svc.ToFloat(pa, "MaterialOverride.Flash", 1, 1, tween.Options{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Step(.25, false)
	v, err := render2d.GetMaterialParameter(play, pa, "Flash")
	if err != nil || math.Abs(v.F-.4) > 1e-5 {
		t.Fatalf("tween first sample %v %v", v, err)
	}
	commitFixtureShader(t, m)
	svc.Step(.25, false)
	v, err = render2d.GetMaterialParameter(play, pa, "Flash")
	if err != nil || math.Abs(v.F-.6) > 1e-5 || !svc.Running(h) {
		t.Fatalf("tween did not rebind after reload: %v %v", v, err)
	}
	svc.Step(.5, false)
	v, _ = render2d.GetMaterialParameter(play, pa, "Flash")
	if v.F != 1 || !svc.Done(h) {
		t.Fatalf("tween did not finish: %v", v)
	}
	for _, n := range []scene.NodeHandle{a, b} {
		v, _ = render2d.GetMaterialParameter(edit, n, "Flash")
		if math.Abs(v.F-.2) > 1e-5 {
			t.Fatal("play changed edit or shared material")
		}
	}
	if string(sceneio.Save(edit)) != string(saved) {
		t.Fatal("play tween dirtied serialized edit scene")
	}
}

func TestMaterialAnimationSurvivesReloadAndKeepsInt32Exact(t *testing.T) {
	s, m, res := fixtureMaterial(t)
	h := fixtureNode(s, "animated")
	other := fixtureNode(s, "shared")
	source := `anim version=1 length=1 {
  track "" prop="MaterialOverride.Flash" kind=float32 {
    key t=0 v=0 interp=linear {}
    key t=1 v=1 {}
  }
  track "" prop="MaterialOverride.Index" kind=int32 {
    key t=0 v=2147483647 {}
    key t=1 v=2147483647 {}
  }
  track "" prop="MaterialOverride.Enabled" kind=bool {
    key t=0 v=true {}
    key t=1 v=true {}
  }
}`
	doc, err := aetext.Parse([]byte(source), "material.aeanim")
	if err != nil {
		t.Fatal(err)
	}
	clip, err := anim.Decode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if problems := clip.Check(nil); len(problems) > 0 {
		t.Fatal(problems)
	}
	res.clip = anim.Marshal(clip)
	p := animation.DefaultPlayer()
	p.Clip = animation.ClipRef{ID: ids.AssetID{Lo: 2}}
	*scene.Add[animation.AnimationPlayer](s, h) = p
	s.Flush()
	s.UpdateTransforms()
	svc := animation.NewService(s, engine.NewWorld("animation"), animation.New())
	defer svc.Close()
	svc.Fixed(.25)
	v, err := render2d.GetMaterialParameter(s, h, "Flash")
	if err != nil || math.Abs(v.F-.25) > 1e-5 {
		t.Fatalf("first animation sample %v %v", v, err)
	}
	iv, _ := render2d.GetMaterialParameter(s, h, "Index")
	if iv.I != math.MaxInt32 {
		t.Fatalf("integer animation lost precision: %d", iv.I)
	}
	bv, _ := render2d.GetMaterialParameter(s, h, "Enabled")
	if bv.I != 1 {
		t.Fatal("bool animation did not write")
	}
	commitFixtureShader(t, m)
	svc.Fixed(.25)
	v, err = render2d.GetMaterialParameter(s, h, "Flash")
	if err != nil || math.Abs(v.F-.5) > 1e-5 {
		t.Fatalf("animation did not rebind after reload: %v %v", v, err)
	}
	v, _ = render2d.GetMaterialParameter(s, other, "Flash")
	if math.Abs(v.F-.2) > 1e-5 {
		t.Fatal("animation mutated shared material")
	}
	iv, _ = render2d.GetMaterialParameter(s, h, "Index")
	if iv.I != math.MaxInt32 {
		t.Fatalf("integer after reload: %d", iv.I)
	}
}
