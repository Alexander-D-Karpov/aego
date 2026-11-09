//go:build !aego_runtime

package docs

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"aego/asset"
	"aego/core/ids"
	"aego/engine/module"
	"aego/format/aetext"
)

func TestTrySourceIncludesEditableSceneScriptAndReferencedPrefab(t *testing.T) {
	ref := "10000000000000000000000000000001"
	e := Entry{ID: "engine.Test", Title: "Example", Try: `scene version=1 { node "00000000000000000000000000000001" { component "engine.Test" { resource=asset("` + ref + `") } } }`, TryScript: "package demo\n", TryPrefabs: map[string]string{ref: "prefab version=1 {}\n"}}
	r := &Registry{entries: []Entry{{ID: e.ID, Owner: "test"}}, byID: map[string]int{e.ID: 0}, mods: map[string]module.Info{"engine": {ID: "engine"}, "test": {ID: "test", Requires: []string{"engine"}}}}
	loc := trySourceLocation{src: module.DocSource{Owner: "test", FS: fstest.MapFS{}}, scene: "try/Test.aescene"}
	s, err := r.exportTrySource(e, loc)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{s.EntryScene, s.EntryScene + ".meta", "scripts/demo/main.aes", "scripts/demo/main.aes.meta", "prefabs/" + ref + ".aeprefab", "prefabs/" + ref + ".aeprefab.meta"} {
		if len(s.Files[name]) == 0 {
			t.Fatalf("source package omitted %s", name)
		}
	}
	if _, exists := s.Files["project.aego"]; exists {
		t.Fatal("engine registry synthesized a project instead of leaving packaging to its host")
	}
	meta, err := asset.ParseSourceMeta(s.Files[s.EntryScene+".meta"], s.EntryScene+".meta")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != trySourceID(loc, s.EntryScene) || len(s.Modules) != 2 || len(s.AssetRefs) != 2 {
		t.Fatalf("source project metadata/dependencies are inconsistent: %+v", s)
	}
	other, err := r.exportTrySource(e, loc)
	if err != nil || !bytes.Equal(s.Files[s.EntryScene+".meta"], other.Files[s.EntryScene+".meta"]) {
		t.Fatal("source IDs changed on repeated docs build", err)
	}
	delete(e.TryPrefabs, ref)
	if _, err := r.exportTrySource(e, loc); err == nil {
		t.Fatal("source package accepted a missing dependency")
	}
}

func TestTrySourceCopiesShaderBytesAndPreservesExistingIDs(t *testing.T) {
	id, _ := ids.ParseAssetID("20000000000000000000000000000001")
	shaderID, _ := ids.ParseAssetID("20000000000000000000000000000002")
	p := []byte(`project version=1 { name="Shader" module="game" mainScene=asset("` + id.String() + `") }`)
	shader := []byte("#pragma aego abi 1\n#pragma postfx\nvoid fragment() { aego_out = vec4(1.0); }\n")
	files := fstest.MapFS{
		"try/Test.assets/project.aego":                {Data: p},
		"try/Test.assets/scenes/source.aescene":       {Data: []byte("scene version=1 {}")},
		"try/Test.assets/scenes/source.aescene.meta":  {Data: trySourceMeta(id, "scene")},
		"try/Test.assets/assets/sample.aeshader":      {Data: shader},
		"try/Test.assets/assets/sample.aeshader.meta": {Data: trySourceMeta(shaderID, "shader")},
	}
	e := Entry{ID: "engine.Test", Try: `scene version=1 { node "00000000000000000000000000000001" name="Latest" {} }`}
	r := &Registry{}
	s, err := r.exportTrySource(e, trySourceLocation{src: module.DocSource{Owner: "test", FS: files}, scene: "try/Test.aescene"})
	if err != nil {
		t.Fatal(err)
	}
	if s.EntryScene != "scenes/source.aescene" || !bytes.Equal(s.Files[s.EntryScene], []byte(e.Try)) || !bytes.Equal(s.Files["assets/sample.aeshader"], shader) || !bytes.Equal(s.Files[s.EntryScene+".meta"], trySourceMeta(id, "scene")) {
		t.Fatal("source export changed stable identities or shader bytes")
	}
}

