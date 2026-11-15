//go:build !aego_runtime

package particles

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/core/mathx"
	"aego/core/rng"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/res"
	"aego/gpu/null"
	"aego/project"
	"aego/render"
	"aego/render/texture"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/vfs"
)

// The CPU particle world and asset loader are real; the renderer uses a null
// device. This is not VFX workspace or visible Game View acceptance.
func TestAuthoredFlipbookInActualParticleWorldHeadless(t *testing.T) {
	d, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	info := d.Info()
	first := info.Frames[0].ID
	e, err := d.Begin(d.Revision(), "Draw a two-frame effect")
	if err != nil {
		t.Fatal(err)
	}
	for i, color := range []art.Color{{R: 255, A: 255}, {G: 255, A: 255}} {
		f := first
		if i != 0 {
			f, err = e.AddFrame(first, 100)
			if err != nil {
				t.Fatal(err)
			}
		}
		cel, err := e.EnsureCel(info.Layers[0].ID, f, art.Albedo)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.SetPixel(cel, 1, 1, art.Paint{Color: color}, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	change, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	frames := d.Info().Frames
	if len(frames) != 2 {
		t.Fatalf("frames: %+v", frames)
	}
	if err := change.Undo(); err != nil || len(d.Info().Frames) != 1 {
		t.Fatalf("Undo did not restore the original single-frame drawing: %v", err)
	}
	if err := change.Apply(); err != nil || len(d.Info().Frames) != 2 || d.Info().Frames[1].ID != frames[1].ID {
		t.Fatalf("Redo did not restore the authored flipbook identity: %v", err)
	}
	source, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	p, err := project.Create(t.TempDir(), "Particle Art", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "assets", "effect.aeart"), source, 0644); err != nil {
		t.Fatal(err)
	}
	reg := pipeline.NewImporterRegistry()
	if err := reg.Register(artsource.New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("art import: %+v, %v", batch, err)
	}
	sourceID, ok := pl.Lookup("assets/effect.aeart")
	if !ok {
		t.Fatal("art source not indexed")
	}
	frame0 := artsource.StableID(sourceID, artsource.FrameName(frames[0].ID, art.Albedo))
	frame1 := artsource.StableID(sourceID, artsource.FrameName(frames[1].ID, art.Albedo))
	pageEntry, ok := pl.Entry(sourceID)
	if !ok {
		t.Fatal("prepared art texture not indexed")
	}
	pageArtifact, ok := pageEntry.Artifact(asset.ArtTexture)
	if !ok {
		t.Fatal("prepared art texture is missing")
	}
	pageBytes, err := pl.Artifacts().ReadFile(pageArtifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	page, err := asset.DecodeTexture(pageBytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct {
		id   asset.Ref[asset.Texture]
		want []byte
	}{
		{asset.NewRef[asset.Texture](frame0), []byte{255, 0, 0, 255}},
		{asset.NewRef[asset.Texture](frame1), []byte{0, 255, 0, 255}},
	} {
		entry, ok := pl.Entry(spec.id.ID)
		if !ok {
			t.Fatalf("prepared frame %s not indexed", spec.id.ID)
		}
		artifact, ok := entry.Artifact(asset.ArtSpriteRegion)
		if !ok {
			t.Fatalf("frame %s has no region", spec.id.ID)
		}
		data, err := pl.Artifacts().ReadFile(artifact.Path)
		if err != nil {
			t.Fatal(err)
		}
		region, err := asset.DecodeRegion(data)
		if err != nil || region.Page != sourceID {
			t.Fatalf("invalid frame region: %+v, %v", region, err)
		}
		at := ((region.Y+1)*page.Width + region.X + 1) * 4
		if at < 0 || at+4 > len(page.Pixels) || !bytes.Equal(page.Pixels[at:at+4], spec.want) {
			t.Fatalf("authored pixel for %s was not prepared", spec.id.ID)
		}
	}

	dev := null.New()
	defer dev.Close()
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	rnd, err := render.New(dev, dev.Caps(), files, log.Nop(), render.Options{MaxSprites: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer rnd.Close()
	ld := loader.New(dev, rnd.Textures(), log.Nop())
	if err := ld.Mount(pl.Index(), pl.Artifacts()); err != nil {
		t.Fatal(err)
	}
	defer ld.Close()
	region0, ok0 := ld.Texture(frame0)
	region1, ok1 := ld.Texture(frame1)
	if !ok0 || !ok1 || region0 == region1 {
		t.Fatalf("authored frames do not resolve distinctly: %+v/%v %+v/%v", region0, ok0, region1, ok1)
	}
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	if err := m.Register(module.New(types, nil)); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	sc := scene.New(types, ld, scene.Options{})
	h := sc.Create(sc.Root(), "Authored effect")
	sc.SetPosition(h, mathx.V2(64, 64))
	c := scene.Add[ParticleSystem2D](sc, h)
	*c = DefaultParticleSystem2D()
	c.Frames = []asset.Ref[asset.Texture]{asset.NewRef[asset.Texture](frame0), asset.NewRef[asset.Texture](frame1)}
	c.FrameFPS, c.FrameLoop = 2, false
	c.Rate, c.PlayOnStart = 0, false
	c.Speed, c.Lifetime, c.Size = mathx.V2(0, 0), mathx.V2(5, 5), mathx.V2(10, 10)
	sc.Flush()
	sc.UpdateTransforms()
	encodedScene := sceneio.Save(sc)
	sc, report, err := sceneio.Load(encodedScene, "effect.aescene", types, ld, sceneio.LoadOptions{KeepIDs: true})
	if err != nil || len(report.Unknown) != 0 {
		t.Fatalf("scene reopen: %+v, %v", report, err)
	}
	h = sc.Find(sc.Root(), "Authored effect")
	loaded, ok := scene.Get[ParticleSystem2D](sc, h)
	if !ok || len(loaded.Frames) != 2 || loaded.Frames[0].ID != frame0 || loaded.Frames[1].ID != frame1 {
		t.Fatal("particle scene lost authored frame references")
	}
	sc.Flush()
	sc.UpdateTransforms()
	w := engine.NewWorld("art particle test")
	w.Random = rng.New(17)
	if err := m.Init(&module.InitContext{}); err != nil {
		t.Fatal(err)
	}
	attach := &module.AttachContext{World: w, Scene: sc, Play: true}
	if err := m.AttachWorld(attach); err != nil {
		t.Fatal(err)
	}
	defer m.DetachWorld(attach)
	fx := Of(w)
	fx.Burst(h, 1)
	if fx.Count(h) != 1 {
		t.Fatalf("actual CPU emitter did not create a particle: %+v", fx.Stats())
	}
	r := frame.New()
	defer r.Close()
	r.Bind(sc, rnd, ld)
	draw := func() texture.Region {
		t.Helper()
		r.Prepare(128, 128, 1)
		r.AddView(view.ScreenView(128, 128, 1), 0)
		fx.AppendToView(r)
		if got := len(r.Bucket(0).Sprites); got != 1 {
			t.Fatalf("particle world submitted %d sprites", got)
		}
		return r.Bucket(0).Sprites[0].Region
	}
	if got := draw(); got != region0 {
		t.Fatalf("new particle uses %+v, want authored first frame %+v", got, region0)
	}
	fx.step(.6, .6)
	if got := draw(); got != region1 {
		t.Fatalf("aged particle uses %+v, want authored second frame %+v", got, region1)
	}
}
