//go:build aego_gltest

package edit

import (
	"aego/core/ids"
	"aego/editor/core/preview"
	"aego/gpu"
	"aego/render/sprite"
	"aego/render/view"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/workspace"
	"aego/modules/render2d"
	"aego/render/postfx"
	"aego/scene"
)

func TestPlatformerVignetteGameView(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := platformerAuthoringProject(t)
	s, err := Open(p, Config{Width: 1103, Height: 777, Hidden: true, NoWatch: true, Logger: log.Nop()}, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ctx := s.App.Context()
	doc, err := ctx.Docs.Open("scenes/main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	pc := s.App.Play()
	if err = pc.Start(doc); err != nil {
		t.Fatal(err)
	}
	s.App.Workspace().Show(workspace.Game)
	s.App.Workspace().Focus(workspace.Game)
	pc.Pause()
	read := func(name string) ([]byte, uint64) {
		t.Helper()
		for range 3 {
			if !s.Step() {
				t.Fatal("editor stopped")
			}
		}
		img, tick, doc, err := s.App.CaptureAgentView("game")
		if err != nil || doc != "scenes/main.aescene" {
			t.Fatal(doc, err)
		}
		if dir := os.Getenv("AEGO_POSTFX_SHOTS"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, "game-view-"+name+".png"), img, 0644); err != nil {
				t.Fatal(err)
			}
		}
		return img, tick
	}
	sc := pc.Scene()
	cam := sc.Find(sc.Root(), "Camera")
	fx, ok := scene.Get[render2d.PostProcess](sc, cam)
	if !ok || !fx.Enabled || len(fx.Effects) != 4 || fx.Effects[2].Enabled || fx.Effects[3].Enabled || fx.Effects[0].TypeID != postfx.BloomID || !fx.Effects[0].Enabled || fx.Effects[1].TypeID != postfx.VignetteID || fx.Effects[1].Enabled {
		t.Fatal("missing default Bloom / disabled Vignette camera stack")
	}
	fx.Enabled = false
	orig, tick := read("off")
	fx.Enabled = true
	active, next := read("bloom")
	fx.Enabled = false
	restored, last := read("restored")
	if tick != next || tick != last || !bytes.Equal(orig, restored) || bytes.Equal(active, orig) {
		t.Fatalf("Game View mismatch at ticks %d/%d/%d", tick, next, last)
	}
	fx.Enabled = true
	fx.Effects[0].Enabled, fx.Effects[1].Enabled = false, true
	vignette, vignetteTick := read("vignette")
	if vignetteTick != tick || bytes.Equal(vignette, orig) {
		t.Fatal("Game View Vignette regression: missing effect or changed simulation tick")
	}
	im, err := png.Decode(bytes.NewReader(active))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("actual editor Game View %v: active and bypass differ at tick %d, restoring bypass restores exact PNG", im.Bounds(), tick)
	if edited, _ := scene.Get[render2d.PostProcess](doc.Scene(), doc.Scene().Find(doc.Scene().Root(), "Camera")); edited == nil || !edited.Enabled || len(edited.Effects) != 4 || edited.Effects[2].Enabled || edited.Effects[3].Enabled || !edited.Effects[0].Enabled || edited.Effects[1].Enabled || doc.History.Dirty() {
		t.Fatal("Play changed the open document")
	}
}

