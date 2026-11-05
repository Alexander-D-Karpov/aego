package integration

import (
	"testing"

	"aego/engine/res"
	"aego/gpu/null"
	"aego/render/icons"
	"aego/render/texture"
	"aego/vfs"
)

// The editor hides atlas load failures, so load the embedded atlas end to end.
func TestEmbeddedIconAtlasLoads(t *testing.T) {
	v, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	set, err := icons.Load(v, texture.NewUploader(null.New()), "engine://icons")
	if err != nil {
		t.Fatalf("embedded icon atlas failed to load: %v", err)
	}
	for _, id := range []string{
		"tool.select", "tool.move", "tool.rotate", "tool.scale",
		"view.grid", "view.snap", "view.zoom",
		"play.start", "play.pause", "play.step", "play.stop",
		"file.save", "cat.node", "cat.transform",
		"cat.render2d", "render2d.point_light", "render2d.spot_light", "render2d.directional_light", "render2d.sprite_light", "render2d.normals", "render2d.shadow_view",
		"animation.player", "animation.animator", "particles.system", "cat.scripts", "comp.script", "file.export",
		"ui.search", "ui.more", "build.ok", "build.busy", "build.error",
	} {
		if !set.Has(id) {
			t.Fatalf("icon %q missing from the embedded atlas", id)
		}
		if _, ok := set.Region(id, 16, 1); !ok {
			t.Fatalf("icon %q did not resolve a 16px region", id)
		}
		if _, ok := set.Region(id, 24, 2); !ok {
			t.Fatalf("icon %q did not resolve a 24px@2x region", id)
		}
	}
}

func TestLightingAndAnimationUseDistinctEmbeddedIcons(t *testing.T) {
	types, templates := probeTypes(t)
	wantComponents := map[string]string{
		"engine.Light2D":             "render2d.point_light",
		"animation.AnimationPlayer":  "animation.player",
		"animation.Animator":         "animation.animator",
		"particles.ParticleSystem2D": "particles.system",
	}
	for name, icon := range wantComponents {
		td, ok := types.Schema().ByName(name)
		if !ok || td.Icon != icon {
			t.Fatalf("%s icon: %+v, want %s", name, td, icon)
		}
	}
	wantTemplates := map[string]string{
		"render2d.point-light":       "render2d.point_light",
		"render2d.spot-light":        "render2d.spot_light",
		"render2d.directional-light": "render2d.directional_light",
		"render2d.sprite-light":      "render2d.sprite_light",
	}
	for _, tpl := range templates {
		if want, ok := wantTemplates[tpl.ID]; ok {
			if tpl.Icon != want {
				t.Fatalf("%s icon = %s, want %s", tpl.ID, tpl.Icon, want)
			}
			delete(wantTemplates, tpl.ID)
		}
	}
	if len(wantTemplates) > 0 {
		t.Fatalf("missing light templates: %v", wantTemplates)
	}
}
