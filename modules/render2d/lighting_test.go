package render2d

import (
	"math"
	"strconv"
	"testing"

	"aego/asset"
	"aego/asset/silhouette"
	"aego/contracts/shapes2d"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/gpu"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/shader"
	"aego/render/texture"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/mutate"
	"aego/scene/propref"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/bytecode"
)

type lightingResolver struct {
	materialResolver
	regions       map[ids.AssetID]texture.Region
	pairs         map[ids.AssetID]ids.AssetID
	emissionPairs map[ids.AssetID]ids.AssetID
	shadowFrames  map[ids.AssetID]silhouette.Shape
}

func (r *lightingResolver) Texture(id ids.AssetID) (texture.Region, bool) {
	x, ok := r.regions[id]
	return x, ok
}
func (r *lightingResolver) NormalFor(id ids.AssetID) (ids.AssetID, texture.Region, bool) {
	n := r.pairs[id]
	x, ok := r.regions[n]
	return n, x, ok
}
func (r *lightingResolver) EmissionFor(id ids.AssetID) (ids.AssetID, texture.Emission, bool) {
	e := r.emissionPairs[id]
	x, ok := r.regions[e]
	return e, texture.Emission{Region: x, Mode: texture.EmissionRGB}, ok
}
func (*lightingResolver) Mesh(ids.AssetID) (*mesh.Mesh, bool) { return nil, false }
func (r *lightingResolver) SpriteShadow(id ids.AssetID) (silhouette.Shape, bool) {
	s, ok := r.shadowFrames[id]
	return s, ok
}

type lightingFixture struct {
	s   *scene.Scene
	w   *engine.World
	r   *frame.Runner
	m   Module
	p   *problems.Service
	res *lightingResolver
}

func lightingTestWorld(t *testing.T) lightingFixture {
	t.Helper()
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if _, err = module.Load(types, nil, m); err != nil {
		t.Fatal(err)
	}
	if err = types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	res := &lightingResolver{regions: map[ids.AssetID]texture.Region{ids.AssetID{Lo: 1}: texture.Whole(gpu.TextureHandle{Index: 1, Gen: 1}, 16, 16)}, pairs: map[ids.AssetID]ids.AssetID{}}
	s, w, r, p := scene.New(types, res, scene.Options{}), engine.NewWorld("lighting-test"), frame.New(), problems.New()
	r.Bind(s, nil, res)
	w.SetView(frame.ServiceID, r)
	m.Init(&module.InitContext{Problems: p})
	ctx := &module.AttachContext{World: w, Scene: s, Editor: true}
	if err = m.AttachWorld(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.DetachWorld(ctx) })
	return lightingFixture{s, w, r, m, p, res}
}
func (f lightingFixture) camera(name string, mask uint64) scene.NodeHandle {
	h := f.s.Create(f.s.Root(), name)
	c := scene.Add[Camera2D](f.s, h)
	*c = DefaultCamera2D()
	c.Zoom = 16
	c.Mask = int64(mask)
	return h
}
func (f lightingFixture) light(name string, kind lighting.Type) scene.NodeHandle {
	h := f.s.Create(f.s.Root(), name)
	c := scene.Add[Light2D](f.s, h)
	*c = DefaultLight2D()
	c.Type = kind
	return h
}
func (f lightingFixture) sprite(name string, layers uint64) scene.NodeHandle {
	h := f.s.Create(f.s.Root(), name)
	c := scene.Add[SpriteRenderer](f.s, h)
	*c = DefaultSpriteRenderer()
	c.Sprite.ID = ids.AssetID{Lo: 1}
	f.s.SetLayers(h, layers)
	return h
}
func (f lightingFixture) tick() {
	f.r.Prepare(640, 360, 1)
	ctx := &engine.Context{World: f.w}
	cameras{}.Run(ctx)
	extract{}.Run(ctx)
}

