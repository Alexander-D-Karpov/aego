package artsource

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset"
	teximport "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
)

// Comparing the prepared artifacts, rather than only the edit model, catches
// changes in channel semantics and hidden RGB used by the real runner shader.
func TestRunnerSurfaceArtifactsKeepFramePixels(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "platformer", "assets", "surfaces")
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	colorPNG, normalPNG := read("runner.png"), read("runner_n.png")
	colorMeta, err := asset.ParseSourceMeta(read("runner.png.meta"), "runner.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	normalMeta, err := asset.ParseSourceMeta(read("runner_n.png.meta"), "runner_n.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	if len(colorMeta.Subs) != 8 || len(normalMeta.Subs) != 8 {
		t.Fatal("unexpected canonical runner slices")
	}
	albedoImage, err := png.Decode(bytes.NewReader(colorPNG))
	if err != nil {
		t.Fatal(err)
	}
	normalImage, err := png.Decode(bytes.NewReader(normalPNG))
	if err != nil {
		t.Fatal(err)
	}
	d, err := art.New(16, 16, art.RGBA, art.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	info := d.Info()
	e, err := d.Begin(d.Revision(), "Reproduce runner surface frames")
	if err != nil {
		t.Fatal(err)
	}
	frame, layer := info.Frames[0].ID, info.Layers[0].ID
	for i, sub := range colorMeta.Subs {
		if i > 0 {
			frame, err = e.AddFrame(frame, 100)
			if err != nil {
				t.Fatal(err)
			}
		}
		r := sub.Rect
		if r[2] != 16 || r[3] != 16 {
			t.Fatalf("unexpected runner frame %d: %v", i, r)
		}
		for _, source := range []struct {
			ch  art.Channel
			img image.Image
		}{{art.Albedo, albedoImage}, {art.Normal, normalImage}} {
			cel, err := e.EnsureCel(layer, frame, source.ch)
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					v := color.NRGBAModel.Convert(source.img.At(r[0]+x, r[1]+y)).(color.NRGBA)
					if err := e.SetPixel(cel, x, y, art.Paint{Color: art.Color{R: v.R, G: v.G, B: v.B, A: v.A}}, nil, false); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	change, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	src, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	newMeta := asset.NewSourceMeta("artsource", Version, asset.TextureDefaults())
	for i, frame := range d.Info().Frames {
		newMeta.Subs = append(newMeta.Subs,
			asset.SourceFrame{Name: FrameName(frame.ID, art.Albedo), ID: colorMeta.Subs[i].ID},
			asset.SourceFrame{Name: FrameName(frame.ID, art.Normal), ID: normalMeta.Subs[i].ID})
	}
	oldColor, err := teximport.New().Import(&pipeline.ImportContext{ID: colorMeta.ID, Path: "assets/surfaces/runner.png", Source: colorPNG, Meta: colorMeta})
	if err != nil {
		t.Fatal(err)
	}
	oldNormal, err := teximport.New().Import(&pipeline.ImportContext{ID: normalMeta.ID, Path: "assets/surfaces/runner_n.png", Source: normalPNG, Meta: normalMeta})
	if err != nil {
		t.Fatal(err)
	}
	converted, err := New().Import(&pipeline.ImportContext{ID: newMeta.ID, Path: "assets/surfaces/runner.aeart", Source: src, Meta: newMeta})
	if err != nil {
		t.Fatal(err)
	}
	getTex := func(list []pipeline.Produced) *asset.TextureArtifact {
		t.Helper()
		for _, a := range list {
			if a.Kind == asset.ArtTexture {
				tex, err := asset.DecodeTexture(a.Data)
				if err != nil {
					t.Fatal(err)
				}
				return tex
			}
		}
		t.Fatal("missing texture artifact")
		return nil
	}
	getRegion := func(result *pipeline.Result, id ids.AssetID) *asset.RegionArtifact {
		t.Helper()
		for _, sub := range result.Subs {
			if sub.ID == id {
				for _, a := range sub.Artifacts {
					if a.Kind == asset.ArtSpriteRegion {
						v, err := asset.DecodeRegion(a.Data)
						if err != nil {
							t.Fatal(err)
						}
						return v
					}
				}
			}
		}
		t.Fatalf("missing region %s", id)
		return nil
	}
	newAlbedo := getTex(converted.Artifacts)
	var newNormal *asset.TextureArtifact
	for _, sub := range converted.Subs {
		if sub.Name == "surface_normal" {
			newNormal = getTex(sub.Artifacts)
		}
	}
	if newNormal == nil {
		t.Fatal("normal sheet missing")
	}
	oldAlbedoTex, oldNormalTex := getTex(oldColor.Artifacts), getTex(oldNormal.Artifacts)
	compare := func(label string, a, b *asset.TextureArtifact, ar, br *asset.RegionArtifact) {
		t.Helper()
		if a.Format != b.Format || a.Semantic != b.Semantic || ar.W != br.W || ar.H != br.H || ar.Filter != br.Filter || ar.HasPivot != br.HasPivot || ar.Pivot != br.Pivot {
			t.Fatalf("%s texture or region metadata changed", label)
		}
		for y := 0; y < ar.H; y++ {
			x := ((ar.Y+y)*a.Width + ar.X) * 4
			z := ((br.Y+y)*b.Width + br.X) * 4
			if !bytes.Equal(a.Pixels[x:x+ar.W*4], b.Pixels[z:z+br.W*4]) {
				t.Fatalf("%s prepared pixels differ in row %d", label, y)
			}
		}
	}
	for i, sub := range colorMeta.Subs {
		before, after := getRegion(oldColor, sub.ID), getRegion(converted, sub.ID)
		if before.Normal != normalMeta.Subs[i].ID || after.Normal != before.Normal {
			t.Fatalf("normal AssetID changed: before=%s after=%s", before.Normal, after.Normal)
		}
		compare("albedo", oldAlbedoTex, newAlbedo, before, after)
		compare("normal", oldNormalTex, newNormal, getRegion(oldNormal, normalMeta.Subs[i].ID), getRegion(converted, normalMeta.Subs[i].ID))
	}
}

func TestSurfaceArtifactRetainsDataRGBAtZeroAlpha(t *testing.T) {
	d, err := art.New(16, 16, art.RGBA, art.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	info := d.Info()
	e, err := d.Begin(d.Revision(), "surface pixels")
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct {
		channel art.Channel
		value   art.Color
	}{
		{art.Normal, art.Color{R: 128, G: 129, B: 255}}, {art.Height, art.Color{R: 91, G: 91, B: 91}}, {art.Emission, art.Color{R: 33, G: 144, B: 233}},
	} {
		cel, err := e.EnsureCel(info.Layers[0].ID, info.Frames[0].ID, spec.channel)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.SetPixel(cel, 0, 0, art.Paint{Color: spec.value}, nil, false); err != nil {
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
	data, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	meta := asset.NewSourceMeta("artsource", Version, asset.TextureDefaults())
	result, err := New().Import(&pipeline.ImportContext{ID: meta.ID, Path: "assets/surface.aeart", Source: data, Meta: meta})
	if err != nil {
		t.Fatal(err)
	}
	// The emission importer deliberately makes its alpha opaque; RGB is data.
	want := map[string][]byte{"surface_normal": {128, 129, 255, 0}, "surface_height": {91, 91, 91, 0}, "surface_emission": {33, 144, 233, 255}}
	for _, sub := range result.Subs {
		v, ok := want[sub.Name]
		if !ok {
			continue
		}
		if len(sub.Artifacts) != 1 || sub.Artifacts[0].Kind != asset.ArtTexture {
			t.Fatalf("%s has no texture artifact", sub.Name)
		}
		tex, err := asset.DecodeTexture(sub.Artifacts[0].Data)
		if err != nil {
			t.Fatal(err)
		}
		if len(tex.Pixels) < 4 || !bytes.Equal(tex.Pixels[:4], v) {
			t.Fatalf("%s lost RGB: %v want %v", sub.Name, tex.Pixels, v)
		}
		delete(want, sub.Name)
	}
	if len(want) != 0 {
		t.Fatalf("surface artifacts missing: %v", want)
	}
}
