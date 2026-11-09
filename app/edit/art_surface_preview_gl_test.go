//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/art"
	"aego/asset"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/platform/sdl"
	"aego/scene"
	"aego/scene/sceneio"
)

// The initial albedo/normal fixture is prepared before launching the editor.
// Emission painting, channel selection and all preview controls below use SDL.
func TestArtSurfaceEmissionBloomNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := newProject(t)
	d, err := art.New(32, 32, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	info := d.Info()
	edit, err := d.Begin(d.Revision(), "Surface fixture")
	if err != nil {
		t.Fatal(err)
	}
	normal, err := edit.EnsureCel(info.Layers[0].ID, info.Frames[0].ID, art.Normal)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if err := edit.SetPixel(info.Cels[0].ID, x, y, art.Paint{Color: art.Color{R: 48, G: 48, B: 48, A: 255}}, nil, false); err != nil {
				t.Fatal(err)
			}
			if err := edit.SetPixel(normal, x, y, art.Paint{Color: art.Color{R: 255, G: 128, B: 128, A: 255}}, nil, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	change, err := edit.Prepare()
	if err != nil || change.Apply() != nil {
		t.Fatal(err)
	}
	saved, err := d.Encode()
	d.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "assets/surface.aeart"), saved, 0644); err != nil {
		t.Fatal(err)
	}
	writeScene(t, p, "scenes/surface.aescene", func(sc *scene.Scene) {
		camera := sc.Create(sc.Root(), "Camera")
		*scene.Add[render2d.Camera2D](sc, camera) = render2d.DefaultCamera2D()
		node := sc.Create(sc.Root(), "Unchanged scene sprite")
		sp := scene.Add[render2d.SpriteRenderer](sc, node)
		*sp = render2d.DefaultSpriteRenderer()
		sp.Sprite.SetAssetRefID(asset.BuiltinWhite)
		sp.Material.SetAssetRefID(asset.BuiltinMaterialLit)
	})
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	step := func() {
		t.Helper()
		if !s.Step() {
			t.Fatal("editor stopped during native surface preview")
		}
	}
	step()
	ed, ws := s.App.Context(), s.App.Workspace()
	sceneDoc, err := ed.Docs.Open("scenes/surface.aescene")
	if err != nil {
		t.Fatal(err)
	}
	sceneBefore := sceneio.Save(sceneDoc.Scene())
	shared, ok := ed.Loader.Material(asset.BuiltinMaterialLit)
	if !ok {
		t.Fatal("shared Lit material missing")
	}
	materialBefore, _ := shared.Get("EmissionIntensity")
	doc, err := ed.OpenAssetPath("assets/surface.aeart")
	if err != nil {
		t.Fatal(err)
	}
	model := doc.Binding.Model.(*artedit.Document)
	maximize := func(id string) mathx.Rect {
		t.Helper()
		if ws.Tree.Maximized() != "" {
			ws.Tree.Restore()
		}
		ws.Show(id)
		ws.Focus(id)
		if !ws.ToggleMaximize(id) {
			t.Fatal("cannot maximize", id)
		}
		for range 3 {
			step()
		}
		r, ok := ws.Tree.PanelRect(id)
		if !ok {
			t.Fatal("missing panel", id)
		}
		return r
	}
	click := func(x, y float32) {
		t.Helper()
		for kind := range 3 {
			if !sdl.TestPointerEvent(x, y, kind) {
				t.Fatal("SDL pointer failed")
			}
			step()
		}
	}
	key := func(k input.Scancode, ctrl bool) {
		t.Helper()
		if !sdl.TestEditKey(k, true, ctrl, false) {
			t.Fatal("SDL key failed")
		}
		step()
		if !sdl.TestEditKey(k, false, false, false) {
			t.Fatal("SDL key release failed")
		}
		step()
	}
	ui := ed.UI
	pad, row, gap := ui.Theme.Padding, ui.Theme.RowHeight, ui.Theme.Spacing
	props := maximize("art.properties")
	click(props.X+100, props.Y+pad+row/2)
	click(props.X+100, props.Y+pad+row+pad+3*(row+gap)+row/2)
	if model.Session.Channel != art.Emission {
		t.Fatal("native surface selector did not choose emission")
	}
	canvas := maximize("render2d.art/" + doc.Asset.String())
	click(canvas.X+canvas.W/2, canvas.Y+canvas.H/2)
	if model.History.Cursor() != 1 || !doc.Dirty() {
		t.Fatal("SDL emission paint did not create one dirty operation")
	}
	emission, _, err := model.Capture(model.Session.Frame, art.Emission)
	if err != nil || !bytes.Contains(emission.Pix, []byte{255, 255, 255, 255}) {
		t.Fatal("canonical dirty emission has no painted white pixel", err)
	}
	preview := maximize("art.preview")
	click(preview.X+100, preview.Y+pad+row/2)
	click(preview.X+100, preview.Y+pad+row+pad+(row+gap)+row/2)
	deadline := time.Now().Add(8 * time.Second)
	for (doc.WorkingPreview.Pending || !ed.WorkingSourceCurrent(doc, model.PreviewRevision())) && doc.WorkingPreview.Error == "" && time.Now().Before(deadline) {
		step()
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if doc.WorkingPreview.Error != "" || !ed.WorkingSourceCurrent(doc, model.PreviewRevision()) {
		t.Fatal("working asset did not reach prepared GPU preview", doc.WorkingPreview)
	}
	for range 4 {
		step()
	}
	capture := func(name string) *image.RGBA {
		t.Helper()
		var pixels *image.RGBA
		var captureErr error
		s.Engine.CaptureFrame(func(v *image.RGBA, err error) { pixels, captureErr = v, err })
		step()
		if pixels == nil || captureErr != nil {
			t.Fatal(captureErr)
		}
		if dir := os.Getenv("AEGO_ART_SURFACE_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, pixels)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
		}
		return pixels
	}
	width := preview.W - 2*pad
	controlY := preview.Y + pad + (row + gap) + row/2
	click(preview.X+pad+width*5/8, controlY) // Emission off.
	for range 2 {
		step()
	}
	off := capture("01-lit-emission-off")
	click(preview.X+pad+width*5/8, controlY)
	for range 2 {
		step()
	}
	on := capture("02-lit-dirty-emission")
	crop := image.Rect(int(preview.X+pad), int(preview.Y+pad+5*(row+gap)), int(preview.X+preview.W-pad), int(preview.Y+preview.H-pad))
	changed := 0
	brightBounds := image.Rectangle{}
	for y := crop.Min.Y; y < crop.Max.Y; y++ {
		for x := crop.Min.X; x < crop.Max.X; x++ {
			a, b := off.RGBAAt(x, y), on.RGBAAt(x, y)
			if int(b.R) > int(a.R)+40 {
				changed++
				point := image.Rect(x, y, x+1, y+1)
				if brightBounds.Empty() {
					brightBounds = point
				} else {
					brightBounds = brightBounds.Union(point)
				}
			}
		}
	}
	if changed < 20 {
		t.Fatal("real prepared dirty emission did not visibly change Lit GPU pixels", changed)
	}
	// Set the allowed maximum preview emission through the real number field.
	// The bright pixel must produce a measurable halo outside its filtered
	// footprint; merely executing Bloom passes is insufficient evidence.
	click(preview.X+pad+width*5/6, preview.Y+pad+2*(row+gap)+row/2)
	key(input.KeyA, true)
	if !sdl.TestTextInput("16") {
		t.Fatal("SDL numeric edit failed")
	}
	step()
	key(input.KeyEnter, false)
	for range 2 {
		step()
	}
	withoutBloom := capture("03-emission-strength-sixteen")
	click(preview.X+pad+width*7/8, controlY)
	ed.Renderer.InspectOutputPasses()
	step()
	passes := ed.Renderer.InspectOutputPasses()
	bloomPasses := 0
	for _, pass := range passes {
		if strings.Contains(strings.ToLower(pass.Label), "bloom") {
			bloomPasses++
		}
	}
	for range 2 {
		step()
	}
	bloom := capture("04-real-bloom")
	if ed.Renderer.Stats().PostFXPasses < 4 || bloomPasses < 4 {
		t.Fatal("Bloom toggle did not execute real extract/blur/composite passes", ed.Renderer.Stats().PostFXPasses, bloomPasses)
	}
	glow := 0
	outside := brightBounds.Inset(-2)
	for y := crop.Min.Y; y < crop.Max.Y; y++ {
		for x := crop.Min.X; x < crop.Max.X; x++ {
			if image.Pt(x, y).In(outside) {
				continue
			}
			a, b := withoutBloom.RGBAAt(x, y), bloom.RGBAAt(x, y)
			if int(b.R) > int(a.R)+8 {
				glow++
			}
		}
	}
	if glow < 10 {
		t.Fatal("real Bloom did not produce a visible halo beyond the painted emission", glow, brightBounds)
	}
	if model.History.Cursor() != 1 || !bytes.Equal(sceneio.Save(sceneDoc.Scene()), sceneBefore) {
		t.Fatal("preview controls changed document history or the scene")
	}
	if got, _ := shared.Get("EmissionIntensity"); got != materialBefore {
		t.Fatal("preview intensity changed the shared material")
	}
	previousPreview := doc.WorkingPreview.Applied
	key(input.KeyZ, true)
	if model.History.Cursor() != 0 {
		t.Fatal("common Undo did not restore emission pixels")
	}
	deadline = time.Now().Add(8 * time.Second)
	for (doc.WorkingPreview.Applied == previousPreview || doc.WorkingPreview.Pending) && doc.WorkingPreview.Error == "" && time.Now().Before(deadline) {
		step()
		time.Sleep(time.Millisecond)
	}
	undone, _, err := model.Capture(model.Session.Frame, art.Emission)
	if err != nil || bytes.Contains(undone.Pix, []byte{255, 255, 255, 255}) {
		t.Fatal("Undo retained emission", err)
	}
	capture("05-emission-undone")
	if got, err := os.ReadFile(filepath.Join(p.Root, "assets/surface.aeart")); err != nil || !bytes.Equal(got, saved) {
		t.Fatal("preview or Undo wrote unsaved source pixels", err)
	}
	t.Logf("actual SDL emission painting -> prepared Lit preview -> intensity -> Bloom (%d physical passes, %d halo pixels) -> common Undo; scene/shared material/source unchanged", bloomPasses, glow)
}
