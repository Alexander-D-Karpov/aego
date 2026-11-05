package integration

import (
	"os"
	"testing"

	"aego/asset"
	"aego/modules/render2d"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/scene"
)

func requirePlatformerPostFX(t *testing.T, fx *render2d.PostProcess) {
	t.Helper()
	if fx == nil || !fx.Enabled || len(fx.Effects) != 4 ||
		fx.Effects[0].TypeID != postfx.BloomID || !fx.Effects[0].Enabled ||
		fx.Effects[1].TypeID != postfx.VignetteID || fx.Effects[1].Enabled ||
		fx.Effects[2].TypeID != postfx.ColorGradeID || fx.Effects[2].Enabled || fx.Effects[2].LUT.ID.IsZero() ||
		fx.Effects[3].TypeID != postfx.CustomID || fx.Effects[3].Enabled || fx.Effects[3].Material.ID.IsZero() {
		t.Fatalf("expected Bloom ON and Vignette OFF on platformer Camera: %+v", fx)
	}
	if p := fx.Effects[0].Parameters; len(p) != 3 || p[0].Value.X != .75 || p[1].Value.X != .22 || p[2].Value.X != 6 {
		t.Fatalf("unexpected Bloom parameters: %+v", p)
	}
}

func TestPlatformerPostFXSampleAssetsImport(t *testing.T) {
	pf := openPlatformer(t)
	lut, ok := pf.app.Pipeline.Lookup("assets/postfx/identity16.png")
	if !ok {
		t.Fatal("sample LUT is missing from the project pipeline")
	}
	meta, ok := pf.app.Loader.TextureMetadata(lut)
	if !ok || meta.Region || meta.Semantic != asset.TextureData || meta.Width != 256 || meta.Height != 16 {
		t.Fatalf("sample LUT was not imported as a standalone linear data strip: %+v", meta)
	}
	id, ok := pf.app.Pipeline.Lookup("assets/postfx/cold.aemat")
	if !ok {
		t.Fatal("sample custom material is missing from the project pipeline")
	}
	m, ok := pf.app.Loader.Material(id)
	if !ok || m == nil || m.Shader() == nil || m.Shader().Domain != shader.DomainPostFX {
		t.Fatalf("sample material did not resolve to a postfx shader: %+v", m)
	}
}

func TestPlatformerPostFXRoute(t *testing.T) {
	data, err := os.ReadFile(lightingRouteFile)
	if err != nil {
		t.Fatal(err)
	}
	var baseline [lightingRouteFrames]uint64
	for _, enabled := range []bool{false, true} {
		pf := openPlatformer(t)
		sc := pf.app.Scene
		fx, ok := scene.Get[render2d.PostProcess](sc, sc.Find(sc.Root(), "Camera"))
		if !ok {
			t.Fatal("platformer camera missing its effects stack")
		}
		requirePlatformerPostFX(t, fx)
		fx.Enabled = enabled
		if _, err := actionsOf(t, pf).Replay(data); err != nil {
			t.Fatal(err)
		}
		var hits []checkpointReached
		stop := observePlayerCheckpoints(t, pf, &hits)
		for frame := range lightingRouteFrames {
			pf.step(1)
			digest := gameplayDigest(pf)
			if !enabled {
				baseline[frame] = digest
			} else if digest != baseline[frame] {
				t.Fatalf("PostFX changed gameplay at frame %d", frame+1)
			}
		}
		stop()
		pf.noFaults(t)
		finish := sc.ID(sc.Find(sc.Root(), "Finish"))
		if len(hits) == 0 || hits[len(hits)-1].point != finish || hits[len(hits)-1].tick != lightingFinishTick {
			t.Fatalf("effects=%v unexpected Finish: %v", enabled, hits)
		}
		t.Logf("PostFX=%v: %d authoritative states; Finish tick %d", enabled, lightingRouteFrames, hits[len(hits)-1].tick)
	}
}
