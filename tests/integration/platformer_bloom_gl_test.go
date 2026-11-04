//go:build aego_gltest

package integration

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/core/mathx"
	"aego/modules/particles"
	"aego/modules/render2d"
	"aego/modules/settings"
	"aego/project"
	"aego/render/sprite"
	"aego/scene"
	"aego/scene/frame"
)

// Attribution uses an actual production frame and two leave-one-source-out
// controls. Only presentation is changed. The lantern's Light2D stays active.
func TestPlatformerBloomReceivesLanternAndEmbersBeforeMenuGL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	root := copyExample(t, "platformer")
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	a, err := run.Open(p, run.Config{Hidden: true, FrameTime: 1. / 60, Seed: 42, Logger: log.New(log.NewTextSink(&logs)), NoProfile: true}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Engine.SetLockstep(true)
	sc := a.Scene
	pf := &platformer{app: a, root: root, player: sc.Find(sc.Root(), "Player")}
	pf.pid, _ = a.Scripts.InstanceOf(pf.player, playerType)
	replay, err := os.ReadFile(lightingRouteFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := actionsOf(t, pf).Replay(replay); err != nil {
		t.Fatal(err)
	}
	driver := a.Driver()
	for range 75 {
		if !a.Engine.Step(driver) {
			t.Fatal(a.Engine.Err())
		}
	}
	w := a.Engine.MainWorld()
	w.Time.SetPaused(true)
	lantern := sc.Find(sc.Root(), "StartLantern")
	embers := sc.Find(lantern, "Embers")
	if n := particles.Of(w).Count(embers); n == 0 {
		t.Fatal("the canonical route has no active sparks here")
	}
	lamp, _ := scene.Get[render2d.SpriteRenderer](sc, lantern)
	reg, ok := lamp.Sprite.Region(sc.Resolver())
	if !ok {
		t.Fatal("missing lantern sprite")
	}
	center := sc.RenderWorld(lantern).Apply(mathx.Vec2{})
	fx, _ := scene.Get[render2d.PostProcess](sc, sc.Find(sc.Root(), "Camera"))
	requirePlatformerPostFX(t, fx)
	tick := w.Time.Tick()
	snap := func(label string, bloom, lampOn, embersOn bool) *image.RGBA {
		t.Helper()
		fx.Enabled = bloom
		a.Runner.SetHidden(func(h scene.NodeHandle) bool { return !embersOn && h == embers })
		removed := 0
		a.Runner.SetPreviewExtract(func(r *frame.Runner) {
			if lampOn {
				return
			}
			for i := range r.Views() {
				b := r.Bucket(i)
				b.Sprites = slices.DeleteFunc(b.Sprites, func(d sprite.Draw) bool {
					if d.Position == center && d.Region == reg {
						removed++
						return true
					}
					return false
				})
			}
		})
		var img *image.RGBA
		var captureErr error
		a.Engine.CaptureFrame(func(v *image.RGBA, e error) { img, captureErr = v, e })
		if !a.Engine.Step(driver) || captureErr != nil || img == nil || w.Time.Tick() != tick {
			t.Fatalf("%s capture: tick %d vs %d, %v", label, w.Time.Tick(), tick, captureErr)
		}
		if !lampOn && removed != 1 {
			t.Fatalf("%s removed %d lantern sprites", label, removed)
		}
		rs := a.RenderStats()
		if rs.PostFXFailures != 0 || rs.LightingFailures != 0 || rs.MaterialCompileFailures != 0 || (rs.PostFXPasses > 0) != bloom {
			t.Fatal(label, rs)
		}
		if dir := os.Getenv("AEGO_POSTFX_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, "attribution-"+label+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, img)
			cErr := f.Close()
			if err != nil || cErr != nil {
				t.Fatal(err, cErr)
			}
		}
		return img
	}
	raw, lit := snap("all-off", false, true, true), snap("all-on", true, true, true)
	for _, tc := range []struct {
		name         string
		lamp, embers bool
	}{{"lantern", false, true}, {"embers", true, false}} {
		withoutRaw := snap(tc.name+"-removed-off", false, tc.lamp, tc.embers)
		withoutLit := snap(tc.name+"-removed-on", true, tc.lamp, tc.embers)
		direct := image.Rectangle{}
		for y := raw.Rect.Min.Y; y < raw.Rect.Max.Y; y++ {
			for x := raw.Rect.Min.X; x < raw.Rect.Max.X; x++ {
				if raw.RGBAAt(x, y) != withoutRaw.RGBAAt(x, y) {
					direct = direct.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		if direct.Empty() {
			t.Fatalf("%s has no direct coverage", tc.name)
		}
		roi := direct.Inset(-16).Intersect(raw.Rect)
		var halo int
		var haloBounds image.Rectangle
		for y := roi.Min.Y; y < roi.Max.Y; y++ {
			for x := roi.Min.X; x < roi.Max.X; x++ {
				if raw.RGBAAt(x, y) != withoutRaw.RGBAAt(x, y) {
					continue
				}
				before, after := withoutLit.RGBAAt(x, y), lit.RGBAAt(x, y)
				if after.A == before.A && max(int(after.R)-int(before.R), int(after.G)-int(before.G), int(after.B)-int(before.B)) > 1 {
					halo++
					haloBounds = haloBounds.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		if halo == 0 {
			t.Fatalf("%s did not brighten any surrounding pixel (direct %v)", tc.name, direct)
		}
		t.Logf("tick=%d source=%s directly visible=%v halo=%d halo bounds=%v", tick, tc.name, direct, halo, haloBounds)
	}
	if repeat := snap("all-restored", true, true, true); !bytes.Equal(repeat.Pix, lit.Pix) {
		t.Fatal("leave-one-out comparisons did not use a reproducible frame")
	}
	settings.Of(w).Open()
	menuOff, menuOn := snap("menu-off", false, true, true), snap("menu-on", true, true, true)
	b := raw.Bounds()
	ww, hh := min(860, b.Dx()-32), min(620, b.Dy()-32)
	inside := image.Rect((b.Dx()-ww)/2+16, (b.Dy()-hh)/2+16, (b.Dx()+ww)/2-16, (b.Dy()+hh)/2-16)
	covered := 0
	for y := inside.Min.Y; y < inside.Max.Y; y++ {
		for x := inside.Min.X; x < inside.Max.X; x++ {
			if menuOff.RGBAAt(x, y) != menuOn.RGBAAt(x, y) {
				t.Fatalf("Bloom modified menu at (%d,%d)", x, y)
			}
			if raw.RGBAAt(x, y) != lit.RGBAAt(x, y) && raw.RGBAAt(x, y) != menuOff.RGBAAt(x, y) {
				covered++
			}
		}
	}
	if covered == 0 {
		t.Fatal("menu ROI contains no pixels hiding an actual Bloom response")
	}
	t.Logf("menu ROI %v unchanged with %d covered scene pixels affected by Bloom", inside, covered)
}