func TestTrySourceIsLazyAndDoesNotMutateRuntimeEntry(t *testing.T) {
	e := Entry{ID: "engine.Test", Try: "scene version=1 {}"}
	r := &Registry{entries: []Entry{e}, byID: map[string]int{e.ID: 0}}
	r.tryExtras(&r.entries[0], module.DocSource{Owner: "test", FS: fstest.MapFS{}}, "try/Test.aescene")
	if r.entries[0].TrySource != nil {
		t.Fatal("runtime registry eagerly retained source project bytes")
	}
	export := r.Export("test")
	if len(export.Entries) != 1 || export.Entries[0].TrySource == nil || r.entries[0].TrySource != nil {
		t.Fatal("build-time export changed runtime registry or omitted editable sources")
	}
}

func TestTrySourceChecksControllerDataAndImportSettingDependencies(t *testing.T) {
	entryID, _ := ids.ParseAssetID("30000000000000000000000000000001")
	sourceID, _ := ids.ParseAssetID("30000000000000000000000000000002")
	dependencyID, _ := ids.ParseAssetID("30000000000000000000000000000003")
	for _, ext := range []string{".aectrl", ".aedata", ".aetable", ".meta"} {
		t.Run(ext, func(t *testing.T) {
			source := "assets/source" + ext
			files := map[string][]byte{
				"scenes/main.aescene":      []byte("scene version=1 {}"),
				"scenes/main.aescene.meta": trySourceMeta(entryID, "scene"),
			}
			if ext == ".meta" {
				source = "assets/albedo.png"
				files[source] = []byte("source bytes are not rewritten by dependency validation")
				files[source+".meta"] = asset.MarshalSourceMeta(&asset.SourceMeta{ID: sourceID, Importer: "texture", Version: 1,
					Settings: asset.ImportSettings{"normal": aetext.AssetRef(dependencyID.String()),
						"nested": aetext.Struct(aetext.Field{Name: "channels", Value: aetext.List(aetext.AssetRef(dependencyID.String()))})}})
			} else {
				files[source] = []byte(`data version=1 { nested={refs=[asset("` + dependencyID.String() + `")]} }`)
				files[source+".meta"] = trySourceMeta(sourceID, "test")
			}
			s := &TrySource{EntryScene: "scenes/main.aescene", Files: files}
			r := &Registry{mods: map[string]module.Info{"engine": {ID: "engine"}}}
			if err := r.checkTrySource(s); err == nil || !strings.Contains(err.Error(), dependencyID.String()) {
				t.Fatalf("%s accepted missing dependency: %v", ext, err)
			}
			if len(s.AssetRefs) != 0 || len(s.Modules) != 0 {
				t.Fatal("failed check published partial dependencies")
			}
			files["assets/dependency.png"] = []byte("preserved dependency bytes")
			files["assets/dependency.png.meta"] = trySourceMeta(dependencyID, "texture")
			if err := r.checkTrySource(s); err != nil {
				t.Fatal(err)
			}
			wantRefs, wantModules := slices.Clone(s.AssetRefs), slices.Clone(s.Modules)
			if !slices.Contains(wantRefs, dependencyID.String()) {
				t.Fatal("dependency omitted from manifest refs")
			}
			if err := r.checkTrySource(s); err != nil || !slices.Equal(s.AssetRefs, wantRefs) || !slices.Equal(s.Modules, wantModules) {
				t.Fatalf("revalidation accumulated refs: %+v %v", s, err)
			}
		})
	}
}

func TestTrySourceRejectsMisplacedScriptWithoutRewritingSources(t *testing.T) {
	id, _ := ids.ParseAssetID("40000000000000000000000000000001")
	data := []byte("// package ignored\npackage demo\nscript Example {}\n")
	s := &TrySource{EntryScene: "scenes/main.aescene", Files: map[string][]byte{
		"scenes/main.aescene":      []byte("scene version=1 {}"),
		"scenes/main.aescene.meta": trySourceMeta(id, "scene"),
		"scripts/main.aes":         data,
	}}
	r := &Registry{}
	if err := r.checkTrySource(s); err == nil || !strings.Contains(err.Error(), "project directory") {
		t.Fatalf("misplaced package accepted: %v", err)
	}
	if !bytes.Equal(s.Files["scripts/main.aes"], data) {
		t.Fatal("validation rewrote the script")
	}
	delete(s.Files, "scripts/main.aes")
	s.Files["scripts/demo/main.aes"] = data
	if err := r.checkTrySource(s); err != nil {
		t.Fatal(err)
	}
	for _, malformed := range []string{"script Missing {}", "package demo\nscript Broken {"} {
		if _, err := trySourceScriptPackage("test.aes", []byte(malformed)); err == nil {
			t.Fatal("invalid source package accepted")
		}
	}
}
