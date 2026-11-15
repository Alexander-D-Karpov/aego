package gpupreview

import (
	"bytes"
	"errors"
	"image"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/core/mathx"
	"aego/modules/particles"
	"aego/modules/render2d"
	"aego/scene"
	"aego/schema"
)

func TestFrameSpecificPivotWinsOverLaterDefault(t *testing.T) {
	d, err := art.New(4, 4, art.RGBA, art.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	f := d.Info().Frames[0].ID
	e, err := d.Begin(d.Revision(), "pivots")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []art.Slice{
		{Name: "pivot", Frame: f, Rect: image.Rect(0, 0, 4, 4), Pivot: image.Pt(1, 3)},
		{Name: "pivot", Rect: image.Rect(0, 0, 4, 4), Pivot: image.Pt(2, 2)},
	} {
		if _, err = e.SetSlice(s); err != nil {
			t.Fatal(err)
		}
	}
	change, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err = change.Apply(); err != nil {
		t.Fatal(err)
	}
	data, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err = a.Publish(1, d.Revision(), data); err != nil {
		t.Fatal(err)
	}
	sr, _ := scene.Get[render2d.SpriteRenderer](a.sc, a.sprite)
	if want := mathx.V2(.25, .75); sr.Origin != want {
		t.Fatalf("preview origin %v differs from frame pivot %v", sr.Origin, want)
	}
	// A presentation option change must preserve the same canonical pivot.
	o := DefaultOptions()
	o.FlipX = true
	o.Shadows = true
	o.Mode = "lit"
	if err = a.Configure(o); err != nil {
		t.Fatal(err)
	}
	if sr.Origin != mathx.V2(.25, .75) {
		t.Fatal("reconfiguration lost the frame pivot")
	}
}

func fixture(t *testing.T) (*art.Document, []byte) {
	t.Helper()
	d, e := art.New(16, 16, art.RGBA, art.Limits{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(d.Close)
	x, e := d.Begin(d.Revision(), "fixture")
	if e != nil {
		t.Fatal(e)
	}
	if e = x.SetPixel(d.Info().Cels[0].ID, 5, 6, art.Paint{Color: art.Color{R: 255, G: 96, B: 32, A: 128}}, nil, false); e != nil {
		t.Fatal(e)
	}
	c, e := x.Prepare()
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Apply(); e != nil {
		t.Fatal(e)
	}
	b, e := d.Encode()
	if e != nil {
		t.Fatal(e)
	}
	return d, b
}

func TestPreviewUsesRuntimeWorldAndNeverMutatesAuthoringDocument(t *testing.T) {
	d, b := fixture(t)
	a, e := New(true)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	if e = a.Publish(1, d.Revision(), b); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 4; i++ {
		a.Step()
	}
	if a.Status().State != "ready" {
		t.Fatalf("preview not ready: %+v", a.Status())
	}
	if e = a.Publish(1, d.Revision(), b); !errors.Is(e, art.ErrConflict) {
		t.Fatalf("stale publish: %v", e)
	}
	o := DefaultOptions()
	o.Mode = "particles"
	if e = a.Configure(o); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 30; i++ {
		a.Step()
	}
	p := particles.Of(a.world)
	if p == nil || p.Stats().Alive == 0 {
		t.Fatalf("real ParticleSystem2D did not simulate: %+v", a.Status())
	}
	pc, ok := scene.Get[particles.ParticleSystem2D](a.sc, a.emitter)
	if !ok || len(pc.Frames) != 1 {
		t.Fatal("flipbook not assigned")
	}
	o.Mode = "lit"
	o.Bloom = true
	o.Shadows = true
	o.FlipX = true
	if e = a.Configure(o); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 4; i++ {
		a.Step()
	}
	sr, _ := scene.Get[render2d.SpriteRenderer](a.sc, a.sprite)
	if !sr.FlipX {
		t.Fatal("preview flip not applied")
	}
	post, _ := scene.Get[render2d.PostProcess](a.sc, a.camera)
	if !post.Enabled || len(post.Effects) != 1 {
		t.Fatal("runtime Bloom not configured")
	}
	after, e := d.Encode()
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(after, b) {
		t.Fatal("preview mutated authoring state")
	}
}

func TestInvalidSnapshotAndMissingChannelDoNotPublishOldPixels(t *testing.T) {
	d, b := fixture(t)
	a, e := New(true)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	if e = a.Publish(1, d.Revision(), b); e != nil {
		t.Fatal(e)
	}
	if e = a.Publish(2, d.Revision(), []byte("bad")); e == nil {
		t.Fatal("invalid snapshot accepted")
	}
	if a.Status().Sequence != 1 {
		t.Fatal("failed publication changed generation")
	}
	o := DefaultOptions()
	o.Mode = "normal"
	if e = a.Configure(o); e != nil {
		t.Fatal(e)
	}
	if a.Status().Error == "" {
		t.Fatal("missing normal channel not diagnosed")
	}
	o.Mode = "sprite"
	if e = a.Configure(o); e != nil {
		t.Fatal(e)
	}
	if a.Status().Error != "" {
		t.Fatal("diagnostic survived corrected mode")
	}
}

func TestLitEmissionBelongsToArtworkAndCurrentFrame(t *testing.T) {
	d, _ := fixture(t)
	i := d.Info()
	first, layer := i.Frames[0].ID, i.Layers[0].ID
	e, err := d.Begin(d.Revision(), "emissive and albedo-only frames")
	if err != nil {
		t.Fatal(err)
	}
	emission, err := e.EnsureCel(layer, first, art.Emission)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.SetPixel(emission, 5, 6, art.Paint{Color: art.Color{R: 255, G: 160, B: 32, A: 255}}, nil, false); err != nil {
		t.Fatal(err)
	}
	second, err := e.AddFrame(first, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.CopyCel(i.Cels[0].ID, layer, second, false); err != nil {
		t.Fatal(err)
	}
	c, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Apply(); err != nil {
		t.Fatal(err)
	}
	before, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err = a.Publish(1, d.Revision(), before); err != nil {
		t.Fatal(err)
	}
	base, ok := a.loader.Material(asset.BuiltinMaterialLit)
	if !ok {
		t.Fatal("stock Lit material missing")
	}
	baseColor, _ := base.Get("EmissionColor")
	baseIntensity, _ := base.Get("EmissionIntensity")
	baseGeneration := base.Generation()
	if baseColor != [4]float32{0, 0, 0, 1} || baseIntensity != [4]float32{} {
		t.Fatal("unexpected stock emission defaults", baseColor, baseIntensity)
	}
	o := DefaultOptions()
	o.Shadows, o.Emission = true, 16
	for _, mode := range []string{"lit", "flash", "lit", "dissolve", "sprite", "lit"} {
		for _, f := range []art.ID{first, second, first} {
			o.Mode, o.Frame = mode, f
			if err = a.Configure(o); err != nil {
				t.Fatal(mode, f, err)
			}
			if mode == "lit" {
				color, err := render2d.GetMaterialParameter(a.sc, a.sprite, "EmissionColor")
				if err != nil || color.Kind != schema.KindColor || color.C != (mathx.Color{R: 1, G: 1, B: 1, A: 1}) {
					t.Fatal("Artwork must use white emission modulation", color, err)
				}
				want := float32(0)
				if f == first {
					want = 16
				}
				intensity, err := render2d.GetMaterialParameter(a.sc, a.sprite, "EmissionIntensity")
				if err != nil || intensity.Kind != schema.KindFloat32 || intensity.F != float64(want) || a.Status().HasEmission != (f == first) {
					t.Fatal("emission must follow the current frame's actual channel", f, intensity, err, a.Status())
				}
			} else if override, ok := scene.Get[render2d.MaterialOverride](a.sc, a.sprite); ok && len(override.Values) != 0 {
				t.Fatal("Lit parameters remained on an incompatible preview shader", mode, override.Values)
			}
			floorColor, err := render2d.GetMaterialParameter(a.sc, a.floor, "EmissionColor")
			if err != nil || floorColor.Kind != schema.KindColor || floorColor.C != (mathx.Color{A: 1}) {
				t.Fatal("shadow receiver unexpectedly became emissive", floorColor, err)
			}
			floorIntensity, err := render2d.GetMaterialParameter(a.sc, a.floor, "EmissionIntensity")
			if err != nil || floorIntensity.Kind != schema.KindFloat32 || floorIntensity.F != 0 || scene.Has[render2d.MaterialOverride](a.sc, a.floor) {
				t.Fatal("Artwork preview changed receiver material parameters", floorIntensity, err)
			}
			if color, _ := base.Get("EmissionColor"); color != baseColor {
				t.Fatal("shared Lit color changed")
			}
			if intensity, _ := base.Get("EmissionIntensity"); intensity != baseIntensity || base.Generation() != baseGeneration {
				t.Fatal("preview changed shared material or shader generation")
			}
		}
	}
	after, err := d.Encode()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("preview controls changed the canonical art document", err)
	}
}
