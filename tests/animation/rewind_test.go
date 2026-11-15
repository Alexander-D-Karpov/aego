package animation_test

import (
	"testing"

	"aego/core/mathx"
	"aego/modules/render2d"
	"aego/scene"
)

// A script can change a property after animation writes it. A restored animation
// must make the same decision about its cached write as the original run.
func TestRewindPreservesLastAnimatedWrite(t *testing.T) {
	for _, animator := range []bool{false, true} {
		name := "player"
		if animator {
			name = "animator"
		}
		t.Run(name, func(t *testing.T) {
			r := ctrlRig(t)
			var h scene.NodeHandle
			if animator {
				h = r.animator(ctrlID)
			} else {
				h = r.node(idleID)
			}
			r.steps(1)
			sprite, _ := scene.Get[render2d.SpriteRenderer](r.s, h)
			sprite.Tint = mathx.Color{G: 1, A: 1}
			sceneKey := r.s.Capture(nil)
			animKey := r.svc.Capture(nil)

			r.steps(1)
			want := r.tint(h)
			if want != (mathx.Color{G: 1, A: 1}) {
				t.Fatalf("unexpected original tint: %+v", want)
			}
			r.s.Restore(sceneKey)
			r.svc.Restore(animKey)
			r.steps(1)
			if got := r.tint(h); got != want {
				t.Fatalf("replay tint %+v, original %+v", got, want)
			}
		})
	}
}
