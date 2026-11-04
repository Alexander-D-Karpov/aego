package render2d

import (
	"math"
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine/base"
	"aego/engine/module"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/shader"
	"aego/render/texture"
	"aego/scene"
	"aego/scene/propref"
	"aego/scene/sceneio"
	"aego/schema"
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
	if err = New().Register(module.New(types, nil)); err != nil {
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
	sr := scene.Add[SpriteRenderer](s, h)
	*sr = DefaultSpriteRenderer()
	sr.Material.ID = ids.AssetID{Lo: 1}
	scene.Add[MaterialOverride](s, h)
	s.Flush()
	return h
}

func TestMaterialOverridesIsolatedInheritedExactAndRoundTrip(t *testing.T) {
	s, m, _ := fixtureMaterial(t)
	a, b := fixtureNode(s, "A"), fixtureNode(s, "B")
	m.Set("Flash", [4]float32{.5})
	if err := SetMaterialParameter(s, a, "Flash", schema.Float32(.5)); err != nil {
		t.Fatal(err)
	}
	m.Set("Flash", [4]float32{.75})
	va, _ := GetMaterialParameter(s, a, "Flash")
	vb, _ := GetMaterialParameter(s, b, "Flash")
	if va.F != .5 || vb.F != .75 {
		t.Fatalf("explicit/inherited a=%v b=%v", va.F, vb.F)
	}
	if err := SetMaterialParameter(s, a, "Index", schema.Int32(math.MaxInt32)); err != nil {
		t.Fatal(err)
	}
	o, _ := scene.Get[MaterialOverride](s, a)
	lanes, err := o.Lanes(m)
	if err != nil {
		t.Fatal(err)
	}
	if got := int32(math.Float32bits(lanes[1])); got != math.MaxInt32 {
		t.Fatalf("integer lost precision: %d", got)
	}
	before := len(o.Values)
	if err := SetMaterialParameter(s, a, "Flash", schema.Bool(true)); err == nil {
		t.Fatal("wrong type accepted")
	}
	if err := SetMaterialParameter(s, a, "Uniform", schema.Float32(1)); err == nil {
		t.Fatal("non-instance parameter accepted")
	}
	if len(o.Values) != before {
		t.Fatal("failed mutation changed override state")
	}
	if uniform, err := GetMaterialParameter(s, a, "Uniform"); err != nil || math.Abs(uniform.F-.7) > 1e-5 {
		t.Fatalf("uniform read: %v %v", uniform, err)
	}
	o.Values = append(o.Values, MaterialParameter{Name: "Removed", Kind: shader.KindFloat})
	saved := sceneio.Save(s)
	if !strings.Contains(string(saved), "Removed") {
		t.Fatal("unknown parameter lost")
	}
	play, _, err := sceneio.Load(saved, "play.aescene", s.Types(), s.Resolver(), sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = SetMaterialParameter(play, play.Handle(s.ID(a)), "Flash", schema.Float32(.9)); err != nil {
		t.Fatal(err)
	}
	va, _ = GetMaterialParameter(s, a, "Flash")
	if va.F != .5 {
		t.Fatal("play changed edit state")
	}
	if !ResetMaterialParameter(s, a, "Flash") {
		t.Fatal("reset failed")
	}
	va, _ = GetMaterialParameter(s, a, "Flash")
	if va.F != .75 {
		t.Fatal("reset did not inherit material")
	}
}

func TestMaterialProprefBindingWritesOnlyNodeAndRejectsStaleGeneration(t *testing.T) {
	s, m, _ := fixtureMaterial(t)
	h := fixtureNode(s, "A")
	resolver := propref.New(s, nil)
	ref, err := resolver.Resolve(h, "MaterialOverride.Flash")
	if err != nil {
		t.Fatal(err)
	}
	if !resolver.Write(&ref, h, [4]float32{.8}) {
		t.Fatal("write")
	}
	value, ok := resolver.Read(&ref, h)
	if !ok || value[0] != .8 {
		t.Fatalf("read %v %v", value, ok)
	}
	if shared, _ := m.Get("Flash"); math.Abs(float64(shared[0]-.2)) > 1e-6 {
		t.Fatal("binding mutated shared material")
	}
	other, err := resolver.Resolve(h, "MaterialOverride.Offset.x")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Overlaps(&other) || ref.Hash() == other.Hash() {
		t.Fatal("distinct dynamic properties collide")
	}
	if got := testing.AllocsPerRun(100, func() { resolver.Write(&ref, h, [4]float32{.5}); resolver.Read(&ref, h) }); got != 0 {
		t.Fatalf("hot propref allocated %v", got)
	}
	o, _ := scene.Get[MaterialOverride](s, h)
	if got := testing.AllocsPerRun(100, func() { o.Lanes(m) }); got != 0 {
		t.Fatalf("lane preparation allocated %v", got)
	}
	old := m.Shader()
	next := *old
	next.Schema = material.NewSchema(append([]shader.Param(nil), old.Schema.Params...), 16, 2)
	dev := null.New()
	cache := material.NewCache(dev, false, func(shader.Domain, int, shader.Blend) gpu.PipelineDesc { return gpu.PipelineDesc{} })
	defer dev.Close()
	defer cache.Close()
	cache.Track(m)
	cache.QueueReload(old, &next, []*material.Material{m}, nil)
	cache.Begin()
	cache.End()
	if m.Generation() < 2 {
		t.Fatal("generation did not advance")
	}
	if resolver.Write(&ref, h, [4]float32{1}) {
		t.Fatal("stale binding accepted")
	}
	fresh, err := resolver.Resolve(h, "MaterialOverride.Flash")
	if err != nil {
		t.Fatal(err)
	}
	if !resolver.Write(&fresh, h, [4]float32{1}) {
		t.Fatal("fresh binding rejected")
	}
}

func TestMaterialAliasPreservesExplicitNodeValue(t *testing.T) {
	s, m, _ := fixtureMaterial(t)
	h := fixtureNode(s, "A")
	o, _ := scene.Get[MaterialOverride](s, h)
	m.Shader().Schema = material.NewSchema([]shader.Param{{Name: "Renamed", Former: []string{"Old"}, Kind: shader.KindFloat, Instance: true, Lane: 0}}, 0, 1)
	o.Values = []MaterialParameter{{Name: "Old", Kind: shader.KindFloat, Value: mathx.Vec4{X: .6}}}
	v, err := o.Effective(m, "Renamed")
	if err != nil || math.Abs(v.F-.6) > 1e-6 {
		t.Fatalf("alias lost %v %v", v, err)
	}
	if !o.ExplicitFor(m, "Renamed") || !o.ResetFor(m, "Renamed") {
		t.Fatal("alias explicit/reset")
	}
}

func TestMaterialReloadWorldCheckRejectsDomainAndExplicitType(t *testing.T) {
	s, m, _ := fixtureMaterial(t)
	h := fixtureNode(s, "active")
	if err := SetMaterialParameter(s, h, "Flash", schema.Float32(.4)); err != nil {
		t.Fatal(err)
	}
	next := *m.Shader()
	next.Schema = material.NewSchema([]shader.Param{{Name: "Flash", Kind: shader.KindInt, Instance: true}}, 0, 1)
	match := func(current *material.Material) bool { return current == m }
	if err := checkMaterialSwap(s, &next, match); err == nil {
		t.Fatal("incompatible explicit type accepted")
	}
	next.Schema = material.NewSchema(nil, 0, 0)
	if err := checkMaterialSwap(s, &next, match); err != nil {
		t.Fatalf("removed field must be preserved unknown: %v", err)
	}
	ResetMaterialParameter(s, h, "Flash")
	next.Domain = shader.DomainMesh2D
	if err := checkMaterialSwap(s, &next, match); err == nil {
		t.Fatal("domain change accepted on node without explicit values")
	}
	if err := checkMaterialSwap(s, &next, func(*material.Material) bool { return false }); err != nil {
		t.Fatal("unrelated material vetoed isolated reload")
	}
}

func TestSpriteWithoutMaterialKeepsLegacySourceShape(t *testing.T) {
	s, _, _ := fixtureMaterial(t)
	h := s.Create(s.Root(), "legacy")
	*scene.Add[SpriteRenderer](s, h) = DefaultSpriteRenderer()
	s.Flush()
	saved := sceneio.Save(s)
	if strings.Contains(string(saved), "material=") {
		t.Fatal("legacy sprite gained a zero material field")
	}
	if err := AssignMaterial(s, h, ids.AssetID{Lo: 42}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sceneio.Save(s)), `material=asset("0000000000000000000000000000002a")`) {
		t.Fatal("nonzero material reference was omitted")
	}
}

func TestMaterialPropertyFirstResolutionIsAllocationFree(t *testing.T) {
	s, _, _ := fixtureMaterial(t)
	h := fixtureNode(s, "runner")
	resolver := propref.New(s, nil)
	var binding propref.Ref
	var err error
	if n := testing.AllocsPerRun(100, func() { binding, err = resolver.Resolve(h, "MaterialOverride.Flash") }); err != nil || n != 0 {
		t.Fatalf("initial resolution allocates %v objects: %v", n, err)
	}
	if _, ok := resolver.Read(&binding, h); !ok {
		t.Fatal("resolved property was unreadable")
	}
}
