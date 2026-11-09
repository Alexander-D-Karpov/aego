package edit

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/editor/core/artedit"
	"aego/editor/core/command"
	"aego/editor/core/selection"
	"aego/editor/core/workspace"
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
	for _, unsupported := range []string{command.EditDuplicate, command.EditRename, command.FileSaveAs} {
		if ed.Commands.Enabled(unsupported) {
			t.Fatalf("unsupported %s would fall through to the selected scene", unsupported)
		}
		if err := ed.Commands.Run(unsupported); err == nil {
			t.Fatalf("unsupported %s was accepted", unsupported)
		}
	}
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
