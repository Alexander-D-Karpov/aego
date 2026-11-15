package edit

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset/pipeline"
	"aego/editor/core/artedit"
	"aego/editor/core/command"
	"aego/editor/core/selection"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/scene"
	"aego/scene/sceneio"
)

// The real shell routes Edit commands to the focused document. Opening and
// selecting fixtures are Go API calls; this is not a physical input test.
func TestArtEditDeleteDoesNotChangeSelectedSceneNode(t *testing.T) {
	p := newProject(t)
	drawing, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	data, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "assets", "edit-actions.aeart"), data, 0644); err != nil {
		t.Fatal(err)
	}
	const scenePath = "scenes/edit-actions.aescene"
	writeScene(t, p, scenePath, func(sc *scene.Scene) { sc.Create(sc.Root(), "Keep this node") })
	s, err := Open(p, headless())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	ed, ws := s.App.Context(), s.App.Workspace()
	sceneDoc, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	sc := sceneDoc.Scene()
	node := sc.Find(sc.Root(), "Keep this node")
	ed.Selection.Set(selection.Item{Doc: sceneDoc.ID, Node: sc.ID(node)})
	beforeScene := sceneio.Save(sc)
	beforeSceneDisk, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(scenePath)))
	if err != nil {
		t.Fatal(err)
	}
	assertScenePreserved := func(stage string) {
		t.Helper()
		disk, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(scenePath)))
		selected, hasSelection := ed.Selection.Active()
		if err != nil || !bytes.Equal(beforeSceneDisk, disk) || !bytes.Equal(beforeScene, sceneio.Save(sc)) ||
			sceneDoc.Dirty() || sceneDoc.Path != scenePath || !hasSelection || selected != (selection.Item{Doc: sceneDoc.ID, Node: sc.ID(node)}) {
			t.Fatalf("%s changed the selected scene or its file: %v", stage, err)
		}
	}
	id, ok := ed.Pipeline.Lookup("assets/edit-actions.aeart")
	if !ok {
		t.Fatal("drawing not imported")
	}
	doc, err := ed.OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("drawing not bound")
	}
	_, err = model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 4, Y: 4}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
	if err != nil {
		t.Fatal(err)
	}
	ws.Focus("render2d.art/" + id.String())
	if err := ed.Commands.Run(command.EditDelete); err != nil {
		t.Fatal(err)
	}
	if after := sceneio.Save(sc); !bytes.Equal(beforeScene, after) {
		t.Fatal("Edit Delete modified the selected scene while the art document was focused")
	}
	pixels, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || pixels.NRGBAAt(4, 4).A != 0 {
		t.Fatalf("Edit Delete did not clear the focused art pixel: %v", err)
	}
	if err := ed.UndoAsset(doc); err != nil {
		t.Fatal(err)
	}
	pixels, _, err = model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || pixels.NRGBAAt(4, 4).A != 255 {
		t.Fatalf("one Undo did not restore the art pixel: %v", err)
	}
	for _, unsupported := range []string{command.EditDuplicate, command.EditRename} {
		if ed.Commands.Enabled(unsupported) {
			t.Fatalf("unsupported %s would fall through to the selected scene", unsupported)
		}
		if err := ed.Commands.Run(unsupported); err == nil {
			t.Fatalf("unsupported %s was accepted", unsupported)
		}
	}
	// Save As is supported by Art, but must prompt for an independent drawing,
	// never for the selected scene behind it.
	if !ed.Commands.Enabled(command.FileSaveAs) {
		t.Fatal("Save As is unavailable for the focused Art document")
	}
	originalSource, err := ed.Pipeline.SourceBytes(doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	originalMeta, err := ed.Pipeline.SourceBytes(doc.Path + ".meta")
	if err != nil {
		t.Fatal(err)
	}
	originalCursor := model.History.Cursor()
	if err := ed.Commands.Run(command.FileSaveAs); err != nil {
		t.Fatal(err)
	}
	questions := ed.TakeQuestions()
	const copyPath = "assets/edit-actions-copy.aeart"
	if len(questions) != 1 || questions[0].Title != i18n.Tr("editor.shell.save_as", "Save As…") ||
		len(questions[0].Fields) != 1 || len(questions[0].Values) != 1 || questions[0].Values[0] != copyPath ||
		len(questions[0].Buttons) != 2 || questions[0].Submit == nil || questions[0].DestructiveLabel == "" || questions[0].DestructiveWhen == nil {
		t.Fatalf("Save As did not prompt for an Art source copy: %+v", questions)
	}
	if questions[0].DestructiveWhen([]string{copyPath}) || !questions[0].DestructiveWhen([]string{doc.Path}) {
		t.Fatal("Save As did not distinguish a new drawing from replacement")
	}
	assertScenePreserved("Art Save As prompt")
	if _, ok := ed.Pipeline.Lookup(copyPath); ok {
		t.Fatal("Art Save As created a copy before the path was submitted")
	}
	for _, rel := range []string{copyPath, copyPath + ".meta"} {
		if _, err := os.Stat(filepath.Join(p.Root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("Art Save As wrote %s before confirmation: %v", rel, err)
		}
	}
	previousBeforeSwap := ed.BeforeSwap
	batches := 0
	ed.BeforeSwap = func(batch *pipeline.Batch) func() {
		batches++
		if len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Errorf("Art Save As import failed or requires retry: failed=%v retry=%v", batch.Failed, batch.Retry)
		}
		if previousBeforeSwap != nil {
			return previousBeforeSwap(batch)
		}
		return nil
	}
	questions[0].Submit([]string{copyPath})
	ed.BeforeSwap = previousBeforeSwap
	if batches == 0 {
		t.Fatal("Art Save As did not publish an import batch")
	}
	copyID, ok := ed.Pipeline.Lookup(copyPath)
	if !ok || copyID == id {
		t.Fatal("Art Save As did not create an independently identified copy")
	}
	copyDoc := ed.Docs.AssetDocument(copyID)
	if copyDoc == nil || copyDoc == doc {
		t.Fatal("Art Save As did not open a separate document")
	}
	copyModel, ok := copyDoc.Binding.Model.(*artedit.Document)
	if !ok || copyModel == model {
		t.Fatal("Art Save As did not bind an independent drawing")
	}
	copyPixels, _, err := copyModel.Capture(copyModel.Session.Frame, art.Albedo)
	if err != nil || copyPixels.NRGBAAt(4, 4).R != 255 || copyPixels.NRGBAAt(4, 4).A != 255 {
		t.Fatalf("Art Save As lost the edited pixel: %v", err)
	}
	assertScenePreserved("Art Save As submission")
	if got, err := ed.Pipeline.SourceBytes(doc.Path); err != nil || !bytes.Equal(got, originalSource) {
		t.Fatal("Art Save As changed the original source", err)
	}
	if got, err := ed.Pipeline.SourceBytes(doc.Path + ".meta"); err != nil || !bytes.Equal(got, originalMeta) {
		t.Fatal("Art Save As changed the original metadata", err)
	}
	if !doc.Dirty() || model.History.Cursor() != originalCursor || doc.Binding.Model != model {
		t.Fatal("Art Save As changed the original dirty document or history")
	}
	ws.Focus("render2d.art/" + id.String())
	model.PendingRecovery = true
	for _, action := range []string{command.EditDelete, command.EditCut, command.EditPaste} {
		if err := ed.Commands.Run(action); err == nil {
			t.Fatalf("%s edited a drawing with pending recovery", action)
		}
	}
	model.PendingRecovery = false
	pixels, _, err = model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || pixels.NRGBAAt(4, 4).A != 255 {
		t.Fatal("rejected recovery edit modified the art")
	}
	cursor := model.History.Cursor()
	if err := ed.Commands.Run(command.EditSelectAll); err != nil || model.Session.Info().SelectedPixels != 16*16 || model.History.Cursor() != cursor {
		t.Fatalf("Select All did not select the art without changing history: %v", err)
	}
	if err := ed.Commands.Run(command.EditCopy); err != nil || model.History.Cursor() != cursor {
		t.Fatalf("Copy changed history or failed: %v", err)
	}
	if err := ed.Commands.Run(command.EditCut); err != nil || model.History.Cursor() != cursor+1 {
		t.Fatalf("Cut was not one art history operation: %v", err)
	}
	pixels, _, err = model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || pixels.NRGBAAt(4, 4).A != 0 {
		t.Fatal("Cut did not clear the art pixel")
	}
	if err := ed.Commands.Run(command.EditPaste); err != nil || model.History.Cursor() != cursor+2 {
		t.Fatalf("Paste was not one art history operation: %v", err)
	}
	pixels, _, err = model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || pixels.NRGBAAt(4, 4).A != 255 {
		t.Fatal("Paste did not restore the copied art pixel")
	}
	if after := sceneio.Save(sc); !bytes.Equal(beforeScene, after) {
		t.Fatal("Art editing changed the selected scene")
	}
	// The same command still acts on a scene when a scene panel has focus.
	ws.Focus(workspace.Scene)
	if err := ed.Commands.Run(command.EditDelete); err != nil {
		t.Fatalf("scene Delete was not available: %v", err)
	}
	if after := sceneio.Save(sc); bytes.Equal(beforeScene, after) {
		t.Fatal("scene Delete did not operate after focus returned to Scene")
	}
}
