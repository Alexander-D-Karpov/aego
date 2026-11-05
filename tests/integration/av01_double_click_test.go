package integration

import (
	"aego/editor/core/document"
	"aego/editor/core/workspace"
	"aego/platform/headless"
	"testing"
)

// Each click hits a drawn tile via move/down/up on the real platform event queue and Session.Step (clickTile/push), without directly invoking navigation, the PNG handler or an open command.
func TestAV01DoubleClickPNGInRealAssetBrowser(t *testing.T) {
	s, ed := openEditor(t)
	if _, ok := s.Host.Engine.Platform().(*headless.Platform); !ok {
		t.Fatal("event queue is not available")
	}
	const name = "av01/separate-surface.png"
	id := writeSheet(t, s, name, 8, 8)
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
	tab := "render2d.image/" + id.String()
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
	if d == nil || d.Handler != "render2d.image" || d.Path != "assets/"+name || d.Metadata == nil || d.Capabilities.Has(document.EditSource) {
		t.Fatalf("two clicks did not open PNG as the image document: %+v", d)
	}
	if !ws.Visible(tab) || ws.Focused() != tab {
		t.Fatalf("opened image not in a focused workspace tab: visible=%t focus=%s", ws.Visible(tab), ws.Focused())
	}
	s.Step()
	if ws.Focused() != tab {
		t.Fatal("image lost keyboard focus on the frame after release")
	}
	clickTile(t, s, point, 0)
	if ws.Focused() != workspace.Assets || !ws.Visible(tab) {
		t.Fatal("single click cannot focus Assets again after opening the image")
	}
	t.Logf("two independent mouse-down/mouse-up clicks at (%g,%g) on real PNG tile: first selected, second opened/focused %s", point.X, point.Y, tab)
}