func TestLightingRegistrationDefaultsScriptAndGenericMutations(t *testing.T) {
	f := lightingTestWorld(t)
	for _, name := range []string{"engine.Light2D", "engine.LightOccluder2D", "engine.LightingEnvironment2D"} {
		td, ok := f.s.Registry().ByName(name)
		if !ok || td.Owner != ID || td.Default == nil || td.Validate == nil || td.Doc == "" {
			t.Fatalf("incomplete registration %s: %+v", name, td)
		}
	}
	td, _ := f.s.Registry().ByName("engine.Light2D")
	for _, name := range []string{"layers", "shadowCasterMask"} {
		fd, ok := td.Field(name)
		if !ok || fd.Kind != schema.KindInt64 {
			t.Fatal("lighting masks must use exact signed 64-bit schema storage")
		}
	}
	l := DefaultLight2D()
	if l.Height <= 0 || l.Elevation <= 0 || l.Layers != -1 || l.ShadowCasterMask != -1 {
		t.Fatal("incompatible lighting defaults")
	}
	o := DefaultLightOccluder2D()
	if err := shapes2d.ValidateContour(o.Polygon, o.Closed); err != nil {
		t.Fatal(err)
	}
	e := DefaultLightingEnvironment2D()
	if e.AmbientIntensity != 1 || e.AmbientColor != (mathx.Color{R: 1, G: 1, B: 1, A: 1}) || e.Exposure != 1 {
		t.Fatal("old scene default changed")
	}
	h := f.s.Create(f.s.Root(), "generic")
	f.s.Flush()
	doc := mutate.Wrap(f.s, "lighting.aescene", false, mutate.Env{Types: f.s.Types(), Resolver: f.res})
	ref := "#" + f.s.ID(h).String()
	if _, err := doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.AddComp(ref, "engine.Light2D"), mutate.Set(ref, "Light2D.type", "spot"), mutate.Set(ref, "Light2D.intensity", "2.5")}}); err != nil {
		t.Fatal(err)
	}
	got, _ := scene.Get[Light2D](f.s, h)
	if got.Type != lighting.Spot || got.Intensity != 2.5 || got.Height != 1 {
		t.Fatalf("generic mutation missed typed defaults: %+v", got)
	}
	if _, err := doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.Set(ref, "Light2D.intensity", "9"), mutate.Set(ref, "Light2D.range", "\"bad\"")}}); err == nil {
		t.Fatal("invalid atomic mutation succeeded")
	}
	got, _ = scene.Get[Light2D](f.s, h)
	if got.Intensity != 2.5 {
		t.Fatal("failed atomic batch changed lighting state")
	}
	bindings := bind.New()
	installLightingScript(bindings.Module("aego/lighting"))
	if err := bindings.Freeze(); err != nil {
		t.Fatal(err)
	}
	for _, imp := range []bytecode.HostImport{
		{Module: "aego/lighting", Name: "SetEnabled", Sig: abi.Sig(abi.T(abi.Void), abi.T(abi.Bool))},
		{Module: "aego/lighting", Name: "Enabled", Sig: abi.Sig(abi.T(abi.Bool))},
		{Module: "aego/lighting", Name: "SetDebug", Sig: abi.Sig(abi.T(abi.Void), abi.T(abi.String))},
		{Module: "aego/lighting", Name: "Debug", Sig: abi.Sig(abi.T(abi.String))},
	} {
		if _, ok := bindings.Resolve(imp); !ok {
			t.Fatalf("missing typed Script binding %s", imp.Name)
		}
	}
}

func TestLightingMask64MutationSerializationCameraExtraction(t *testing.T) {
	for _, bit := range []uint{0, 31, 32, 63} {
		t.Run(strconv.Itoa(int(bit)), func(t *testing.T) {
			f := lightingTestWorld(t)
			mask := uint64(1) << bit
			cam := f.camera("cam", mask)
			spriteNode := f.sprite("receiver", mask)
			lightNode := f.light("light", lighting.Point)
			f.s.Flush()
			doc := mutate.Wrap(f.s, "lighting.aescene", false, mutate.Env{Types: f.s.Types(), Resolver: f.res})
			ref := "#" + f.s.ID(lightNode).String()
			text := strconv.FormatInt(int64(mask), 10)
			if _, err := doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.Set(ref, "Light2D.layers", text), mutate.Set(ref, "Light2D.shadowCasterMask", text)}}); err != nil {
				t.Fatal(err)
			}
			f.tick()
			if len(f.r.Views()) != 1 || uint64(f.r.Views()[0].Layers) != mask || len(f.r.Bucket(0).Sprites) != 1 || f.r.Bucket(0).Sprites[0].VisibilityLayers != mask || len(f.r.Bucket(0).Lighting.Lights) != 1 || f.r.Bucket(0).Lighting.Lights[0].ShadowCasterMask != mask {
				t.Fatalf("mask bit %d truncated on camera/extraction", bit)
			}
			loaded, _, err := sceneio.Load(sceneio.Save(f.s), "roundtrip.aescene", f.s.Types(), f.res, sceneio.LoadOptions{KeepIDs: true})
			if err != nil {
				t.Fatal(err)
			}
			lc, _ := scene.Get[Light2D](loaded, loaded.Handle(f.s.ID(lightNode)))
			cc, _ := scene.Get[Camera2D](loaded, loaded.Handle(f.s.ID(cam)))
			if uint64(lc.Layers) != mask || uint64(lc.ShadowCasterMask) != mask || uint64(cc.Mask) != mask || loaded.Layers(loaded.Handle(f.s.ID(spriteNode))) != mask {
				t.Fatalf("mask bit %d lost in scene/schema round-trip", bit)
			}
			l, _ := scene.Get[Light2D](f.s, lightNode)
			l.Layers = 0
			f.tick()
			if len(f.r.Bucket(0).Lighting.Lights) != 0 {
				t.Fatal("explicit zero light mask became all layers")
			}
		})
	}
}

