package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/modules/particles"
)

func TestPlatformerEffectPresets(t *testing.T) {
	pf := openPlatformer(t)
	for _, name := range []string{"Dust", "Spark", "Smoke", "Rain", "Explosion"} {
		t.Run(name, func(t *testing.T) {
			rel := "assets/presets/" + name + ".aepreset"
			id, ok := pf.app.Pipeline.Lookup(rel)
			if !ok {
				t.Fatal("no preset asset", rel)
			}
			e, ok := pf.app.Pipeline.Entry(id)
			if !ok {
				t.Fatal("no preset entry")
			}
			art, ok := e.Artifact(asset.ArtPreset)
			if !ok {
				t.Fatal("missing imported preset")
			}
			compiled, err := pf.app.Pipeline.Artifacts().ReadFile(art.Path)
			if err != nil {
				t.Fatal(err)
			}
			c, err := particles.ReadPreset(pf.app.Scene.Types(), pf.app.Scene.Resolver(), compiled)
			if err != nil {
				t.Fatal(err)
			}
			if c.Sprite.IsZero() {
				t.Fatal("preset has no texture")
			}
			source, err := os.ReadFile(filepath.Join(pf.root, rel))
			if err != nil {
				t.Fatal(err)
			}
			builtin, err := particles.PresetSource(name)
			if err != nil || !bytes.Equal(source, builtin) {
				t.Fatal("example diverges from Create preset:", err)
			}
			if !c.Material.IsZero() {
				if _, ok := c.Material.Material(pf.app.Scene.Resolver()); !ok {
					t.Fatalf("missing material %s", c.Material.ID)
				}
			}
		})
	}
}
