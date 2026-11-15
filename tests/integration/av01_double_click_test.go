package integration

import (
	"bytes"
	"testing"

	"aego/asset/pipeline"
	"aego/editor/core/artedit"
	"aego/editor/core/document"
	"aego/editor/core/workspace"
	"aego/platform/headless"
)

// Each click hits a drawn tile via move/down/up on the real platform event
// queue and Session.Step, without directly invoking the PNG handler or an open
// command. The test uses the panel API only to choose fixture folders.
func TestAV01DoubleClickPNGInRealAssetBrowser(t *testing.T) {
	s, ed := openEditor(t)
	if _, ok := s.Host.Engine.Platform().(*headless.Platform); !ok {
		t.Fatal("event queue is not available")
	}
	const name = "av01/separate-surface.png"
	previousBeforeSwap := ed.BeforeSwap
	batches := 0
	ed.BeforeSwap = func(batch *pipeline.Batch) func() {
		batches++
		if len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Errorf("fixture import failed or requires retry: failed=%v retry=%v", batch.Failed, batch.Retry)
		}
		if previousBeforeSwap != nil {
			return previousBeforeSwap(batch)
		}
		return nil
	}
	id := writeSheet(t, s, name, 8, 8)
	ed.BeforeSwap = previousBeforeSwap
	if batches == 0 {
		t.Fatal("fixture did not publish an import batch")
	}
	imageBefore, err := ed.Pipeline.SourceBytes("assets/" + name)
	if err != nil {
		t.Fatal(err)
	}
	metaBefore, err := ed.Pipeline.SourceBytes("assets/" + name + ".meta")
	if err != nil {
		t.Fatal(err)
	}
	ap, ws := s.App.AssetsPanel(), s.App.Workspace()
	ws.Show(workspace.Assets)
	ap.SetFolder("assets/av01")
	ap.SetTileSize(48)
	for range 3 {
		s.Step()
	}
	tile, _, ok := ap.TileRect(id)
	if !ok || tile.W <= 0 || tile.H <= 0 {
		t.Fatal("real PNG tile is not drawn")
	}
	tab := "render2d.art/" + id.String()
	if ws.Visible(tab) {
		t.Fatal("image tab already open before input")
	}
	ws.Focus(workspace.Scene)
	if ws.Focused() != workspace.Scene {
		t.Fatal("initial focus precondition failed")
	}
	point := center(tile)
	clickTile(t, s, point, 0)
	if ap.Selected() != id || ws.Visible(tab) || ws.Focused() != workspace.Assets {
		t.Fatalf("single click: selected=%v visible=%v focus=%s", ap.Selected(), ws.Visible(tab), ws.Focused())
	}
	after, _, ok := ap.TileRect(id)
	if !ok || !after.Contains(point) {
		t.Fatal("pointer no longer hits PNG before click 2")
	}
	clickTile(t, s, point, 0)
	d := ed.Docs.AssetDocument(id)
	if d == nil || d.Handler != "render2d.art.png" || d.Path != "assets/"+name || d.Metadata == nil || !d.Capabilities.Has(document.EditSource) || d.Dirty() {
		t.Fatalf("two clicks did not open a clean editable PNG document: %+v", d)
	}
	model, ok := d.Binding.Model.(*artedit.Document)
	if !ok || !model.Draft {
		t.Fatal("the PNG is not an in-memory Art draft")
	}
	if got, err := ed.Pipeline.SourceBytes("assets/" + name); err != nil || !bytes.Equal(got, imageBefore) {
		t.Fatal("opening the PNG changed its source", err)
	}
	if got, err := ed.Pipeline.SourceBytes("assets/" + name + ".meta"); err != nil || !bytes.Equal(got, metaBefore) {
		t.Fatal("opening the PNG changed its metadata", err)
	}
	if len(ed.TakeQuestions()) != 0 {
		t.Fatal("opening the PNG asked for a conversion")
	}
	if !ws.Visible(tab) || ws.Focused() != tab {
		t.Fatalf("opened image not in a focused workspace tab: visible=%t focus=%s", ws.Visible(tab), ws.Focused())
	}
	s.Step()
	if ws.Focused() != tab {
		t.Fatal("image lost keyboard focus on the frame after release")
	}
	// First entry to Art restores its own default Assets folder rather than the
	// folder left in Design. Check the actual panel and folder before navigating
	// back; a stale Design tile rect must not be mistaken for an Art tile.
	assetsRect, assetsDrawn := ws.Tree.PanelRect(workspace.Assets)
	if ws.Current().ID != workspace.Art || !assetsDrawn || assetsRect.W <= 0 || assetsRect.H <= 0 || ap.Folder() != "assets" {
		t.Fatalf("unexpected Art Assets state: workspace=%s focus=%s rect=%+v drawn=%t folder=%s", ws.Current().ID, ws.Focused(), assetsRect, assetsDrawn, ap.Folder())
	}
	if _, _, ok := ap.TileRect(id); ok {
		t.Fatal("PNG from the Design subfolder was incorrectly drawn in the Art root folder")
	}
	// As in the initial setup, navigate to the fixture folder with the panel API;
	// the subsequent selection still uses a real mouse event on the newly drawn tile.
	ap.SetFolder("assets/av01")
	for range 2 {
		s.Step()
	}
	newTile, _, ok := ap.TileRect(id)
	if !ok {
		t.Fatalf("PNG tile is not drawn after navigating in Art: rect=%+v folder=%s selected=%v", assetsRect, ap.Folder(), ap.Selected())
	}
	if !assetsRect.Contains(center(newTile)) {
		t.Fatalf("Art PNG tile is not clickable inside the Assets panel: panel=%+v tile=%+v", assetsRect, newTile)
	}
	clickTile(t, s, center(newTile), 0)
	if ws.Focused() != workspace.Assets || !ws.Visible(tab) || ap.Selected() != id {
		t.Fatal("single click cannot focus Assets again after opening the image")
	}
	// The two workspaces remember their folders independently. Changing Design
	// must not hide the Art tile again or replace the already-open draft.
	if !ws.Switch(workspace.Design) || !s.Step() || ap.Folder() != "assets/av01" {
		t.Fatalf("Design did not restore its previous folder: %s", ap.Folder())
	}
	ap.SetFolder("assets")
	if !s.Step() || !ws.Switch(workspace.Art) || !s.Step() {
		t.Fatal("could not return to Art")
	}
	if ap.Folder() != "assets/av01" || ed.Docs.AssetDocument(id) != d || d.Binding.Model != model || !model.Draft || d.Dirty() {
		t.Fatal("returning to Art lost its folder or open draft")
	}
	if _, _, ok := ap.TileRect(id); !ok {
		t.Fatal("returning to Art did not redraw the PNG tile")
	}
	if got, err := ed.Pipeline.SourceBytes("assets/" + name); err != nil || !bytes.Equal(got, imageBefore) {
		t.Fatal("workspace switches changed the PNG source", err)
	}
	if got, err := ed.Pipeline.SourceBytes("assets/" + name + ".meta"); err != nil || !bytes.Equal(got, metaBefore) {
		t.Fatal("workspace switches changed the PNG metadata", err)
	}
	if len(ed.TakeQuestions()) != 0 {
		t.Fatal("workspace switches unexpectedly requested conversion")
	}
	t.Logf("two independent mouse-down/mouse-up clicks at (%g,%g) on real PNG tile: first selected, second opened/focused %s", point.X, point.Y, tab)
}