func TestLightingManualPolygonNoPhysicsPresentationAndSeparateRevisions(t *testing.T) {
	f := lightingTestWorld(t)
	f.camera("cam", ^uint64(0))
	parent := f.s.Create(f.s.Root(), "moving-parent")
	h := f.s.Create(parent, "occluder")
	*scene.Add[LightOccluder2D](f.s, h) = DefaultLightOccluder2D()
	l := f.light("light", lighting.Point)
	f.s.Reparent(l, parent, -1)
	f.s.SetLocal(h, scene.Transform2D{Rotation: .6, Scale: mathx.Vec2{X: 1, Y: 1}})
	f.s.Flush()
	pose := scene.Transform2D{Position: mathx.Vec2{X: 30, Y: 4}, Rotation: .4, Scale: mathx.Vec2{X: -2, Y: 3}}
	f.s.SetPresentationLocal(parent, pose)
	f.tick()
	if _, ok := f.w.ServiceAny(shapes2d.ServiceID); ok {
		t.Fatal("polygon test accidentally needs a shape provider")
	}
	in := &f.r.Bucket(0).Lighting
	if len(in.Occluders) != 1 || len(in.Points) != 4 {
		t.Fatal("offscreen manual occluder was omitted")
	}
	before := in.Occluders[0]
	p0 := in.Points[0]
	want := f.s.RenderWorld(h).Apply(mathx.Vec2{X: -.5, Y: -.5})
	if p0 != want {
		t.Fatalf("occluder differs from sprite presentation: %v != %v", p0, want)
	}
	f.tick()
	after := f.r.Bucket(0).Lighting.Occluders[0]
	if before.GeometryRevision != after.GeometryRevision || before.TransformRevision != after.TransformRevision {
		t.Fatal("static geometry invalidates every frame")
	}
	pose.Position.X = 31
	f.s.SetPresentationLocal(parent, pose)
	f.tick()
	moved := f.r.Bucket(0).Lighting.Occluders[0]
	if moved.GeometryRevision != before.GeometryRevision || moved.TransformRevision == before.TransformRevision || f.r.Bucket(0).Lighting.Points[0].X != p0.X+1 {
		t.Fatal("presentation move changed geometry or used fixed pose")
	}
	oc, _ := scene.Get[LightOccluder2D](f.s, h)
	oc.Polygon[0].X = -.8
	f.tick()
	changed := f.r.Bucket(0).Lighting.Occluders[0]
	if changed.GeometryRevision == moved.GeometryRevision || changed.TransformRevision != moved.TransformRevision {
		t.Fatal("shape edit did not keep independent revisions")
	}
	f.s.SetEnabled(parent, false)
	f.tick()
	if len(f.r.Bucket(0).Lighting.Occluders) != 0 {
		t.Fatal("disabled ancestor still casts")
	}
	ls, _ := engine.ServiceOf[*lightingState](f.w, lightingServiceID)
	if len(ls.cache) != 0 {
		t.Fatal("disabled/deleted contours retained historical cache")
	}
	f.s.SetEnabled(parent, true)
	f.tick()
	f.s.Destroy(h)
	f.tick()
	if len(ls.cache) != 0 {
		t.Fatal("removed occluder kept cached geometry")
	}
}

type fixtureShapeProvider struct {
	owner    ids.NodeID
	revision uint64
	points   []mathx.Vec2
}

