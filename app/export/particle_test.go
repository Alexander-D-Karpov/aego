package export

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"aego/asset"
	matimp "aego/asset/importers/material"
	"aego/asset/importers/preset"
	"aego/asset/importers/scenefile"
	teximp "aego/asset/importers/texture"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/gpu/null"
	"aego/modules/particles"
	"aego/project"
	"aego/render/texture"
	"aego/runtime/bundle"
	"aego/runtime/player"
	"aego/vfs"
)

func TestParticleDataExportFrozenDesktopAndWeb(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Particles", false)
	if err != nil {
		t.Fatal(err)
	}
	write := func(path, src string) { writeMaterialExportSource(t, p.Root, path, src) }
	write("scenes/main.aescene", "scene version=1 {}")
	var names = []string{"Dust", "Spark", "Smoke", "Rain", "Explosion"}
	for _, n := range names {
		raw, err := particles.PresetSource(n)
		if err != nil {
			t.Fatal(err)
		}
		write("assets/effects/"+n+".aepreset", string(raw))
	}
	write("assets/effects/editor-only.aepreset", `prefab version=1 { node "22222222222222222222222222222222" { component "engine.Camera2D" version=1 {} } }`)
	for _, n := range []string{"frameA", "frameB", "mask"} {
		materialDependencyPNG(t, p, "assets/"+n+".png", color.NRGBA{R: 15, G: 25, B: 120, A: 180})
	}
	write("assets/sparks.aemat", fmt.Sprintf("material version=1 shader=asset(%q) { params {} }", asset.BuiltinShaderAdditive.String()))
	reg := pipeline.NewImporterRegistry()
	for _, imp := range []pipeline.Importer{scenefile.NewScene(), teximp.New(), matimp.New(), preset.New()} {
		if err := reg.Register(imp); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	update := func() {
		t.Helper()
		if _, _, err := pl.Scan(); err != nil {
			t.Fatal(err)
		}
		if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
			t.Fatalf("import: %v %+v", err, batch.Failed)
		}
	}
	update()
	id := func(rel string) ids.AssetID {
		t.Helper()
		v, ok := pl.Lookup(rel)
		if !ok {
			t.Fatalf("asset %s was not indexed", rel)
		}
		return v
	}
	p.MainScene = id("scenes/main.aescene")
	mask, frameA, frameB, mat := id("assets/mask.png"), id("assets/frameA.png"), id("assets/frameB.png"), id("assets/sparks.aemat")
	presetPath := "assets/effects/masked.aepreset"
	makePreset := func(mask, fb ids.AssetID) string {
		return fmt.Sprintf(`prefab version=1 {
 node "22222222222222222222222222222222" name="Masked" enabled=true layers=1 {
   component "particles.ParticleSystem2D" version=1 {
     shape=spriteMask mask=asset(%q) frames=[asset(%q),asset(%q)] frameFPS=9 material=asset(%q)
   }
 }
}`, mask.String(), frameA.String(), fb.String(), mat.String())
	}
	write(presetPath, makePreset(mask, frameB))
	update()
	wanted, ordinary := id(presetPath), id("assets/effects/editor-only.aepreset")
	snap := Take(pl)
	if ds := Check(p, snap, bundle.Release, false); Blocking(ds) != 0 {
		t.Fatalf("valid frozen input: %+v", ds)
	}
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if err = particles.New().Register(module.New(types, nil)); err != nil {
		t.Fatal(err)
	}
	if err = types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	for _, target := range []Target{{OS: "linux", Arch: "amd64"}, webTarget} {
		t.Run(target.String(), func(t *testing.T) {
			out := t.TempDir()
			j := job{p: p, snap: snap, t: target, profile: bundle.Release, rep: &Report{}, l: layoutFor(target, out, p)}
			if err := j.data(); err != nil {
				t.Fatal(err)
			}
			var game *bundle.Data
			var data *vfs.VFS
			if target.Web() {
				b := player.Bundle{Files: map[string][]byte{}}
				b.Manifest, err = os.ReadFile(filepath.Join(j.l.data, bundle.ManifestFile))
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range j.files {
					raw, err := os.ReadFile(filepath.Join(j.l.data, filepath.FromSlash(entry.Path)))
					if err != nil {
						t.Fatal(err)
					}
					b.Files[entry.Path] = raw
				}
				game, data, err = player.MountBytes(&b)
			} else {
				game, data, err = player.Mount(j.l.data)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := game.Index.Lookup(ordinary); ok {
				t.Fatal("editor-only camera preset in runtime manifest")
			}
			dev := null.New()
			defer dev.Close()
			l := loader.New(dev, texture.NewUploader(dev), log.Nop())
			defer l.Close()
			if err := l.Mount(game.Index, data); err != nil {
				t.Fatal(err)
			}
			for _, n := range names {
				pid := id("assets/effects/" + n + ".aepreset")
				cfg, err := particles.LoadPreset(types, l, pid)
				if err != nil {
					t.Fatalf("load %s: %v", n, err)
				}
				if err = cfg.Validate(); err != nil {
					t.Fatalf("%s: %v", n, err)
				}
			}
			entry, ok := game.Index.Lookup(wanted)
			if !ok {
				t.Fatal("missing compiled effect")
			}
			for _, d := range []ids.AssetID{mask, frameA, frameB, mat} {
				if !slices.Contains(entry.Deps, d) {
					t.Errorf("missing dependency %s", d)
				}
			}
			cfg, err := particles.LoadPreset(types, l, wanted)
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.Frames) != 2 || cfg.Frames[0].ID != frameA || cfg.Frames[1].ID != frameB || cfg.Mask.ID != mask || cfg.Material.ID != mat {
				t.Fatal("channel identities changed in export")
			}
			for i := range cfg.Frames {
				if _, ok := cfg.Frames[i].Region(l); !ok {
					t.Fatalf("frame %d unavailable in production mount", i)
				}
			}
			if _, ok := cfg.Material.Material(l); !ok {
				t.Fatal("cannot resolve the baked material")
			}
			if points, ok := l.ParticleMask(mask, 127); !ok || len(points) == 0 {
				t.Fatal("mask did not resolve from the exported pixels")
			}
		})
	}
	t.Run("changed mask source", func(t *testing.T) {
		name := "assets/mask.png"
		source := filepath.Join(p.Root, name)
		original, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.WriteFile(source, original, 0644); err != nil {
				t.Error(err)
			}
		})
		materialDependencyPNG(t, p, name, color.NRGBA{R: 1, A: 1})
		if ds := Check(p, snap, bundle.Release, false); !hasDiagnostic(ds, CodeDirtySource) {
			t.Fatalf("changed mask source could ship old coverage: %+v", ds)
		}
	})
	t.Run("disabled authoring node", func(t *testing.T) {
		raw, err := particles.PresetSource("Smoke")
		if err != nil {
			t.Fatal(err)
		}
		off := strings.Replace(string(raw), "enabled=true", "enabled=false", 1)
		c, err := particles.ReadPreset(types, nil, []byte(off))
		if err != nil || c.Rate != 3 {
			t.Fatalf("cannot read configuration of disabled authoring node: %v", err)
		}
	})
	t.Run("invalid preset structure", func(t *testing.T) {
		const bad = "assets/effects/two.aepreset"
		const emitter = `component "particles.ParticleSystem2D" version=1 {}`
		const camera = `component "engine.Camera2D" version=1 {}`
		node := func(id, body string) string { return fmt.Sprintf("node %q { %s }", id, body) }
		const one = "10000000000000000000000000000000"
		const two = "20000000000000000000000000000000"
		t.Cleanup(func() {
			if err := os.Remove(filepath.Join(p.Root, bad)); err != nil {
				t.Error(err)
			}
			update()
		})
		for _, tc := range []struct{ name, source string }{
			{"two roots", node(one, emitter) + "\n" + node(two, emitter)},
			{"wrong scalar type", node(one, `component "particles.ParticleSystem2D" version=1 {rate="fast"}`)},
			{"bad vector", node(one, `component "particles.ParticleSystem2D" version=1 {speed=["fast",10]}`)},
			{"bad gradient", node(one, `component "particles.ParticleSystem2D" version=1 {colorOverLife=[{time=0 color="red"}]}`)},
			{"integer overflow", node(one, `component "particles.ParticleSystem2D" version=1 {maxParticles=4294967360}`)},

			{"two components on one node", node(one, emitter+"\n"+emitter)},
			{"camera preceding emitter", node(one, camera+"\n"+emitter)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				data := "prefab version=1 {\n" + tc.source + "\n}"
				write(bad, data)
				update()
				if _, err := particles.ReadPreset(types, nil, []byte(data)); err == nil {
					t.Error("runtime accepted ambiguous preset")
				}
				ds := Check(p, Take(pl), bundle.Release, false)
				if !hasDiagnostic(ds, CodeScene) {
					t.Fatalf("strict export accepted ambiguous preset: %+v", ds)
				}
			})
		}
	})
	missing := ids.NewAssetID()
	for _, tc := range []struct {
		name     string
		mask, fb ids.AssetID
	}{{"frame", mask, missing}, {"mask", missing, frameB}} {
		write(presetPath, makePreset(tc.mask, tc.fb))
		if ds := Check(p, snap, bundle.Release, false); !hasDiagnostic(ds, CodeDirtySource) {
			t.Fatalf("%s: changed source could ship stale artifact: %+v", tc.name, ds)
		}
		update()
		ds := Check(p, Take(pl), bundle.Release, false)
		if Blocking(ds) == 0 || !hasDiagnostic(ds, "PRT002") || !hasDiagnostic(ds, CodeMissing) {
			t.Fatalf("%s: dangling asset did not block strict export: %+v", tc.name, ds)
		}
	}
}
func hasDiagnostic(ds []problems.Diagnostic, code string) bool {
	return slices.ContainsFunc(ds, func(d problems.Diagnostic) bool { return d.Code == code && d.Severity == problems.Error })
}
