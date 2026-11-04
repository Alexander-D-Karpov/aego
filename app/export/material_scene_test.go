package export

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/core/ids"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/modules/render2d"
	"aego/project"
	"aego/render/shader"
	"aego/render/shaderx"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/script/bind"
)

func TestExportRejectsMaterialDomainAndOverrideMismatchInScenesAndPrefabs(t *testing.T) {
	for _, tc := range []struct {
		name, domain, code string
		prefab, override   bool
	}{
		{"scene-domain", "mesh2d", "MAT006", false, false},
		{"scene-override", "sprite", "MAT002", false, true},
		{"unused-prefab-domain", "mesh2d", "MAT006", true, false},
		{"unused-prefab-override", "sprite", "MAT002", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, files, _ := preparedMaterialSnapshot(t, "", "")
			u, ds := shaderx.Parse([]byte("#pragma aego abi 1\n#pragma domain "+tc.domain+"\n#pragma material\nuniform float Value; // @instance @default(1)\n#pragma end\nvoid fragment(){aego_out=vec4(Value);}\n"), "fixture.aeshader", nil)
			if len(ds) != 0 {
				t.Fatal(ds)
			}
			a := asset.BuildShader(u)
			// This tests export validation dispatch, not a graphics-device link.
			a.Validation = shader.SchemaChecked | shader.ReferenceChecked
			files["shader.aesh"] = asset.EncodeShader(a)
			types, err := assembly.NewTypes()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := builtin.Runtime().Load(types, bind.New(), assembly.Options{}); err != nil {
				t.Fatal(err)
			}
			if err := types.Schema().Freeze(); err != nil {
				t.Fatal(err)
			}
			sc := scene.New(types, &resolver{s: s}, scene.Options{})
			n := sc.Create(sc.Root(), "Sprite")
			sr := scene.Add[render2d.SpriteRenderer](sc, n)
			*sr = render2d.DefaultSpriteRenderer()
			sr.Material.ID = ids.AssetID{Lo: 101}
			if tc.override {
				scene.Add[render2d.MaterialOverride](sc, n).Values = []render2d.MaterialParameter{{Name: "Value", Kind: shader.KindInt, Integer: 7}}
			}
			sc.Flush()
			if !hasExportError(module.ValidateScene(sc, nil), tc.code, "") {
				t.Fatal("fixture does not produce its registered component diagnostic")
			}
			mainID := ids.AssetID{Lo: 102}
			source := "scenes/main.aescene"
			files["main.aescene"] = sceneio.Save(sc)
			if tc.prefab {
				// Export includes this prefab, though the main scene never instantiates it.
				files["main.aescene"] = sceneio.Save(scene.New(types, &resolver{s: s}, scene.Options{}))
				files["unused.aeprefab"] = sceneio.SaveSubtreeWith(sc, n, nil)
				source = "prefabs/unused.aeprefab"
				s.idx.Put(asset.Entry{ID: ids.AssetID{Lo: 103}, Kind: asset.ArtPrefab, Source: source, Deps: []ids.AssetID{sr.Material.ID}, Artifacts: []asset.Artifact{{Kind: asset.ArtPrefab, Path: "cache://unused.aeprefab"}}})
			}
			s.idx.Put(asset.Entry{ID: mainID, Kind: asset.ArtScene, Source: "scenes/main.aescene", Artifacts: []asset.Artifact{{Kind: asset.ArtScene, Path: "cache://main.aescene"}}})
			diagnostics := Check(&project.Project{MainScene: mainID}, s, "release", false)
			found := false
			for _, d := range diagnostics {
				if d.Code == tc.code && d.Severity == problems.Error && d.Subject.File == source {
					found = true
				}
			}
			if !found {
				t.Fatalf("export omitted component diagnostic for %s: %+v", source, diagnostics)
			}
		})
	}
}