func (p *fixtureShapeProvider) Append(node ids.NodeID, key string, out *shapes2d.Buffer) error {
	if node != p.owner {
		return shapes2d.ErrMissingNode
	}
	if key != "" && key != "shape.stable" {
		return shapes2d.ErrMissingShape
	}
	return out.Append(shapes2d.Contour{ID: shapes2d.ID{Node: p.owner, Part: 73}, Owner: p.owner, Key: "shape.stable", Revision: p.revision, Closed: true}, p.points)
}

func TestLightingShapeProviderOwnerAppliedOnceAndMissingProviderDiagnostic(t *testing.T) {
	f := lightingTestWorld(t)
	f.camera("cam", ^uint64(0))
	owner := f.s.Create(f.s.Root(), "shape-owner")
	f.s.SetPosition(owner, mathx.Vec2{X: 4, Y: 2})
	node := f.s.Create(f.s.Root(), "occluder-reference")
	f.s.SetPosition(node, mathx.Vec2{X: 100, Y: 100})
	c := scene.Add[LightOccluder2D](f.s, node)
	*c = DefaultLightOccluder2D()
	c.Source = OccluderShapeProvider
	c.ShapeNode = scene.RefTo(f.s, owner)
	c.Shape = "shape.stable"
	f.tick()
	ds, rev := LightingDiagnostics(f.w)
	if len(ds) != 1 || ds[0].Code != "LGT005" || len(f.r.Bucket(0).Lighting.Occluders) != 0 {
		t.Fatal("missing provider silently accepted")
	}
	f.tick()
	_, again := LightingDiagnostics(f.w)
	if again != rev {
		t.Fatal("unchanged missing-provider warning repeats every frame")
	}
	p := &fixtureShapeProvider{owner: f.s.ID(owner), revision: 1, points: []mathx.Vec2{{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}}}
	f.w.SetView(shapes2d.ServiceID, p)
	f.tick()
	in := &f.r.Bucket(0).Lighting
	if len(in.Occluders) != 1 || in.Occluders[0].Part != 73 || in.Points[0] != (mathx.Vec2{X: 3, Y: 1}) {
		t.Fatalf("provider owner transformed twice or used receiver transform: %+v %v", in.Occluders, in.Points)
	}
	before := in.Occluders[0]
	p.points[0].X = -2
	p.revision++
	f.tick()
	after := f.r.Bucket(0).Lighting.Occluders[0]
	if after.GeometryRevision == before.GeometryRevision || after.TransformRevision != before.TransformRevision {
		t.Fatal("provider revision did not update only geometry")
	}
	f.s.Destroy(owner)
	f.tick()
	if len(f.r.Bucket(0).Lighting.Occluders) != 0 {
		t.Fatal("missing owner left a dangling shadow")
	}
}

func TestLightingCameraEnvironmentIsolationDuplicateAndViewToggle(t *testing.T) {
	f := lightingTestWorld(t)
	ca := f.camera("A", 1)
	cb := f.camera("B", 2)
	f.light("shared", lighting.Directional)
	for i, h := range []scene.NodeHandle{ca, cb} {
		n := f.s.Create(f.s.Root(), "environment")
		e := scene.Add[LightingEnvironment2D](f.s, n)
		*e = DefaultLightingEnvironment2D()
		e.Camera = scene.RefTo(f.s, h)
		e.AmbientIntensity = float32(i) + .1
	}
	f.tick()
	a, b := &f.r.Bucket(0).Lighting, &f.r.Bucket(1).Lighting
	if a.Environment.AmbientIntensity != .1 || b.Environment.AmbientIntensity != 1.1 || len(a.Lights) != 1 || len(b.Lights) != 1 {
		t.Fatal("cameras share environment state or lost lights")
	}
	a.Lights[0].Intensity = 99
	if b.Lights[0].Intensity == 99 {
		t.Fatal("camera inputs alias mutable slices")
	}
	before := string(sceneio.Save(f.s))
	f.r.SetLightingView(false, lighting.DebugNormals)
	f.tick()
	if !f.r.Bucket(0).Lighting.Disabled || f.r.Bucket(0).Lighting.Debug != lighting.DebugNormals || string(sceneio.Save(f.s)) != before {
		t.Fatal("view toggle changed scene or failed to reach input")
	}
	dup := f.s.Create(f.s.Root(), "duplicate")
	*scene.Add[LightingEnvironment2D](f.s, dup) = DefaultLightingEnvironment2D()
	f.tick()
	if f.r.Bucket(0).Lighting.HasEnvironment || f.r.Bucket(1).Lighting.HasEnvironment {
		t.Fatal("duplicate environment selected arbitrarily")
	}
	found := false
	for _, d := range module.ValidateScene(f.s, nil) {
		if d.Code == "LGT003" && d.BlocksPlay {
			found = true
		}
	}
	if !found {
		t.Fatal("duplicate environments not exposed to validation")
	}
}