func TestPostFXEditPreviewWithoutPlay(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := platformerAuthoringProject(t)
	s, err := Open(p, Config{Width: 1103, Height: 777, Hidden: true, NoWatch: true, Logger: log.Nop()}, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ctx := s.App.Context()
	doc, err := ctx.Docs.Open("scenes/main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	sc := doc.Scene()
	h := sc.Find(sc.Root(), "Camera")
	ctx.Select(sc.ID(h))
	s.App.Workspace().Show(workspace.Game)
	s.App.Workspace().Focus(workspace.Game)
	read := func() []byte {
		t.Helper()
		for range 3 {
			if !s.Step() {
				t.Fatal("editor stopped")
			}
		}
		data, tick, rel, err := s.App.CaptureAgentView("game")
		if err != nil || tick != 0 || rel != doc.Path {
			t.Fatalf("preview: %s %d %v", rel, tick, err)
		}
		return data
	}
	fx, _ := scene.Get[render2d.PostProcess](sc, h)
	active := read()
	fx.Enabled = false
	off := read()
	fx.Enabled = true
	if bytes.Equal(active, off) || !bytes.Equal(active, read()) || doc.History.Dirty() {
		t.Fatal("authored camera preview did not follow active/off/active in Edit mode")
	}
}

func TestCameraInspectorShowsRenderedPostStatus(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := platformerAuthoringProject(t)
	s, err := Open(p, Config{Width: 1500, Height: 1100, Hidden: true, NoWatch: true, Logger: log.Nop()}, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	doc, err := ed.Docs.Open("scenes/main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	sc := doc.Scene()
	cam := sc.Find(sc.Root(), "Camera")
	id := sc.ID(cam)
	fx, _ := scene.Get[render2d.PostProcess](sc, cam)
	instance := fx.Effects[0].InstanceID
	ed.Select(id)
	s.App.Workspace().Show(workspace.Game)
	s.App.Workspace().Show(workspace.Inspector)
	s.App.Inspector().ScrollTo(render2d.NamePostProcess)
	step := func() {
		for range 4 {
			if !s.Step() {
				t.Fatal("editor stopped")
			}
		}
	}
	check := func(want postfx.State, reason string) {
		t.Helper()
		step()
		w, out, preview := ed.GamePostOutput(ed.Doc())
		st, ok := render2d.ObservedPostEffect(w, out, id, instance, preview)
		if !ok || st.State != want || st.Reason != reason {
			t.Fatalf("actual compositor %+v %v; want %v %s", st, ok, want, reason)
		}
	}
	check(postfx.Active, "")
	items := s.App.UIList().TestDrawnItems(mathx.Rect{W: 1500, H: 1100}, ed.UI.Font(), "Game View plan", 0, 0)
	if len(items) == 0 {
		t.Fatal("selected Inspector does not show observed active status")
	}
	if dir := os.Getenv("AEGO_POSTFX_SHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) {
			if err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, "inspector-postfx-observation.png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err = png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
		})
		s.Step()
	}
	ed.Select(sc.ID(sc.Root()))
	step()
	if w, _, _ := ed.GamePostOutput(doc); w != nil {
		t.Fatal("unselected stopped camera retained recent output")
	}
	ed.Select(id)
	step()
	pc := s.App.Play()
	if err := pc.Start(doc); err != nil {
		t.Fatal(err)
	}
	check(postfx.Active, "")
	playing, _ := scene.Get[render2d.PostProcess](pc.Scene(), pc.Scene().Handle(id))
	playing.Enabled = false
	check(postfx.Disabled, "component disabled")
	if !fx.Enabled || doc.History.Dirty() {
		t.Fatal("reading play output edited authored component")
	}
	pc.Stop()
	ed.Select(id)
	check(postfx.Active, "")
	s.App.Workspace().Show(workspace.Scene)
	step()
	if w, _, _ := ed.GamePostOutput(doc); w != nil {
		t.Fatal("hidden Game View presented an obsolete sample")
	}
}

func TestPostFXAndParticlePresetThumbnails(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := platformerAuthoringProject(t)
	s, err := Open(p, Config{Width: 800, Height: 600, Hidden: true, NoWatch: true, Logger: log.Nop()}, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ctx := s.App.Context()
	content := ctx.Thumbs.(interface {
		Content(ids.AssetID) (image.Image, string, error)
	})
	presetDoc, err := ctx.OpenAssetPath("assets/presets/Arcade.aepreset")
	if err != nil || presetDoc.Handler != "render2d.postfx-preset" || presetDoc.Source == nil {
		t.Fatal("preset has no editable preview document", err)
	}
	if err := presetDoc.Source.ValidateSave(presetDoc.Source.Code.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := presetDoc.Source.ValidateSave([]byte("broken preset")); err == nil {
		t.Fatal("invalid preset saved")
	}
	for _, rel := range []string{"assets/postfx/cold.aemat", "assets/postfx/cold.aeshader", "assets/presets/Arcade.aepreset", "assets/presets/Dust.aepreset"} {
		func() {
			t.Log(rel)
			id, ok := ctx.Pipeline.Lookup(rel)
			if !ok {
				t.Fatal("missing imported asset")
			}
			deadline := time.Now().Add(12 * time.Second)
			for time.Now().Before(deadline) {
				img, _, err := content.Content(id)
				if err == nil {
					colors := map[color.NRGBA]bool{}
					for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
						for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
							c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
							colors[c] = true
						}
					}
					if len(colors) < 6 {
						t.Fatal("empty preview", len(colors))
					}
					return
				}
				if !errors.Is(err, preview.ErrPending) {
					t.Fatal(err)
				}
				if !s.Step() {
					t.Fatal("editor stopped")
				}
			}
			t.Fatal("thumbnail was never ready")
		}()
	}
}

// Exercise first thumbnail generation, shader/material selection and the document tab on a real GPU.
// A sprite fallback is also colourful, so compare Cold with the same shader at Strength=0 in linear colour.
func TestColdPostMaterialPreviewAndThumbnail(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := platformerAuthoringProject(t)
	var messages bytes.Buffer
	s, err := Open(p, Config{Width: 1103, Height: 777, Hidden: true, NoWatch: true, Logger: log.New(log.NewTextSink(&messages))}, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	const cold = "assets/postfx/cold.aemat"
	const shaderPath = "assets/postfx/cold.aeshader"
	coldID, _ := ed.Pipeline.Lookup(cold)
	shID, _ := ed.Pipeline.Lookup(shaderPath)
	refID, err := ed.CreateAsset("assets/postfx/control.aemat", []byte(fmt.Sprintf(`material version=1 shader=asset(%q) { params {Strength=0} }`, shID.String())))
	if err != nil {
		t.Fatal(err)
	}
	type contents interface {
		Content(ids.AssetID) (image.Image, string, error)
	}
	service := ed.Thumbs.(contents)
	get := func(id ids.AssetID) image.Image {
		t.Helper()
		end := time.Now().Add(20 * time.Second)
		for time.Now().Before(end) {
			im, _, err := service.Content(id)
			if err == nil {
				return im
			}
			if !errors.Is(err, preview.ErrPending) {
				t.Fatal(err)
			}
			if !s.Step() {
				t.Fatal("editor stopped")
			}
		}
		t.Fatal("thumbnail timed out")
		return nil
	}
	coldTile, refTile := get(coldID), get(refID)
	shTile := get(shID)
	changed := 0
	for y := refTile.Bounds().Min.Y; y < refTile.Bounds().Max.Y; y++ {
		for x := refTile.Bounds().Min.X; x < refTile.Bounds().Max.X; x++ {
			ref := color.NRGBAModel.Convert(refTile.At(x, y)).(color.NRGBA)
			out := color.NRGBAModel.Convert(coldTile.At(x, y)).(color.NRGBA)
			sh := color.NRGBAModel.Convert(shTile.At(x, y)).(color.NRGBA)
			if out != sh || out.A != ref.A || out.B != ref.B {
				t.Fatalf("material/shader/PMA at (%d,%d): %v %v reference %v", x, y, out, sh, ref)
			}
			converted := func(src uint8, gain float32) int {
				return int(math.Round(float64(255 * mathx.LinearToSRGB(gain*mathx.SRGBToLinear(float32(src)/255)))))
			}
			red, green := converted(ref.R, .904), converted(ref.G, .964)
			if abs := func(v int) int { return max(v, -v) }; abs(int(out.R)-red) > 2 || abs(int(out.G)-green) > 2 {
				t.Fatalf("wrong fullscreen Cold result at (%d,%d), got %v vs identity %v, predicted R/G %d/%d", x, y, out, ref, red, green)
			}
			if out != ref {
				changed++
			}
		}
	}
	if changed < refTile.Bounds().Dx()*refTile.Bounds().Dy()/2 {
		t.Fatalf("unfiltered or empty material tile: %d changed pixels", changed)
	}

	ed.InspectAsset(coldID)
	s.App.Workspace().Show(workspace.Inspector)
	s.App.Workspace().Focus(workspace.Inspector)
	checkOutput := func(stage string) {
		t.Helper()
		found := false
		for range 8 {
			ed.Renderer.InspectOutputPasses()
			s.Step()
			for _, pass := range ed.Renderer.InspectOutputPasses() {
				if pass.Label == postfx.CustomID && !pass.Window && pass.Width >= 192 && pass.Height >= 192 {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("%s did not render its own fullscreen preview", stage)
		}
	}
	checkOutput("Inspector")
	d, err := ed.OpenAssetPath(cold)
	if err != nil || d.Handler != "render2d.material" {
		t.Fatalf("Cold document: %v %v", d, err)
	}
	checkOutput("document")
	if strings.Contains(messages.String(), "MAT004") || strings.Contains(messages.String(), "SHD009") {
		t.Fatalf("valid PostFX usages raised a driver error: %s", messages.String())
	}
	// The negative control deliberately submits the material as a sprite. It
	// must report a MAT004 draw mismatch and keep the shader usable for PostFX.
	m, ok := ed.Loader.Material(coldID)
	if !ok {
		t.Fatal("Cold material unavailable")
	}
	done := false
	ed.OnRenderAfterViews(func() {
		target, err := ed.Renderer.Device().CreateTexture(gpu.TextureDesc{Width: 32, Height: 32, Format: gpu.TexRGBA8})
		if err != nil {
			t.Fatal(err)
		}
		defer ed.Renderer.Device().DestroyTexture(target)
		v := view.ScreenView(32, 32, 1)
		v.Target = target
		f := ed.Renderer.View(v)
		f.Sprite(sprite.Draw{Region: ed.Renderer.White(), Position: mathx.V2(16, 16), Size: mathx.V2(8, 8), Origin: mathx.V2(.5, .5), Tint: mathx.White, Material: m})
		f.End()
		done = true
	})
	s.Step()
	if !done || !strings.Contains(messages.String(), "MAT004") || strings.Contains(messages.String(), "SHD009") {
		t.Fatalf("geometry error must remain MAT004 without claiming a bad shader: %s", messages.String())
	}
}
