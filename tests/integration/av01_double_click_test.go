package integration

import (
	"bytes"
	"testing"

	"aego/editor/core/artedit"
	"aego/editor/core/document"
	"aego/editor/core/workspace"
	"aego/platform/headless"
)

// Each click hits a drawn tile via move/down/up on the real platform event queue and Session.Step (clickTile/push), without directly invoking navigation, the PNG handler or an open command.
func TestAV01DoubleClickPNGInRealAssetBrowser(t *testing.T) {
	s, ed := openEditor(t)
	if _, ok := s.Host.Engine.Platform().(*headless.Platform); !ok {
		t.Fatal("event queue is not available")
	}
	const name = "av01/separate-surface.png"
	id := writeSheet(t, s, name, 8, 8)
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
	if model, ok := d.Binding.Model.(*artedit.Document); !ok || !model.Draft {
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
	// The first open may switch Design to Art, where Assets is in a different
	// dock. Find the current tile instead of clicking its former position.
	newTile, _, ok := ap.TileRect(id)
	if !ok {
		t.Fatal("PNG tile is not visible in the Art workspace")
	}
	clickTile(t, s, center(newTile), 0)
	if ws.Focused() != workspace.Assets || !ws.Visible(tab) {
		t.Fatal("single click cannot focus Assets again after opening the image")
	}
	t.Logf("two independent mouse-down/mouse-up clicks at (%g,%g) on real PNG tile: first selected, second opened/focused %s", point.X, point.Y, tab)
}
