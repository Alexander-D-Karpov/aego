//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"

	"aego/core/log"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/workspace"
	"aego/project"
	"aego/ui/dock"
)

// Reproduces the reported layout: Inspector is a tab beside Assets, not a
// separate dock. Both clicks use the same SDL coordinates and the source is a
// generated fixture, never a user's project. This is synthetic UI coverage.
func TestArtPNGDoubleClickWithInspectorInAssetsDock(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p, err := project.Create(t.TempDir(), "PNG double click", false)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	interopStep(t, s)
	ed := s.App.Context()
	pixels := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	pixels.SetNRGBA(3, 4, color.NRGBA{R: 245, G: 31, B: 178, A: 255})
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, pixels); err != nil {
		t.Fatal(err)
	}
	id, err := assetops.CreateFile(ed, "assets/double-click.png", pngData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	metaBefore, err := s.Pipeline.SourceBytes("assets/double-click.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	ws := s.App.Workspace()
	if !ws.Switch(workspace.Art) {
		t.Fatal("Art workspace is unavailable")
	}
	ws.Show(workspace.Assets)
	ws.Show(workspace.Inspector)
	if ws.Tree.FindPanel(workspace.Assets) != ws.Tree.FindPanel(workspace.Inspector) {
		ws.Tree.DockPanel(workspace.Inspector, ws.Tree.FindPanel(workspace.Assets), dock.SideCenter)
	}
	ws.Show(workspace.Assets)
	s.App.AssetsPanel().SetFolder("assets")
	for range 3 {
		interopStep(t, s)
	}
	tile, _, visible := s.App.AssetsPanel().TileRect(id)
	if !visible {
		t.Fatal("fixture PNG is not visible in Assets")
	}
	interopClick(t, s, tile.Center().X, tile.Center().Y)
	if _, visible := ws.Tree.PanelRect(workspace.Assets); !visible {
		t.Fatal("first click hid Assets behind Inspector")
	}
	current, _, visible := s.App.AssetsPanel().TileRect(id)
	if !visible || current != tile {
		t.Fatal("the image tile moved between the two clicks")
	}
	interopClick(t, s, tile.Center().X, tile.Center().Y)
	interopStep(t, s)
	doc := ed.Docs.AssetDocument(id)
	if doc == nil {
		t.Fatal("double-click did not open the PNG document")
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || !model.Draft || doc.Dirty() {
		t.Fatal("double-click did not open a clean, editable in-memory drawing")
	}
	if _, visible := ws.Tree.PanelRect("render2d.art/" + id.String()); !visible {
		t.Fatal("double-click did not focus the Art canvas")
	}
	if _, visible := ws.Tree.PanelRect(workspace.Assets); !visible {
		t.Fatal("opening the canvas hid Assets on release")
	}
	if len(ed.TakeQuestions()) != 0 {
		t.Fatal("opening the image unexpectedly requested conversion")
	}
	if got, err := s.Pipeline.SourceBytes("assets/double-click.png"); err != nil || !bytes.Equal(got, pngData.Bytes()) {
		t.Fatal("opening changed the original PNG", err)
	}
	if got, err := s.Pipeline.SourceBytes("assets/double-click.png.meta"); err != nil || !bytes.Equal(got, metaBefore) {
		t.Fatal("opening changed the PNG metadata", err)
	}
}
