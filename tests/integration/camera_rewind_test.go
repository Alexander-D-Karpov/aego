package integration

import (
	"testing"

	"aego/editor/core/play"
	"aego/gpu"
	"aego/modules/camera"
	"aego/render/rt"
)

// A zone blend that starts while a seek replays undrawn ticks must not blend from a zero or stale pose.
func TestCameraRewindKeepsZoom(t *testing.T) {
	for _, every := range []uint64{60, 1 << 30} {
		s, pc := rewindSession(t)
		pc.SetKeyEvery(every)
		e := s.Host.Engine
		pool := rt.New(e.GPU(), gpu.TexSRGBA8, "test.camera")
		tgt, err := pool.Resize(nil, 320, 180, true)
		if err != nil {
			t.Fatal(err)
		}
		render := func() float32 {
			frame := e.GPU().BeginFrame(320, 180)
			if err := s.Renderer.BeginFrame(frame, 320, 180, 1); err != nil {
				t.Fatal(err)
			}
			pc.Render(tgt)
			s.Renderer.EndFrame()
			e.GPU().EndFrame()
			return camera.Of(pc.World()).Render("").Zoom
		}
		zoom := map[int]float32{}
		lo, hi := float32(1e9), float32(0)
		drive(e, pc, 0, 800, func(f int) {
			z := render()
			zoom[f] = z
			lo, hi = min(lo, z), max(hi, z)
		})
		// Without keys, seeks jump past the scrub cache (TransientSpan) so each one replays from the start.
		step := 13
		if every > 1000 {
			step = play.TransientSpan + 13
		}
		for n := 790; n > 0; n -= step {
			if err := pc.Seek(n); err != nil {
				t.Fatal(err)
			}
			z := render()
			if z < lo-0.01 || z > hi+0.01 {
				t.Fatalf("keys every %d, seek %d: zoom %v outside the run's %v..%v", every, n, z, lo, hi)
			}
			// Restoring a keyframe finishes unscaled blends (D306); a replay from the start keeps them.
			if every > 1000 && (z < zoom[n]-0.01 || z > zoom[n]+0.01) {
				t.Fatalf("replayed seek %d: zoom %v, was %v", n, z, zoom[n])
			}
		}
		pool.Release(tgt)
		pool.Close()
	}
}
