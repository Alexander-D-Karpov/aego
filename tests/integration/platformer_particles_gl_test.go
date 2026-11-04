//go:build aego_gltest

package integration

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/core/mathx"
	"aego/modules/particles"
	"aego/project"
)

func TestPlatformerLandingDustGL(t *testing.T) {
	dir := os.Getenv("AEGO_PARTICLE_SHOTS")
	if dir == "" {
		t.Skip("set AEGO_PARTICLE_SHOTS to capture paired landing frames")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(lightingRouteFile)
	if err != nil {
		t.Fatal(err)
	}
	frames := [3]int{75, 0, 0}
	var pixels [3]*image.RGBA
	var draws [3]uint32
	for _, enabled := range []bool{true, false} {
		mode := "off"
		if enabled {
			mode = "on"
		}
		t.Run(mode, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			root := copyExample(t, "platformer")
			p, err := project.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			a, err := run.Open(p, run.Config{Hidden: true, FrameTime: 1.0 / 60, Seed: 42, Logger: log.New(log.NewTextSink(&logs)), NoProfile: true}, combatModule{})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			a.Engine.SetLockstep(true)
			pf := &platformer{app: a, root: root, player: a.Scene.Find(a.Scene.Root(), "Player")}
			pf.pid, _ = a.Scripts.InstanceOf(pf.player, playerType)
			fx := particles.Of(a.Engine.MainWorld())
			fx.SetEnabled(enabled)
			dust := a.Scene.Find(pf.player, "Dust")
			lantern := a.Scene.Find(a.Scene.Root(), "StartLantern")
			embers := a.Scene.Find(lantern, "Embers")
			smoke := a.Scene.Find(lantern, "Smoke")
			if _, err := actionsOf(t, pf).Replay(data); err != nil {
				t.Fatal(err)
			}
			landed := false
			stop := observeLandings(t, pf, func() { landed = true })
			defer stop()
			driver := a.Driver()
			for frame := 1; frame <= 450; frame++ {
				landed = false
				shotID := -1
				var shot *image.RGBA
				var captureErr error
				for i, f := range frames {
					if frame == f {
						shotID = i
						a.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, captureErr = img, err })
					}
				}
				if !a.Engine.Step(driver) {
					t.Fatalf("frame %d: %v\n%s", frame, a.Engine.Err(), logs.String())
				}
				if enabled && frames[1] == 0 && frame > 30 && landed && mathx.Abs(pf.motion().Velocity.X) > 3 {
					frames[1], frames[2] = frame+1, frame+12
				}
				if shotID == -1 {
					continue
				}
				stats, render := fx.Stats(), a.RenderStats()
				if captureErr != nil || shot == nil || (stats.Submitted > 0) != enabled || render.MaterialCompileFailures != 0 || render.LightingFailures != 0 {
					t.Fatalf("frame %d capture=%v particles=%+v render=%+v\n%s", frame, captureErr, stats, render, logs.String())
				}
				if enabled && (fx.Count(embers) == 0 || fx.Count(smoke) == 0 || (shotID > 0) != (fx.Count(dust) > 0)) {
					t.Fatalf("frame %d: dust=%d embers=%d smoke=%d", frame, fx.Count(dust), fx.Count(embers), fx.Count(smoke))
				}
				file, err := os.Create(filepath.Join(dir, fmt.Sprintf("platformer-%s-%04d.png", mode, frame)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, shot)
				closeErr := file.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("save capture: %v %v", err, closeErr)
				}
				if enabled {
					pixels[shotID] = shot
					draws[shotID] = render.DrawCalls
				} else {
					ref := pixels[shotID]
					if ref == nil || ref.Rect != shot.Rect {
						t.Fatal("missing matching ON frame")
					}
					changed := 0
					bounds := image.Rectangle{}
					for y := 0; y < shot.Rect.Dy(); y++ {
						for x := 0; x < shot.Rect.Dx(); x++ {
							if ref.RGBAAt(x, y) != shot.RGBAAt(x, y) {
								changed++
								bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
							}
						}
					}
					if changed == 0 || changed >= shot.Rect.Dx()*shot.Rect.Dy()/20 {
						t.Fatalf("frame %d: expected a particle difference, got %d pixels in %v", frame, changed, bounds)
					}
					t.Logf("frame=%d changed=%d bounds=%v draws OFF/ON=%d/%d", frame, changed, bounds, render.DrawCalls, draws[shotID])
				}
				t.Logf("%s %s frame=%d player=%v live=%d submit=%d", a.Engine.Caps().Renderer, mode, frame, a.Scene.WorldPosition(pf.player), stats.Alive, stats.Submitted)
				if shotID == len(frames)-1 {
					pf.noFaults(t)
					return
				}
			}
			t.Fatal("did not capture the landing and departure")
		})
	}
}
