//go:build aego_gltest

package integration

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/modules/particles"
	"aego/project"
	"aego/tests/particlefixture"
)

var updateParticleGoldens = flag.Bool("update-particle-goldens", false, "capture the five fixed-seed particle presets")

func TestParticlePresetGLGoldens(t *testing.T) {
	for _, name := range particlefixture.Names {
		t.Run(name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			root := t.TempDir()
			if err := particlefixture.Write(root); err != nil {
				t.Fatal(err)
			}
			p, err := project.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			a, err := run.Open(p, run.Config{Hidden: true, FrameTime: 1. / 60, Seed: 42, Logger: log.Nop(), NoProfile: true, ScenePath: filepath.Join(root, "scenes", name+".aescene")})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			a.Engine.SetLockstep(true)
			fx := particles.Of(a.Engine.MainWorld())
			var shot *image.RGBA
			var captureErr error
			d := a.Driver()
			for i := 1; i <= 60; i++ {
				if i == 60 {
					a.Engine.CaptureFrame(func(im *image.RGBA, e error) { shot, captureErr = im, e })
				}
				if !a.Engine.Step(d) {
					t.Fatal(a.Engine.Err())
				}
			}
			if shot == nil || captureErr != nil || fx.Stats().Submitted == 0 {
				t.Fatalf("frame 60: %v stats=%+v", captureErr, fx.Stats())
			}
			file := filepath.Join("testdata", "particle-goldens", name+".png")
			if *updateParticleGoldens {
				if err = os.MkdirAll(filepath.Dir(file), 0755); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(file)
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, shot)
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("save golden: %v %v", err, closeErr)
				}
			}
			f, err := os.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			want, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if want.Bounds() != shot.Bounds() {
				t.Fatalf("capture size %v vs golden %v", shot.Bounds(), want.Bounds())
			}
			bg := color.RGBAModel.Convert(want.At(0, 0)).(color.RGBA)
			active, bad := 0, 0
			for y := 0; y < shot.Rect.Dy(); y++ {
				for x := 0; x < shot.Rect.Dx(); x++ {
					got, ref := shot.RGBAAt(x, y), color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
					if got != bg || ref != bg {
						active++
					}
					for _, diff := range []int{int(got.R) - int(ref.R), int(got.G) - int(ref.G), int(got.B) - int(ref.B), int(got.A) - int(ref.A)} {
						if diff < -5 || diff > 5 {
							bad++
							break
						}
					}
				}
			}
			if active < 6 || bad > max(2, active/100) {
				t.Fatalf("changed=%d active=%d", bad, active)
			}
			t.Logf("%s renderer=%q frame=60 submitted=%d activePixels=%d mismatched=%d", name, a.Engine.Caps().Renderer, fx.Stats().Submitted, active, bad)
		})
	}
}
