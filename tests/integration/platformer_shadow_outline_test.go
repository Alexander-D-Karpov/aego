package integration

import (
	"fmt"
	"image/png"
	"os"
	"testing"

	"aego/asset/silhouette"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/modules/physics/components"
	"aego/modules/render2d"
	"aego/scene"
)

func TestPlatformerCharacterShadowOutlines(t *testing.T) {
	pf := openPlatformer(t)
	s := pf.app.Scene
	pf.step(1)
	resolver, ok := s.Resolver().(interface {
		SpriteShadow(ids.AssetID) (silhouette.Shape, bool)
	})
	if !ok {
		t.Fatal("platformer resolver cannot provide frame alpha shadows")
	}
	for _, name := range []string{"Player", "EnemyLeft", "EnemyRight", "Enemy3", "FastEnemy4", "Enemy5", "FastEnemy5"} {
		t.Run(name, func(t *testing.T) {
			h := s.Find(s.Root(), name)
			occluder, ok := scene.Get[render2d.LightOccluder2D](s, h)
			if !ok || occluder.Source != render2d.OccluderSpriteAlpha || occluder.Layers != 2 || occluder.Shape != "" {
				t.Fatalf("expected sprite-frame alpha shadow, got %+v", occluder)
			}
			sprite, ok := scene.Get[render2d.SpriteRenderer](s, h)
			if !ok || sprite.Sprite.ID.IsZero() {
				t.Fatalf("missing current sprite frame: %+v", sprite)
			}
			if shape, ok := resolver.SpriteShadow(sprite.Sprite.ID); !ok || shape.W <= 0 || shape.H <= 0 || len(shape.Parts) == 0 {
				t.Fatalf("current sprite frame did not resolve to alpha shadow geometry: %+v", shape)
			}
			if name != "Player" {
				box, ok := scene.Get[components.BoxCollider2D](s, h)
				if !ok || box.Size != mathx.V2(.9, .9) {
					t.Fatalf("enemy physics box changed: %+v", box)
				}
			}
		})
	}
	shape := s.Find(pf.player, "Shape")
	capsule, ok := scene.Get[components.CapsuleCollider2D](s, shape)
	if !ok || capsule.Length != 1 || capsule.Radius != .3 {
		t.Fatalf("player physics capsule changed: %+v", capsule)
	}
}

func TestPlatformerRunnerAlphaShadowEveryFrame(t *testing.T) {
	pf := openPlatformer(t)
	f, err := os.Open(pf.app.Pipeline.Abs("assets/surfaces/runner.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	image, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 128 || image.Bounds().Dy() != 16 {
		t.Fatalf("unexpected runner sheet size %v", image.Bounds())
	}
	unique := map[string]bool{}
	for frame := 0; frame < 8; frame++ {
		id, ok := pf.app.Pipeline.Lookup(fmt.Sprintf("assets/surfaces/runner.png#runner_%03d", frame))
		if !ok {
			t.Fatalf("runner frame %d missing from imported assets", frame)
		}
		shape, ok := pf.app.Loader.SpriteShadow(id)
		if !ok || shape.W != 16 || shape.H != 16 {
			t.Fatalf("runner frame %d has no prepared alpha shape: %+v", frame, shape)
		}
		unique[fmt.Sprint(shape.Parts)] = true
		for _, flip := range []bool{false, true} {
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					coverage := 0
					for _, r := range shape.Parts {
						left := r.X
						if flip {
							left = shape.W - r.X - r.W
						}
						if x >= left && x < left+r.W && y >= r.Y && y < r.Y+r.H {
							coverage++
						}
					}
					sourceX := x
					if flip {
						sourceX = 15 - x
					}
					_, _, _, alpha := image.At(frame*16+sourceX, y).RGBA()
					want := 0
					if alpha>>8 >= 128 {
						want = 1
					}
					if coverage != want {
						t.Fatalf("frame %d flip=%v pixel %d,%d: coverage=%d, want=%d", frame, flip, x, y, coverage, want)
					}
				}
			}
		}
	}
	if len(unique) < 2 {
		t.Fatal("all animation frames produced the same shadow shape")
	}
}