func TestLightingValidatesEnergyGeometryAndKeepsUnresolvedReferences(t *testing.T) {
	f := lightingTestWorld(t)
	f.camera("cam", ^uint64(0))
	h := f.light("bad", lighting.Spot)
	f.s.Flush()
	l, _ := scene.Get[Light2D](f.s, h)
	l.Intensity = float32(math.NaN())
	l.Cookie.ID = ids.AssetID{Lo: 987}
	f.tick()
	if len(f.r.Bucket(0).Lighting.Lights) != 0 {
		t.Fatal("NaN light entered the renderer")
	}
	l.Intensity = 1
	l.InnerAngle = 2
	l.OuterAngle = 1
	if len(validateLight2D(module.ValidateContext{Scene: f.s, Node: h}, l)) == 0 {
		t.Fatal("invalid cone accepted")
	}
	l.Type = lighting.Directional
	l.Shadows = true
	if len(validateLight2D(module.ValidateContext{Scene: f.s, Node: h}, l)) == 0 {
		t.Fatal("unsupported directional shadows accepted")
	}
	l.Type = lighting.Point
	l.Shadows = false
	f.tick()
	if l.Cookie.ID != (ids.AssetID{Lo: 987}) {
		t.Fatal("missing cookie ID was erased")
	}
	for _, d := range f.r.Bucket(0).Lighting.Lights {
		if d.Cookie.Texture != (gpu.TextureHandle{}) {
			t.Fatal("missing cookie created fictitious GPU texture")
		}
	}
	n := f.s.Create(f.s.Root(), "bad-contour")
	oc := scene.Add[LightOccluder2D](f.s, n)
	*oc = DefaultLightOccluder2D()
	oc.Polygon = []mathx.Vec2{{}, {X: 1, Y: 1}, {Y: 1}, {X: 1}}
	f.tick()
	if len(f.r.Bucket(0).Lighting.Occluders) != 0 {
		t.Fatal("self-intersection entered lighting")
	}
}

func TestLightingNormalPriorityFramesAndFlatMissingFallback(t *testing.T) {
	f := lightingTestWorld(t)
	f.camera("cam", ^uint64(0))
	h := f.sprite("lit", 1)
	params := []shader.Param{{Name: "NormalMap", Kind: shader.KindTexture, Normal: true, Slot: shader.TexNormal}}
	sh := &material.Shader{Name: "lit", Domain: shader.DomainSprite, Features: shader.FeatureLighting | shader.FeatureAffine, Schema: material.NewSchema(params, 0, 0), Variants: []material.Variant{{}}}
	m, ok := material.New("lit", sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("fixture material")
	}
	f.res.mat = m
	f.res.regions[ids.AssetID{Lo: 2}] = texture.Whole(gpu.TextureHandle{Index: 2, Gen: 1}, 32, 16)
	f.res.regions[ids.AssetID{Lo: 3}] = texture.Whole(gpu.TextureHandle{Index: 3, Gen: 1}, 16, 16)
	f.res.pairs[ids.AssetID{Lo: 1}] = ids.AssetID{Lo: 2}
	f.s.Flush()
	sr, _ := scene.Get[SpriteRenderer](f.s, h)
	sr.Material.ID = ids.AssetID{Lo: 9}
	sr.FlipX = true
	f.tick()
	d := f.r.Bucket(0).Sprites[0]
	if !d.Lighting || !d.HasLinear || d.Normal != f.res.regions[ids.AssetID{Lo: 2}] {
		t.Fatal("paired normal or full normal basis missing")
	}
	if !m.SetTexture("NormalMap", material.Tex{Texture: gpu.TextureHandle{Index: 3, Gen: 1}, Region: f.res.regions[ids.AssetID{Lo: 3}], NormalMap: true}) {
		t.Fatal("set explicit normal")
	}
	f.tick()
	if f.r.Bucket(0).Sprites[0].Normal.Texture.Index != 3 {
		t.Fatal("material normal failed to override frame pair")
	}
	m.Reset("NormalMap")
	sr.Sprite.ID = ids.AssetID{Lo: 3}
	f.res.pairs[sr.Sprite.ID] = ids.AssetID{Lo: 404}
	f.tick()
	if !f.r.Bucket(0).Sprites[0].Normal.IsZero() || f.res.pairs[sr.Sprite.ID] != (ids.AssetID{Lo: 404}) {
		t.Fatal("missing animated normal should be flat with association retained")
	}
	ds, _ := LightingDiagnostics(f.w)
	if len(ds) != 1 || ds[0].Code != "LGT006" {
		t.Fatal("missing frame diagnostic absent")
	}
}

