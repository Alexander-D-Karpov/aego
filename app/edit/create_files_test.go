package edit

import (
	"bytes"
	stdctx "context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/schema"
	"aego/sdk"
)

type createFileSettings struct {
	Value int `aego:"value"`
}

type createFileModule struct{}

func (createFileModule) Info() sdk.ModuleInfo { return sdk.ModuleInfo{ID: "createfiletest"} }
func (createFileModule) Register(r *sdk.Registry) error {
	return sdk.DataType[createFileSettings](r, schema.Name("createfiletest.Settings")).Err()
}

func TestCreateFileActionsProduceImportedSources(t *testing.T) {
	p := newProject(t)
	s, err := Open(p, headless(), createFileModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor did not start")
	}
	ed := s.App.Context()
	for _, d := range ed.Docs.Docs() {
		if err := ed.Docs.Close(d.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	if ed.Doc() != nil || !s.App.AddPalette().CanCreateMenu() {
		t.Fatal("Create must remain available for project files without an open scene")
	}
	panel := s.App.AssetsPanel()
	panel.SetFolder("assets")
	find := func(key string) context.CreateAction {
		t.Helper()
		for _, action := range panel.FileCreateActions() {
			if action.Key == key {
				return action
			}
		}
		t.Fatalf("missing file action %s", key)
		return context.CreateAction{}
	}
	create := func(key, name, rel string, kind asset.ArtifactKind) {
		t.Helper()
		if err := find(key).Run(ids.NodeID{}); err != nil {
			t.Fatal(err)
		}
		q := ed.TakeQuestions()
		if len(q) != 1 || q[0].Submit == nil {
			t.Fatalf("%s did not open a file creation prompt: %+v", key, q)
		}
		q[0].Submit([]string{name})
		id, ok := ed.Pipeline.Lookup(rel)
		if !ok {
			notice, _ := ed.LastNotice()
			t.Fatalf("%s did not create %s: %+v", key, rel, notice)
		}
		entry, ok := ed.Pipeline.Entry(id)
		if !ok || entry.Kind != kind {
			t.Fatalf("%s created an invalid or unimported source: %+v", key, entry)
		}
		if _, err := os.Stat(filepath.Join(p.Root, filepath.FromSlash(rel))); err != nil {
			t.Fatal(err)
		}
	}
	create("file.animation", "walk", "assets/walk.aeanim", asset.ArtAnim)
	create("file.controller", "controller", "assets/controller.aectrl", asset.ArtCtrl)
	create("file.data.createfiletest.Settings", "config", "assets/config.aedata", asset.ArtData)
	create("file.table.createfiletest.Settings", "configs", "assets/configs.aetable", asset.ArtTable)
	create("file.scene", "room", "assets/room.aescene", asset.ArtScene)
	create("file.prefab", "object", "assets/object.aeprefab", asset.ArtPrefab)
	if err := find("file.script").Run(ids.NodeID{}); err != nil {
		t.Fatal(err)
	}
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].Submit == nil {
		t.Fatal("script action did not ask for package and name")
	}
	q[0].Submit([]string{"game", "NewScript"})
	if id, ok := ed.Pipeline.Lookup("scripts/game/new_script.aes"); !ok || id.IsZero() {
		t.Fatal("script action did not create a project script")
	}
	findModule := func(key string) context.CreateAction {
		t.Helper()
		for _, action := range ed.CreateActions {
			if action.Key == key {
				return action
			}
		}
		t.Fatalf("missing module file action %s", key)
		return context.CreateAction{}
	}
	for _, tc := range []struct {
		key, rel string
		values   []string
		kind     asset.ArtifactKind
	}{
		{"render2d.png", "assets/blank.png", []string{"assets/blank.png", "21", "13"}, asset.ArtTexture},
		{"render2d.shader-include", "assets/common.glsl", []string{"assets/common.glsl"}, asset.ArtShaderInclude},
		{"particles.create-preset", "assets/dust.aepreset", []string{"assets/dust.aepreset", "Dust"}, asset.ArtPreset},
	} {
		action := findModule(tc.key)
		if action.RunInFolder == nil {
			t.Fatalf("%s is missing from the Assets folder menu", tc.key)
		}
		if err := action.RunInFolder("assets"); err != nil {
			t.Fatal(err)
		}
		questions := ed.TakeQuestions()
		if len(questions) != 1 || questions[0].Submit == nil {
			t.Fatalf("%s did not open a creation prompt", tc.key)
		}
		questions[0].Submit(tc.values)
		id, ok := ed.Pipeline.Lookup(tc.rel)
		if !ok {
			notice, _ := ed.LastNotice()
			t.Fatalf("%s was not created: %+v", tc.key, notice)
		}
		entry, ok := ed.Pipeline.Entry(id)
		if !ok || entry.Kind != tc.kind {
			t.Fatalf("%s was not imported: %+v", tc.key, entry)
		}
		if tc.key == "render2d.png" {
			f, err := os.Open(filepath.Join(p.Root, filepath.FromSlash(tc.rel)))
			if err != nil {
				t.Fatal(err)
			}
			cfg, decodeErr := png.DecodeConfig(f)
			f.Close()
			if decodeErr != nil || cfg.Width != 21 || cfg.Height != 13 {
				t.Fatalf("created PNG is invalid: %+v, %v", cfg, decodeErr)
			}
			doc := ed.Docs.AssetDocument(id)
			if doc == nil || !doc.Capabilities.Has(document.EditSource) || doc.Dirty() {
				t.Fatal("new PNG did not open as a clean editable document")
			}
			model, ok := doc.Binding.Model.(*artedit.Document)
			if !ok || !model.Draft {
				t.Fatal("new PNG did not open as an in-memory Art draft")
			}
			original, err := ed.Pipeline.SourceBytes(tc.rel)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}); err != nil || !doc.Dirty() {
				t.Fatal("the new image could not be painted in memory", err)
			}
			if current, err := ed.Pipeline.SourceBytes(tc.rel); err != nil || !bytes.Equal(current, original) {
				t.Fatal("painting changed the original PNG before Save", err)
			}
			if err := ed.SaveAsset(doc); !errors.Is(err, document.ErrSavePending) {
				t.Fatal("the first Save did not require an explicit destination", err)
			}
			questions := ed.TakeQuestions()
			if len(questions) != 1 || questions[0].Submit == nil {
				t.Fatal("the first Save did not offer a linked source path")
			}
			questions[0].Submit([]string{"assets/blank.aeart"})
			artID, ok := ed.Pipeline.Lookup("assets/blank.aeart")
			if !ok || ed.Docs.AssetDocument(artID) == nil || ed.Docs.AssetDocument(artID).Binding.Model != model || model.Draft || model.Asset.Dirty() {
				t.Fatal("confirmed Save did not transfer the drawing to the new source")
			}
			meta, ok := ed.Pipeline.MetaOf(id)
			link, err := artsource.TextureLinkOf(&meta)
			if !ok || err != nil || link == nil || link.Source != artID {
				t.Fatal("confirmed Save did not preserve the PNG's linked identity", err)
			}
			if current, err := ed.Pipeline.SourceBytes(tc.rel); err != nil || !bytes.Equal(current, original) {
				t.Fatal("confirmed Save overwrote the original PNG", err)
			}
		}
	}
	panel.SetFolder("assets")
	if err := find("file.import").Run(ids.NodeID{}); err != nil {
		t.Fatal(err)
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("headless import must offer a path when no native picker is available")
	}
	source, err := filepath.Abs(filepath.Join("..", "..", "art", "testdata", "aseprite", "upstream", "tags3.aseprite"))
	if err != nil {
		t.Fatal(err)
	}
	questions[0].Submit([]string{source})
	if id, ok := ed.Pipeline.Lookup("assets/tags3.aeart"); !ok || id.IsZero() {
		notice, _ := ed.LastNotice()
		t.Fatalf("file picker fallback did not import Aseprite: %+v", notice)
	}
}
