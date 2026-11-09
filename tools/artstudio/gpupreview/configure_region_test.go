package gpupreview

import (
	"bytes"
	"image"
	"testing"

	"aego/art"
	"aego/modules/render2d"
	"aego/render/texture"
	"aego/scene"
)

// A presentation-only selection does not republish the document or change the
// loader generation. Verify the actual runtime draw region, not merely the
// selected Frame/Mode status. The headless GPU does not rasterize: framebuffer
// pixels are checked separately by the real browser GPU acceptance suite.
func TestConfigureChangesCachedFrameAndChannelRegion(t *testing.T) {
	d, err := art.New(4, 4, art.RGBA, art.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	info := d.Info()
	layer, first := info.Layers[0].ID, info.Frames[0].ID
	e, err := d.Begin(d.Revision(), "distinct preview surfaces")
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.AddFrame(first, 100)
	if err != nil {
		t.Fatal(err)
	}
	frames := []art.ID{first, second}
	channels := []art.Channel{art.Albedo, art.Normal, art.Height, art.Emission}
	colors := [2][4]art.Color{
		{{R: 240, A: 255}, {R: 128, G: 128, B: 255, A: 255}, {R: 40, G: 40, B: 40, A: 255}, {R: 255, G: 220, A: 255}},
		{{B: 240, A: 255}, {R: 255, G: 128, B: 128, A: 255}, {R: 200, G: 200, B: 200, A: 255}, {R: 220, B: 255, A: 255}},
	}
	for fi, frame := range frames {
		for ci, channel := range channels {
			cel, err := e.EnsureCel(layer, frame, channel)
			if err != nil {
				t.Fatal(err)
			}
			for y := range 4 {
				for x := range 4 {
					if err := e.SetPixel(cel, x, y, art.Paint{Color: colors[fi][ci]}, nil, false); err != nil {
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
	before, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for fi, frame := range frames {
		for ci, channel := range channels {
			pixels, err := d.Render(frame, art.RenderOptions{Channel: channel})
			if err != nil || pixels.Bounds() != image.Rect(0, 0, 4, 4) {
				t.Fatal("canonical surface fixture", frame, channel, err)
			}
			want := colors[fi][ci]
			if got := pixels.NRGBAAt(1, 1); got.R != want.R || got.G != want.G || got.B != want.B || got.A != want.A {
				t.Fatal("canonical fixture did not retain distinct channel pixels", frame, channel, got, want)
			}
		}
	}
	for _, selected := range []struct {
		name    string
		frame   art.ID
		channel art.Channel
	}{
		{"frame", second, art.Albedo},
		{"normal", first, art.Normal},
		{"height", first, art.Height},
		{"emission", first, art.Emission},
	} {
		t.Run(selected.name, func(t *testing.T) {
			a, err := New(true)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if err := a.Publish(1, d.Revision(), before); err != nil {
				t.Fatal(err)
			}
			sr, ok := scene.Get[render2d.SpriteRenderer](a.sc, a.sprite)
			if !ok {
				t.Fatal("runtime Artwork sprite is missing")
			}
			initial, ok := sr.Sprite.Region(a.loader) // Populate the real Ref cache.
			if !ok {
				t.Fatal("initial albedo region is missing")
			}
			wantID := a.data.frames[selected.frame][selected.channel]
			want, ok := a.loader.Texture(wantID)
			if !ok || want == initial {
				t.Fatal("fixture needs a distinct prepared texture/region", wantID)
			}
			generation := a.loader.Generation()
			o := DefaultOptions()
			o.Frame = selected.frame
			if selected.channel != art.Albedo {
				o.Mode = string(selected.channel)
			}
			if err := a.Configure(o); err != nil {
				t.Fatal(err)
			}
			if a.loader.Generation() != generation || a.Status().Sequence != 1 || a.Status().Revision != d.Revision() {
				t.Fatal("selection unexpectedly republished the document or invalidated the entire loader")
			}
			if sr.Sprite.ID != wantID || a.Status().Frame != selected.frame || a.Status().Mode != o.Mode {
				t.Fatal("selection status/reference did not follow the requested frame/channel")
			}
			assertDraw := func(want texture.Region) {
				t.Helper()
				got, ok := sr.Sprite.Region(a.loader)
				if !ok || got != want {
					t.Fatalf("selected AssetRef retains another surface: selected=%s generation=%d got=%+v want=%+v", sr.Sprite.ID, generation, got, want)
				}
				if !a.Step() {
					t.Fatal("preview stopped")
				}
				if len(a.runner.Views()) != 1 || len(a.runner.Bucket(0).Sprites) != 1 || a.runner.Bucket(0).Sprites[0].Region != want {
					t.Fatal("actual runtime draw did not use the selected texture/UV region")
				}
			}
			assertDraw(want)
			// Returning to the first albedo must also invalidate the selected cache.
			o.Mode, o.Frame = "sprite", first
			if err := a.Configure(o); err != nil {
				t.Fatal(err)
			}
			assertDraw(initial)
			if a.loader.Generation() != generation || a.Status().Sequence != 1 {
				t.Fatal("returning to albedo changed the loader generation or publication")
			}
		})
	}
	after, err := d.Encode()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("preview selection changed the authoring document", err)
	}
}
