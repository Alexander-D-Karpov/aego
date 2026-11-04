//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/asset"
	"aego/engine/docs"
	"aego/modules/render2d"
	"aego/scene"
)

func TestLightingTryPreparedNormalMovesAndInspectorUndoRestarts(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	var entry docs.Entry
	for _, e := range p.TryScenes() {
		if e.ID == "render2d/normal-mapping" {
			entry = e
		}
	}
	if entry.Try == "" || entry.TryScript == "" || len(entry.TryArtifacts) == 0 {
		t.Fatal("normal Try has no scene/script/prepared normal assets")
	}
	code, err := p.TryCode(entry.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	request, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "artifacts": entry.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", request); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(2); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	node := sc.Find(sc.Root(), "Normal mapped dome")
	sprite, ok := scene.Get[render2d.SpriteRenderer](sc, node)
	if !ok {
		t.Fatal("normal receiver missing")
	}
	mat, ok := a.loader.Material(sprite.Material.ID)
	if !ok {
		t.Fatal("prepared material not resolved")
	}
	region, explicit, resolved := mat.Normal()
	if !explicit || !resolved || region.Texture.IsZero() {
		t.Fatal("Lit normal binding is missing")
	}
	docData, err := asset.DecodeTryArtifacts(entry.TryArtifacts)
	if err != nil {
		t.Fatal(err)
	}
	var normalPixels int
	for _, e := range docData.Assets {
		for _, art := range e.Artifacts {
			if art.Kind == asset.ArtTexture {
				v, err := asset.DecodeTexture(art.Data)
				if err != nil {
					t.Fatal(err)
				}
				if v.Semantic == asset.TextureNormal {
					for i := 0; i < len(v.Pixels); i += 4 {
						if v.Pixels[i] != 128 || v.Pixels[i+1] != 128 {
							normalPixels++
						}
					}
				}
			}
		}
	}
	if normalPixels < 1000 {
		t.Fatal("normal image is a flat placeholder")
	}
	flat := sc.Find(sc.Root(), "Flat normal comparison")
	flatSprite, _ := scene.Get[render2d.SpriteRenderer](sc, flat)
	flatMat, ok := a.loader.Material(flatSprite.Material.ID)
	if !ok {
		t.Fatal("flat Lit comparison material missing")
	}
	if _, explicit, _ := flatMat.Normal(); explicit {
		t.Fatal("normal leaked into shared flat material")
	}
	light := sc.Find(sc.Root(), "Moving light")
	before := sc.WorldPosition(light)
	if err := a.Step(30); err != nil {
		t.Fatal(err)
	}
	if sc.WorldPosition(light) == before {
		t.Fatal("normal Try light does not move")
	}
	if err := a.session.Set("engine.Light2D", "height", 1.25); err != nil {
		t.Fatal(err)
	}
	if err := a.Step(2); err != nil {
		t.Fatal(err)
	}
	readHeight := func() float32 {
		s := a.session.Controller().Scene()
		h := s.Find(s.Root(), "Moving light")
		c, _ := scene.Get[render2d.Light2D](s, h)
		return c.Height
	}
	if readHeight() != 1.25 {
		t.Fatal("Try Inspector did not apply light height")
	}
	if err := a.session.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := a.Step(2); err != nil {
		t.Fatal(err)
	}
	if readHeight() != 2.4 {
		t.Fatalf("undo lost original height: %g", readHeight())
	}
	if errors := errorsOf(a); len(errors) > 0 {
		t.Fatalf("normal Try faults: %v", errors)
	}
}
