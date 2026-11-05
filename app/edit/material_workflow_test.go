package edit

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/panels/scripteditor"
	"aego/modules/render2d"
	"aego/modules/tween"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
)

// The headless workflow exercises the editor's commands, dirty documents, History, importer and Play world;
// native/browser integration tests separately establish driver compilation and pixel output.
func TestMaterialAuthoringWorkflowHeadless(t *testing.T) {
	materialAuthoringWorkflow(t, headless())
}

func materialAuthoringWorkflow(t *testing.T, cfg Config) {
	t.Helper()
	p := newProject(t)
	const scenePath = "scenes/material-workflow.aescene"
	writeScene(t, p, scenePath, func(sc *scene.Scene) {
		cam := sc.Create(sc.Root(), "Camera")
		*scene.Add[render2d.Camera2D](sc, cam) = render2d.DefaultCamera2D()
		for _, name := range []string{"A", "B"} {
			h := sc.Create(sc.Root(), name)
			*scene.Add[render2d.SpriteRenderer](sc, h) = render2d.DefaultSpriteRenderer()
		}
	})
	s, err := Open(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	d, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	if cfg.Headless && ed.Renderer.Caps().Renderer != "null" {
		t.Fatal("this workflow must not report headless validation as a GPU test")
	}
	if !cfg.Headless && ed.Renderer.Caps().Renderer == "null" {
		t.Fatal("native workflow requires an actual graphics device")
	}

	// Invoke the same registry action as Create -> Material. Both sources must
	// land in the persistent Assets history, not an abandoned local undo stack.
	created := false
	for _, action := range ed.CreateActions {
		if action.Key == "render2d.material" {
			if err = action.Run(ids.NodeID{}); err != nil {
				t.Fatal(err)
			}
			created = true
			break
		}
	}
	if !created {
		t.Fatal("Material action is missing from the Create palette registry")
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("Create Material did not open its creation form")
	}
	const materialPath = "assets/materials/workflow.aemat"
	questions[0].Submit([]string{materialPath})
	materialID, ok := ed.Pipeline.Lookup(materialPath)
	if !ok {
		t.Fatal("palette action did not create a material")
	}
	md, err := ed.Docs.OpenScript(materialPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := asset.ParseMaterial(md.Code.Bytes(), materialPath)
	if err != nil {
		t.Fatal(err)
	}
	shaderID := parsed.Shader
	shaderPath, ok := ed.Pipeline.PathOf(shaderID)
	if !ok {
		t.Fatal("material has no created shader")
	}
	if s.App.Data().Inspected() != materialID {
		t.Fatal("Create Material did not select the created asset in the Inspector")
	}
	materialMeta, _ := os.ReadFile(filepath.Join(p.Root, materialPath+".meta"))
	shaderMeta, _ := os.ReadFile(filepath.Join(p.Root, shaderPath+".meta"))
	for _, command := range []string{"assets.undo", "assets.redo"} {
		c, found := ed.Commands.Get(command)
		if !found || c.Run == nil {
			t.Fatal("shared Assets history command is missing", command)
		}
		if err = c.Run(); err != nil {
			t.Fatal(command, err)
		}
		if command == "assets.undo" {
			for _, path := range []string{materialPath, shaderPath} {
				if _, e := os.Stat(filepath.Join(p.Root, path)); !os.IsNotExist(e) {
					t.Fatal("creation did not undo both sources", path, e)
				}
			}
		}
	}
	for path, original := range map[string][]byte{materialPath + ".meta": materialMeta, shaderPath + ".meta": shaderMeta} {
		data, e := os.ReadFile(filepath.Join(p.Root, path))
		if e != nil || !bytes.Equal(data, original) {
			t.Fatal("creation redo changed stable asset identity", path, e)
		}
	}
	md, err = ed.Docs.OpenScript(materialPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{shaderPath, materialPath} {
		ed.Open(context.At(source, 1, 0))
		s.Step()
		if !s.App.Workspace().Has(scripteditor.PanelID(source)) {
			t.Fatal("source action did not open the shared code editor", source)
		}
	}
	beforeMaterial := append([]byte(nil), md.Code.Bytes()...)
	for i, value := range []string{"0.2", "0.4", "0.6"} {
		_, err = document.ApplyMaterialMutation(md, ed.Loader.Shader, mutate.Tx{Label: "Material Opacity", Ops: []mutate.Op{mutate.MaterialSet("Opacity", value)}}, i != 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !md.Code.Undo() || !bytes.Equal(md.Code.Bytes(), beforeMaterial) || md.Code.CanUndo() {
		t.Fatal("one Inspector drag did not undo to the original shared source")
	}
	if !md.Code.Redo() {
		t.Fatal("material source redo")
	}
	if err = ed.Docs.SaveScript(md); err != nil {
		t.Fatal(err)
	}
	if entry, exists := ed.Pipeline.Entry(materialID); !exists || entry.Error != "" {
		t.Fatal("Save did not import the material with file watching disabled", entry)
	}
	cache := ed.Renderer.Materials()
	commit := func() { cache.Begin(); cache.End() }
	commit()
	live, ok := ed.Loader.Material(materialID)
	if !ok {
		t.Fatal("created material did not load through the runtime artifact")
	}
	if value, _ := live.Get("Opacity"); math.Abs(float64(value[0]-.6)) > 1e-6 {
		t.Fatal("saved Inspector value did not reach the shared material", value)
	}

	// Scene assignment and instance edits use the normal scene History.
	sc := d.Scene()
	a, b := sc.Find(sc.Root(), "A"), sc.Find(sc.Root(), "B")
	aID, bID := sc.ID(a), sc.ID(b)
	spriteType, _ := sc.Registry().ByName("engine.SpriteRenderer")
	materialField, _ := spriteType.Field("material")
	if err = d.Props.Set(nil, []ids.NodeID{aID, bID}, spriteType.ID, materialField.ID, schema.Asset(materialID)); err != nil {
		t.Fatal(err)
	}
	overrideType, _ := sc.Registry().ByName("engine.MaterialOverride")
	valuesField, _ := overrideType.Field("values")
	if err = d.History.Exec(commands.NewMutation("Add Material Override", commands.EnvFor(sc), commands.AddComp(sc, aID, overrideType, nil))); err != nil {
		t.Fatal(err)
	}
	tx := d.History.Begin("Material override Opacity")
	for _, value := range []float32{.2, .3, .4} {
		candidate := render2d.MaterialOverride{}
		if err = candidate.Set(live, "Opacity", schema.Float32(value)); err != nil {
			t.Fatal(err)
		}
		var values schema.Value
		valuesField.Get(unsafe.Pointer(&candidate), &values)
		if err = d.Props.Set(tx, []ids.NodeID{aID}, overrideType.ID, valuesField.ID, values); err != nil {
			t.Fatal(err)
		}
	}
	tx.Commit()
	get := func(s *scene.Scene, id ids.NodeID) float64 {
		t.Helper()
		v, e := render2d.GetMaterialParameter(s, s.Handle(id), "Opacity")
		if e != nil {
			t.Fatal(e)
		}
		return v.F
	}
	if err = d.History.Undo(); err != nil || math.Abs(get(sc, aID)-.6) > 1e-6 {
		t.Fatal("node override slider did not undo in one step", err)
	}
	if err = d.History.Redo(); err != nil || math.Abs(get(sc, aID)-.4) > 1e-6 || math.Abs(get(sc, bID)-.6) > 1e-6 {
		t.Fatal("node override was not isolated or redo failed", err)
	}
	d.History.MarkSaved()
	beforeScene := sceneio.Save(sc)
	pc := s.App.Play()
	if err = pc.Start(d); err != nil {
		t.Fatal(err)
	}
	service := tween.Of(pc.World())
	if service == nil {
		t.Fatal("real Play world did not attach the Tween module")
	}
	if _, err = service.ToFloat(pc.Scene().Handle(aID), "MaterialOverride.Opacity", .9, .5, tween.Options{}); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		pc.Tick(.05)
	}
	if get(pc.Scene(), aID) < .85 || math.Abs(get(pc.Scene(), bID)-.6) > 1e-6 {
		t.Fatal("attached Play-world Tween did not animate only its node")
	}
	pc.Stop()
	if !bytes.Equal(sceneio.Save(sc), beforeScene) || d.Dirty() {
		t.Fatal("Play animation changed or dirtied EditScene")
	}

	// Saving current source imports a candidate; publication is still atomic at
	// the renderer boundary. A newer invalid edit must cancel an older candidate.
	sd, err := ed.Docs.OpenScript(shaderPath)
	if err != nil {
		t.Fatal(err)
	}
	initialSource := string(sd.Code.Bytes())
	withGain := func(value string) string {
		return strings.Replace(initialSource, "#pragma end", "uniform float Gain; // @default("+value+")\n#pragma end", 1)
	}
	importShader := func(source string, valid bool) {
		t.Helper()
		sd.Code.ReplaceSnapshot([]byte(source), false)
		if e := ed.Docs.SaveScript(sd); e != nil {
			t.Fatal(e)
		}
		found := false
		for _, info := range ed.Pipeline.Assets() {
			if info.ID != shaderID {
				continue
			}
			found = true
			if (valid && info.Error != "") || (!valid && info.Error == "") {
				t.Fatal("source Save did not publish the expected import validation state", valid, info.Error)
			}
		}
		if !found {
			t.Fatal("shader disappeared after saving its source")
		}
	}
	old := live.Shader()
	importShader(withGain("0.1"), true)
	if live.Shader() != old || !cache.Pending(live) {
		t.Fatal("candidate schema was published before the frame boundary")
	}
	commit()
	if live.Shader() == old || cache.Pending(live) || math.Abs(get(sc, aID)-.4) > 1e-6 {
		t.Fatal("successful reload did not commit or lost node values")
	}
	lastGood, generation := live.Shader(), live.Generation()
	importShader(withGain("0.2"), true)
	if !cache.Pending(live) {
		t.Fatal("older valid source was not queued")
	}
	importShader("#include \"missing-workflow-include.glsl\"\n"+withGain("0.2"), false)
	commit()
	if cache.Pending(live) || live.Shader() != lastGood || live.Generation() != generation || math.Abs(get(sc, aID)-.4) > 1e-6 {
		t.Fatal("newer invalid source allowed a stale candidate or changed last-good state")
	}
	importShader(withGain("0.3"), true)
	commit()
	value, exists := live.Get("Gain")
	if !exists || math.Abs(float64(value[0]-.3)) > 1e-6 || live.Generation() == generation {
		t.Fatal("fixed source did not recover through the editor's import path")
	}
}
