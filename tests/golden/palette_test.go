//go:build aego_golden

package golden

import (
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/templates"
	"aego/gpu"
	"aego/project"
	"aego/render/view"
	"aego/scene"
	"aego/scene/sceneio"
)

const paletteState = `palette version=1 {
  components recent=["comp:gameplay.Motion2D", "comp:physics.BoxCollider2D", "comp:gameplay.Health"] favorites=["comp:camera.VirtualCamera2D"] open=["Gameplay"] {}
  create recent=["tpl:engine.empty"] favorites=[] open=[] {}
  use key="comp:gameplay.Motion2D" count=3 {}
}
`

func paletteSession(t *testing.T) *edit.Session {
	t.Helper()
	runtime.LockOSThread()
	p, err := project.Create(t.TempDir(), "PaletteGolden", false)
	if err != nil {
		t.Fatal(err)
	}
	src := scene.New(goldenTypes(t), nil, scene.Options{})
	cameraScene(src)
	src.Flush()
	rel := "scenes/golden.aescene"
	if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(rel)), sceneio.Save(src), 0o644); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(p.Root, ".aego", "user")
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "palette.aetext"), []byte(paletteState), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Hidden: true, NoWatch: true, Width: 1000, Height: 620, Logger: log.Nop()})
	if err != nil {
		t.Skipf("no GL context available: %v", err)
	}
	t.Cleanup(s.Close)
	if name := s.Engine.Caps().Renderer; !softwareRenderer(name) && os.Getenv("AEGO_GOLDEN_ALLOW_HARDWARE") == "" {
		t.Fatalf("goldens require a software rasterizer, got %q", name)
	}
	s.Step()
	d, err := s.App.Context().Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	sc := d.Scene()
	s.App.Context().Select(sc.ID(sc.Find(sc.Root(), "Player")))
	s.Step()
	return s
}

func paletteShot(t *testing.T, s *edit.Session, name string) {
	t.Helper()
	for range 4 {
		s.Step()
	}
	rectShot(t, s, s.App.AddPalette().Rect(), name)
}

// rectShot renders the editor UI and compares the part inside r, in UI units, with a golden.
func rectShot(t *testing.T, s *edit.Session, r mathx.Rect, name string) {
	t.Helper()
	e := s.Engine
	m := e.Metrics()
	w, h := m.PixelW, m.PixelH
	ui := s.App.Context().UI
	tgt, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: w, Height: h, Format: gpu.TexSRGBA8, Label: "golden.palette"})
	if err != nil {
		t.Fatal(err)
	}
	frame := e.GPU().BeginFrame(w, h)
	if err := s.Renderer.BeginFrame(frame, w, h, 1); err != nil {
		t.Fatal(err)
	}
	v := view.ScreenView(w, h, m.Density())
	v.Target, v.Clear, v.DoClear, v.Label = tgt, ui.Theme.Background, true, "golden.palette"
	f := s.Renderer.View(v)
	white := s.Renderer.White()
	if fnt := ui.Font(); fnt != nil && !fnt.White.IsZero() {
		white = fnt.White
	}
	s.App.UIList().Flush(f, ui.Font(), white, 0)
	f.End()
	s.Renderer.EndFrame()
	e.GPU().EndFrame()
	buf := make([]byte, w*h*4)
	err = e.GPU().ReadPixels(tgt, 0, 0, w, h, buf)
	full := &image.NRGBA{Pix: buf, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	dens := float32(m.Density())
	box := image.Rect(int(r.X*dens)-8, int(r.Y*dens)-8, int((r.X+r.W)*dens)+8, int((r.Y+r.H)*dens)+8).Intersect(full.Rect)
	img := image.NewNRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	draw.Draw(img, img.Rect, full, box.Min, draw.Src)
	checkGolden(t, name, img, err)
}

func TestPaletteRecentGolden(t *testing.T) {
	s := paletteSession(t)
	ctx := s.App.Context()
	s.App.AddPalette().AddComponent(ctx.Selected(), nil)
	paletteShot(t, s, "editor_palette_recent")
}

func TestPaletteSearchGolden(t *testing.T) {
	s := paletteSession(t)
	ctx := s.App.Context()
	pal := s.App.AddPalette()
	pal.AddComponent(ctx.Selected(), nil)
	pal.Palette().SetQuery("box")
	paletteShot(t, s, "editor_palette_search")
}

func TestPaletteCreateGolden(t *testing.T) {
	s := paletteSession(t)
	pal := s.App.AddPalette()
	pal.CreateNode(s.App.Context().Selected()[0], templates.Place{}, nil, nil)
	pal.Palette().SetQuery("cam")
	paletteShot(t, s, "editor_palette_create")
}

// Components the node already has stay selectable and show why they cannot be added.
func TestPaletteDisabledGolden(t *testing.T) {
	s := paletteSession(t)
	ctx := s.App.Context()
	pal := s.App.AddPalette()
	pal.AddComponent(ctx.Selected(), nil)
	pal.Palette().SetQuery("ri")
	paletteShot(t, s, "editor_palette_disabled")
}
