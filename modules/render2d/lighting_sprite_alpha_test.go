package render2d

import (
	"slices"
	"testing"

	"aego/asset/silhouette"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/scene"
)

// Test actual lighting extraction, not only the bitmap-to-spans conversion.
// Distinct, asymmetric frames catch stale cached geometry and incorrect flips.
func TestSpriteAlphaShadowTracksFrameAndFlip(t *testing.T) {
	f := lightingTestWorld(t)
	f.camera("cam", ^uint64(0))
	h := f.sprite("animated", 1)
	oc := scene.Add[LightOccluder2D](f.s, h)
	*oc = DefaultLightOccluder2D()
	oc.Source = OccluderSpriteAlpha
	sprite, _ := scene.Get[SpriteRenderer](f.s, h)
	sprite.Origin, sprite.Size = mathx.V2(0, 0), mathx.V2(4, 4)
	f.res.shadowFrames = map[ids.AssetID]silhouette.Shape{
		{Lo: 1}: {W: 4, H: 4, Parts: []silhouette.Rect{{X: 0, Y: 0, W: 1, H: 2}, {X: 2, Y: 1, W: 2, H: 1}}},
		{Lo: 2}: {W: 4, H: 4, Parts: []silhouette.Rect{{X: 1, Y: 0, W: 2, H: 1}}},
	}
	f.res.regions[ids.AssetID{Lo: 2}] = f.res.regions[ids.AssetID{Lo: 1}]
	f.s.Flush()
	sprite, _ = scene.Get[SpriteRenderer](f.s, h)

	check := func(want []mathx.Vec2) uint64 {
		t.Helper()
		f.tick()
		in := f.r.Bucket(0).Lighting
		if !slices.Equal(in.Points, want) || len(in.Occluders)*4 != len(want) {
			t.Fatalf("frame=%s flipX=%v: got %v, want %v", sprite.Sprite.ID, sprite.FlipX, in.Points, want)
		}
		for _, o := range in.Occluders {
			if o.ID != f.s.ID(h) || !o.Closed || o.Count != 4 {
				t.Fatalf("invalid frame occluder: %+v", o)
			}
		}
		return in.Occluders[0].GeometryRevision
	}
	first := check([]mathx.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 2}, {X: 0, Y: 2}, {X: 2, Y: 1}, {X: 4, Y: 1}, {X: 4, Y: 2}, {X: 2, Y: 2}})
	if again := check([]mathx.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 2}, {X: 0, Y: 2}, {X: 2, Y: 1}, {X: 4, Y: 1}, {X: 4, Y: 2}, {X: 2, Y: 2}}); again != first {
		t.Fatal("unchanged alpha geometry invalidated")
	}
	sprite.Sprite.ID = ids.AssetID{Lo: 2}
	second := check([]mathx.Vec2{{X: 1, Y: 0}, {X: 3, Y: 0}, {X: 3, Y: 1}, {X: 1, Y: 1}})
	if second == first {
		t.Fatal("new animation frame reused stale shadow geometry")
	}
	sprite.Sprite.ID = ids.AssetID{Lo: 1}
	sprite.FlipX = true
	flipped := check([]mathx.Vec2{{X: 3, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 2}, {X: 3, Y: 2}, {X: 0, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 0, Y: 2}})
	if flipped == second {
		t.Fatal("FlipX reused stale shadow geometry")
	}
}

func TestAnimatedAlphaContoursReuseStorageWithoutKeepingOldIDs(t *testing.T) {
	f := lightingTestWorld(t)
	f.camera("cam", ^uint64(0))
	h := f.s.Create(f.s.Root(), "runner")
	sr := scene.Add[SpriteRenderer](f.s, h)
	*sr = DefaultSpriteRenderer()
	sr.Size = mathx.V2(4, 4)
	oc := scene.Add[LightOccluder2D](f.s, h)
	*oc = DefaultLightOccluder2D()
	oc.Source = OccluderSpriteAlpha
	a, b := ids.AssetID{Lo: 1}, ids.AssetID{Lo: 2}
	f.res.shadowFrames = map[ids.AssetID]silhouette.Shape{
		a: {W: 4, H: 4, Parts: []silhouette.Rect{{X: 0, Y: 0, W: 1, H: 1}}},
		b: {W: 4, H: 4, Parts: []silhouette.Rect{{X: 0, Y: 0, W: 2, H: 1}, {X: 2, Y: 1, W: 1, H: 2}}},
	}
	f.s.Flush()
	sr, _ = scene.Get[SpriteRenderer](f.s, h)
	state, _ := engine.ServiceOf[*lightingState](f.w, lightingServiceID)
	step := func() {
		if sr.Sprite.ID == b {
			sr.Sprite.ID = a
		} else {
			sr.Sprite.ID = b
		}
		f.tick()
	}
	for range 8 {
		step()
	}
	if got := testing.AllocsPerRun(600, step); got != 0 {
		t.Fatalf("alternating warmed shape still allocates %.2f objects/frame", got)
	}
	id := f.s.ID(h)
	sr.Sprite.ID = b
	f.tick()
	part := occluderKey{id, 2}
	first := state.cache[part].geometryRevision
	sr.Sprite.ID = a
	f.tick()
	if _, present := state.cache[part]; present {
		t.Fatal("vanished silhouette part kept its cache identity")
	}
	sr.Sprite.ID = b
	f.tick()
	if state.cache[part].geometryRevision <= first {
		t.Fatal("reappearing geometry kept a retired revision")
	}
}