func TestLightingEmissionFollowsFrameWithoutChangingSharedMaterial(t *testing.T) {
	f := lightingTestWorld(t)
	f.camera("cam", ^uint64(0))
	h := f.sprite("lit", 1)
	params := []shader.Param{{Name: "EmissionMap", Kind: shader.KindTexture, Emission: true, Slot: shader.TexEmission}}
	sh := &material.Shader{Name: "lit", Domain: shader.DomainSprite, Features: shader.FeatureLighting | shader.FeatureAffine | shader.FeatureSurfaceChannels, Schema: material.NewSchema(params, 0, 0), Variants: []material.Variant{{}}}
	m, ok := material.New("lit", sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("fixture material")
	}
	f.res.mat = m
	f.res.regions[ids.AssetID{Lo: 2}] = texture.Whole(gpu.TextureHandle{Index: 2, Gen: 1}, 16, 16)
	f.res.regions[ids.AssetID{Lo: 3}] = texture.Whole(gpu.TextureHandle{Index: 3, Gen: 1}, 16, 16)
	f.res.emissionPairs = map[ids.AssetID]ids.AssetID{{Lo: 1}: {Lo: 2}, {Lo: 3}: {Lo: 1}}
	f.s.Flush()
	sr, _ := scene.Get[SpriteRenderer](f.s, h)
	sr.Material.ID = ids.AssetID{Lo: 9}
	f.tick()
	if d := f.r.Bucket(0).Sprites[0]; d.Emission.Region.Texture.Index != 2 || d.Emission.Mode != texture.EmissionRGB {
		t.Fatal("frame emission missing")
	}
	sr.Sprite.ID = ids.AssetID{Lo: 3}
	f.tick()
	if f.r.Bucket(0).Sprites[0].Emission.Region.Texture.Index != 1 || m.Explicit("EmissionMap") {
		t.Fatal("frame change mutated shared material or retained previous channel")
	}
	if !m.SetTexture("EmissionMap", material.Tex{Texture: gpu.TextureHandle{Index: 2, Gen: 1}, Region: f.res.regions[ids.AssetID{Lo: 2}], EmissionMode: texture.EmissionMask}) {
		t.Fatal("explicit map rejected")
	}
	f.tick()
	if f.r.Bucket(0).Sprites[0].Emission.Mode != texture.EmissionMask {
		t.Fatal("material mode failed to override frame role")
	}
	m.Reset("EmissionMap")
	f.res.emissionPairs[sr.Sprite.ID] = ids.AssetID{Lo: 404}
	f.tick()
	if f.r.Bucket(0).Sprites[0].Emission.Valid() || f.res.emissionPairs[sr.Sprite.ID] != (ids.AssetID{Lo: 404}) {
		t.Fatal("missing emission was discarded or rendered as valid")
	}
	ds, _ := LightingDiagnostics(f.w)
	if len(ds) != 1 || ds[0].Code != "LGT006" {
		t.Fatalf("missing emission diagnostic absent: %v", ds)
	}
}

