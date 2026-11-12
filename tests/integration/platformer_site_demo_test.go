package integration

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"aego/core/mathx"
	"aego/modules/gameplay"
	"aego/platform/input"
)

// Exercise the site's actual isolated project rather than changing the user's
// platformer scene. The original first pit remains covered by the pit tests.
func TestWebsitePlatformerCheckpointFloor(t *testing.T) {
	source := copyExample(t, "platformer")
	root := filepath.Join(t.TempDir(), "website-project")
	python := "python3"
	if runtime.GOOS == "windows" {
		python = "python"
	}
	cmd := exec.Command(python, "-B", "../../tools/site_demo_prepare.py", source, root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare website project: %v\n%s", err, out)
	}
	t.Run("original-gap-negative-control", func(t *testing.T) {
		pf := platformerAt(t, source)
		pf.app.Scene.SetPosition(pf.player, mathx.Vec2{X: 16.5, Y: 2.9})
		pf.step(180)
		if n := gameplay.Of(pf.app.Engine.MainWorld()).Stats().Respawns; n == 0 {
			t.Fatal("original scene did not reproduce a fall through the gap")
		}
	})
	for _, tc := range []struct {
		name  string
		start float32
		key   input.Scancode
		end   func(float32) bool
	}{
		{"right", 14, input.KeyRight, func(x float32) bool { return x > 17.5 }},
		{"left", 18, input.KeyLeft, func(x float32) bool { return x < 14.5 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pf := platformerAt(t, root)
			s := pf.app.Scene
			for _, name := range []string{"EnemyLeft", "EnemyRight", "Enemy3", "FastEnemy4", "Enemy5", "FastEnemy5"} {
				if enemy := s.Find(s.Root(), name); !enemy.IsZero() {
					s.SetEnabled(enemy, false)
				}
			}
			s.SetPosition(pf.player, mathx.Vec2{X: tc.start, Y: 2.9})
			pf.step(40)
			pf.hold(tc.key, true)
			pf.step(60)
			pf.hold(tc.key, false)
			p := s.WorldPosition(pf.player)
			if !tc.end(p.X) || p.Y > 3.15 || !pf.motion().Grounded {
				t.Fatalf("could not walk across the former gap: position=%v grounded=%v", p, pf.motion().Grounded)
			}
			pf.noFaults(t)
		})
	}
}
