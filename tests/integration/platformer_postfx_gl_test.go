//go:build aego_gltest

package integration

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/core/mathx"
	"aego/modules/render2d"
	"aego/modules/settings"
	"aego/project"
	"aego/render"
	"aego/scene"
)

type outputCapture struct {
	world, menu, menuOn *image.RGBA
	stats               render.Stats
}

func capturePlatformerOutput(t *testing.T, mode string) outputCapture {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	root := copyExample(t, "platformer")
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	app, err := run.Open(p, run.Config{Hidden: true, FrameTime: 1.0 / 60, Seed: 42, Logger: log.New(log.NewTextSink(&logs)), NoProfile: true}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	app.Engine.SetLockstep(true)
	sc := app.Scene
	camera := sc.Find(sc.Root(), "Camera")
	fx, ok := scene.Get[render2d.PostProcess](sc, camera)
	if !ok {
		t.Fatal("expected PostFX on platformer Camera")
	}
	requirePlatformerPostFX(t, fx)
	switch mode {
	case "absent":
		scene.Remove[render2d.PostProcess](sc, camera)
	case "off", "on":
		// Preserve the original Vignette CPU-reference/UI regression separately
		// from the newly enabled default Bloom.
		fx.Effects[0].Enabled, fx.Effects[1].Enabled = false, true
		fx.Enabled = mode == "on"
	case "bloom":
		// Keep the authored camera settings unchanged.
	default:
		t.Fatalf("unknown capture mode %q", mode)
	}
	driver := app.Driver()
	for range 10 {
		if !app.Engine.Step(driver) {
			t.Fatal(app.Engine.Err(), logs.String())
		}
	}
	shot := func(label string) *image.RGBA {
		t.Helper()
		var img *image.RGBA
		var reason error
		app.Engine.CaptureFrame(func(c *image.RGBA, e error) { img, reason = c, e })
		if !app.Engine.Step(driver) || reason != nil || img == nil {
			t.Fatal(label, reason, logs.String())
		}
		if dir := os.Getenv("AEGO_POSTFX_SHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, mode+"-"+label+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, img)
			if e := f.Close(); e != nil || err != nil {
				t.Fatal(err, e)
			}
		}
		return img
	}
	out := outputCapture{world: shot("world"), stats: app.RenderStats()}
	v := settings.Of(app.Engine.MainWorld())
	if v == nil {
		t.Fatal("platformer has no settings overlay")
	}
	if mode == "off" {
		v.Open()
		out.menu = shot("menu")
		tick := app.Engine.MainWorld().Time.Tick()
		fx.Enabled = true
		out.menuOn = shot("menu-on")
		if !v.Opened() || app.Engine.MainWorld().Time.Tick() != tick {
			t.Fatal("settings comparison must use the same paused scene")
		}
	}
	if out.stats.MaterialCompileFailures != 0 || out.stats.LightingFailures != 0 {
		t.Fatal(logs.String(), out.stats)
	}
	t.Logf("%s: %s, %v, draws=%d resolves=%d", mode, app.Engine.Caps().Renderer, out.world.Rect, out.stats.DrawCalls, out.stats.ResolvePasses)
	return out
}

func TestPlatformerPostFXGL(t *testing.T) {
	var captures [4]outputCapture
	for i, mode := range []string{"absent", "off", "on", "bloom"} {
		captures[i] = capturePlatformerOutput(t, mode)
	}
	original, off, on := captures[0], captures[1], captures[2]
	bloom := captures[3]
	if original.stats.PostFXPasses != 0 || off.stats.PostFXPasses != 0 || off.stats.PostFXTargets != 0 || on.stats.PostFXPasses != 1 || on.stats.PostFXFailures != 0 {
		t.Fatalf("absent/OFF/ON physical FX passes=%d/%d/%d, scratch OFF/ON=%d/%d, failures=%d", original.stats.PostFXPasses, off.stats.PostFXPasses, on.stats.PostFXPasses, off.stats.PostFXTargets, on.stats.PostFXTargets, on.stats.PostFXFailures)
	}
	t.Logf("PostFX passes absent/OFF/ON=%d/%d/%d; scratch acquisitions OFF/ON=%d/%d", original.stats.PostFXPasses, off.stats.PostFXPasses, on.stats.PostFXPasses, off.stats.PostFXTargets, on.stats.PostFXTargets)
	if !bytes.Equal(original.world.Pix, off.world.Pix) || original.stats.DrawCalls != off.stats.DrawCalls || original.stats.ResolvePasses != off.stats.ResolvePasses {
		t.Fatal("disabled stack changed the old world or UI path")
	}
	if bloom.stats.PostFXPasses != 4 || bloom.stats.PostFXTargets != 2 || bloom.stats.PostFXFailures != 0 || bytes.Equal(bloom.world.Pix, off.world.Pix) {
		t.Fatalf("default Bloom did not render: passes=%d targets=%d failures=%d, image changed=%v", bloom.stats.PostFXPasses, bloom.stats.PostFXTargets, bloom.stats.PostFXFailures, !bytes.Equal(bloom.world.Pix, off.world.Pix))
	}
	if off.world.Rect != on.world.Rect || off.menu.Rect != off.menuOn.Rect {
		t.Fatal("output size changed")
	}
	b := on.world.Bounds()
	center := image.Pt(b.Dx()/2, b.Dy()/2)
	if on.world.RGBAAt(center.X, center.Y) != off.world.RGBAAt(center.X, center.Y) {
		t.Fatal("Vignette changed centre pixel")
	}
	darkened, maxError := 0, 0
	var linear [256]float32
	for i := range linear {
		linear[i] = mathx.SRGBToLinear(float32(i) / 255)
	}
	wf, hf := float64(b.Dx()), float64(b.Dy())
	for i := 0; i < len(off.world.Pix); i += 4 {
		old, now := off.world.Pix[i:i+4], on.world.Pix[i:i+4]
		if now[3] != old[3] {
			t.Fatalf("coverage changed at pixel %d", i/4)
		}
		x, y := float64((i/4)%b.Dx())+.5-wf/2, float64((i/4)/b.Dx())+.5-hf/2
		t01 := max(0., min(1., (2*math.Hypot(x, y)/min(wf, hf)-.35)/.65))
		gain := float32(1 - .6*t01*t01*(3-2*t01))
		// The reference starts from an already quantized RGBA8 bypass image.
		for channel := range 3 {
			want := int(math.Round(float64(255 * mathx.LinearToSRGB(linear[old[channel]]*gain))))
			delta := int(now[channel]) - want
			maxError = max(maxError, delta, -delta)
		}
		if !bytes.Equal(old, now) {
			darkened++
		}
	}
	if maxError > 1 {
		t.Fatalf("Vignette differs from CPU reference by %d RGBA8 levels", maxError)
	}
	t.Logf("max RGB error vs CPU Vignette on quantized OFF=%d/255; alpha exact", maxError)
	if darkened < b.Dx()*b.Dy()/10 {
		t.Fatalf("only %d world pixels darkened", darkened)
	}
	w, h := min(860, b.Dx()-32), min(620, b.Dy()-32)
	panel := image.Rect((b.Dx()-w)/2+16, (b.Dy()-h)/2+16, (b.Dx()+w)/2-16, (b.Dy()+h)/2-16)
	covered, visible := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			before, after := off.menu.RGBAAt(x, y), off.menuOn.RGBAAt(x, y)
			if image.Pt(x, y).In(panel) {
				if before != after {
					t.Fatalf("settings window changed at %d,%d: %v vs %v", x, y, before, after)
				}
				if off.world.RGBAAt(x, y) != on.world.RGBAAt(x, y) && off.world.RGBAAt(x, y) != before {
					covered++
				}
			} else if before != after {
				visible++
			}
		}
	}
	if covered < 100 || visible < 100 {
		t.Fatalf("missing UI/world evidence: %d / %d", covered, visible)
	}
	t.Logf("world RGBA: absent=OFF, ON changes %d/%d; settings interior unchanged over %d affected world pixels; world still visible through backdrop (%d pixels)", darkened, b.Dx()*b.Dy(), covered, visible)
}
