//go:build aego_gltest

package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/project"
	"aego/render"
	"aego/render/lighting"
)

// The demonstration route is opt-in because it draws thousands of real frames; the regular suite still verifies the complete gameplay replay.
func TestPlatformerLightingGLRoute(t *testing.T) {
	dir := os.Getenv("AEGO_PLATFORMER_SHOTS")
	if dir == "" {
		t.Skip("set AEGO_PLATFORMER_SHOTS to capture the canonical native lighting route")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(lightingRouteFile)
	if err != nil {
		t.Fatal(err)
	}
	type capture struct {
		Frame        int          `json:"frame"`
		Tick         uint64       `json:"tick"`
		Lighting     bool         `json:"lighting"`
		File         string       `json:"file"`
		GameplayHash string       `json:"gameplayHash"`
		Stats        render.Stats `json:"render"`
	}
	report := struct {
		Project      string    `json:"project"`
		RouteSHA256  string    `json:"routeSHA256"`
		Renderer     string    `json:"renderer"`
		Seed         uint64    `json:"seed"`
		Captures     []capture `json:"captures"`
		InputFrames  int       `json:"inputFrames"`
		NativeWidth  int       `json:"width"`
		NativeHeight int       `json:"height"`
	}{Project: "examples/platformer", Seed: 42, InputFrames: 1840}
	hash := sha256.Sum256(data)
	report.RouteSHA256 = hex.EncodeToString(hash[:])
	frames := []int{400, 1039, 1216, 1840}
	before := map[int]capture{}
	beforePixels := map[int][]byte{}
	for _, enabled := range []bool{true, false} {
		name := "off"
		if enabled {
			name = "on"
		}
		t.Run(name, func(t *testing.T) {
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
			// Match the exported browser's bench/replay clock, including the
			// first fixed tick, instead of relying on the hidden-window delta.
			a.Engine.SetLockstep(true)
			report.Renderer = a.Engine.Caps().Renderer
			pf := &platformer{app: a, root: root, player: a.Scene.Find(a.Scene.Root(), "Player")}
			pf.pid, _ = a.Scripts.InstanceOf(pf.player, playerType)
			a.Runner.SetLightingView(enabled, lighting.DebugLighting)
			if _, err := actionsOf(t, pf).Replay(data); err != nil {
				t.Fatal(err)
			}
			driver := a.Driver()
			for frame := 1; frame <= report.InputFrames; frame++ {
				var shot *image.RGBA
				var captureErr error
				captureNow := slices.Contains(frames, frame)
				if captureNow {
					a.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, captureErr = img, err })
				}
				if !a.Engine.Step(driver) {
					t.Fatalf("route stopped at %d: %v\n%s", frame, a.Engine.Err(), logs.String())
				}
				if !captureNow {
					continue
				}
				if captureErr != nil || shot == nil {
					t.Fatalf("capture %d failed: %v", frame, captureErr)
				}
				report.NativeWidth, report.NativeHeight = shot.Rect.Dx(), shot.Rect.Dy()
				filename := fmt.Sprintf("%s-%04d.png", name, frame)
				f, err := os.Create(filepath.Join(dir, filename))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, shot)
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("save capture: %v %v", err, closeErr)
				}
				c := capture{Frame: frame, Tick: a.Engine.MainWorld().Time.Tick(), Lighting: enabled, File: filename, GameplayHash: fmt.Sprintf("%016x", gameplayDigest(pf)), Stats: a.RenderStats()}
				if c.Stats.MaterialCompileFailures != 0 || c.Stats.LightingFailures != 0 {
					t.Fatalf("GPU preparation failed: %+v\n%s", c.Stats, logs.String())
				}
				if enabled {
					before[frame], beforePixels[frame] = c, slices.Clone(shot.Pix)
				} else {
					if c.GameplayHash != before[frame].GameplayHash {
						t.Fatalf("lighting view changed gameplay at %d", frame)
					}
					if bytes.Equal(shot.Pix, beforePixels[frame]) {
						t.Fatalf("lighting on/off produced the same pixels at %d", frame)
					}
				}
				report.Captures = append(report.Captures, c)
				t.Logf("%s frame=%d player=%v drawCalls=%d lights=%d shadowMasks=%d outputBytes=%d", name, frame, a.Scene.WorldPosition(pf.player), c.Stats.DrawCalls, c.Stats.SelectedLights, c.Stats.DirtyShadowMasks, c.Stats.LightingTargetBytes)
			}
			pf.noFaults(t)
			if at := a.Scene.WorldPosition(pf.player); at.X < 66.45 {
				t.Fatalf("native GPU replay missed Finish: %v", at)
			}
		})
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "results.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
}