func TestLightingAnimationProprefsAndScriptViewStateDoNotMutateEdit(t *testing.T) {
	f := lightingTestWorld(t)
	f.camera("cam", 1)
	h := f.light("light", lighting.Point)
	f.tick()
	data := sceneio.Save(f.s)
	play, _, err := sceneio.Load(data, "play.aescene", f.s.Types(), f.res, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	ph := play.Handle(f.s.ID(h))
	resolver := propref.New(play, nil)
	for _, path := range []string{"Light2D.intensity", "Light2D.range", "Light2D.innerAngle", "Light2D.outerAngle", "Light2D.color"} {
		ref, err := resolver.Resolve(ph, path)
		if err != nil {
			t.Fatal(err)
		}
		if !resolver.Write(&ref, ph, [4]float32{.5, .4, .3, 1}) {
			t.Fatalf("cannot animate %s", path)
		}
	}
	if string(sceneio.Save(f.s)) != string(data) {
		t.Fatal("Play propref changed EditScene")
	}
	c := &bind.Ctx{World: f.w, Scene: f.s}
	setLightingEnabled(c, false)
	setLightingDebug(c, "shadows")
	if c.Failed() || lightingEnabled(c) || lightingDebug(c) != "shadows" || string(sceneio.Save(f.s)) != string(data) {
		t.Fatal("Script view control mutated serialized scene")
	}
	bad := &bind.Ctx{World: f.w}
	setLightingDebug(bad, "unknown")
	if !bad.Failed() || lightingDebug(c) != "shadows" {
		t.Fatal("invalid Script mode changed view")
	}
}

func TestLightingPreparedStableCPUPathNoAllocationsAndLifetime(t *testing.T) {
	f := lightingTestWorld(t)
	f.camera("cam", ^uint64(0))
	f.light("point", lighting.Point)
	f.light("spot", lighting.Spot)
	h := f.s.Create(f.s.Root(), "shape")
	*scene.Add[LightOccluder2D](f.s, h) = DefaultLightOccluder2D()
	f.tick()
	if n := testing.AllocsPerRun(300, f.tick); n != 0 {
		t.Fatalf("warmed lighting extract path allocates %v times per frame", n)
	}
	s, _ := engine.ServiceOf[*lightingState](f.w, lightingServiceID)
	f.m.DetachWorld(&module.AttachContext{World: f.w, Scene: f.s})
	if len(s.cache) != 0 || s.scene != nil {
		t.Fatal("detach retained local geometry")
	}
	if ds, _ := f.p.Collect(); len(ds) != 0 {
		t.Fatal("detach retained Problems source")
	}
	if _, ok := f.w.ServiceAny(lightingServiceID); ok {
		t.Fatal("world retained lighting state")
	}
}

var _ asset.NormalSource = (*lightingResolver)(nil)

type metadataCookieResolver struct {
	materialResolver
	info         asset.TextureMetadata
	found        bool
	textureCalls int
}

func (r *metadataCookieResolver) TextureMetadata(ids.AssetID) (asset.TextureMetadata, bool) {
	return r.info, r.found
}
func (r *metadataCookieResolver) Texture(ids.AssetID) (texture.Region, bool) {
	r.textureCalls++
	return texture.Region{}, false
}

func TestLightingCookieValidationUsesPreparedSemanticsWithoutGPU(t *testing.T) {
	f := lightingTestWorld(t)
	for _, semantic := range []asset.TextureSemantic{asset.TextureColor, asset.TextureData, asset.TextureNormal, asset.TextureMask, asset.TextureHeight, asset.TextureCookieMask, asset.TextureEmissionRGB, asset.TextureEmissionMask} {
		t.Run(strconv.Itoa(int(semantic)), func(t *testing.T) {
			r := &metadataCookieResolver{info: asset.TextureMetadata{Width: 32, Height: 16, Semantic: semantic}, found: true}
			s := scene.New(f.s.Types(), r, scene.Options{})
			h := s.Create(s.Root(), "cookie-light")
			l := DefaultLight2D()
			l.Type = lighting.Sprite
			l.Cookie.ID = ids.AssetID{Lo: 19}
			ds := validateLight2D(module.ValidateContext{Scene: s, Node: h}, &l)
			allowed := semantic == asset.TextureData || semantic == asset.TextureMask || semantic == asset.TextureHeight || semantic == asset.TextureCookieMask
			if allowed && len(ds) != 0 {
				t.Fatalf("prepared headless cookie rejected: %+v", ds)
			}
			if !allowed && (len(ds) == 0 || !ds[0].BlocksPlay) {
				t.Fatal("color/normal cookie semantics were accepted")
			}
			if r.textureCalls != 0 {
				t.Fatal("validation requested a GPU texture despite prepared metadata")
			}
			r.found = false
			ds = validateLight2D(module.ValidateContext{Scene: s, Node: h}, &l)
			if len(ds) != 1 || ds[0].Code != "LGT006" || l.Cookie.ID != (ids.AssetID{Lo: 19}) {
				t.Fatal("missing cookie should retain ID and expose diagnostic")
			}
		})
	}
}
