package shell

import (
	"reflect"
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/dock"
)

// Geometry checks are not a replacement for the deferred native screenshots,
// HiDPI testing, physical input, or validation of a saved user workspace.
func TestArtFactorySeparatesToolsCanvasLayersAndPalette(t *testing.T) {
	w := workspace.New(t.TempDir(), nil)
	for _, id := range []string{workspace.Assets, workspace.Scene, workspace.Hierarchy, workspace.Inspector, "art.tools", "art.options", "art.properties", "art.palette", "art.color", "art.layers", "art.channels", "art.selection", "art.info", "art.timeline", "art.preview", "art.asset_preview"} {
		if err := w.Register(workspace.Desc{Panel: defaultLayoutPanel(id)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, preset := range BuiltinWorkspaces() {
		if err := w.RegisterPreset(preset); err != nil {
			t.Fatal(err)
		}
	}
	w.LoadLayout()
	if !w.Switch(workspace.Art) {
		t.Fatal("Art workspace is unavailable")
	}
	if w.Tree.Has(workspace.Hierarchy) || w.Tree.Has(workspace.Inspector) {
		t.Fatal("scene panels occupy the Art factory layout")
	}
	tools, options := w.Tree.FindPanel("art.tools"), w.Tree.FindPanel("art.options")
	if tools == 0 || options == 0 || tools == options {
		t.Fatal("Art Tools and Tool Options must be independent panels")
	}
	if w.Tree.FindPanel(workspace.Assets) == 0 || w.Tree.FindPanel("art.timeline") == 0 || w.Tree.FindPanel(workspace.Assets) == w.Tree.FindPanel("art.timeline") {
		t.Fatal("Assets and Animation Timeline must have separate bottom docks")
	}
	for _, size := range []mathx.Vec2{{X: 800, Y: 600}, {X: 1366, Y: 768}, {X: 1920, Y: 1080}, {X: 2560, Y: 1440}} {
		w.Tree.Layout(mathx.Rect{W: size.X, H: size.Y})
		var rects [9]mathx.Rect
		for i, id := range []string{"art.tools", workspace.Scene, "art.layers", "art.palette", workspace.Assets, "art.preview", "art.options", "art.color", "art.timeline"} {
			r, ok := w.Tree.PanelRect(id)
			if !ok || r.W <= 0 || r.H <= 0 {
				t.Fatalf("%v: %s is not visible: %+v", size, id, r)
			}
			rects[i] = r
		}
		if rects[0].W > size.X*.17 || rects[1].W < size.X*.5 || rects[1].H <= rects[4].H {
			t.Fatalf("%v: unreasonable factory proportions: %+v", size, rects)
		}
		if rects[0].X != rects[6].X || rects[0].W != rects[6].W || rects[0].Y+rects[0].H > rects[6].Y {
			t.Fatalf("%v: Tool Options must be below Art Tools: %+v", size, rects)
		}
		if rects[4].Y < rects[1].Y+rects[1].H {
			t.Fatalf("%v: Assets must be below the canvas: %+v", size, rects)
		}
		if rects[4].Y != rects[8].Y || rects[4].H != rects[8].H || rects[4].X+rects[4].W > rects[8].X || rects[4].W < size.X*.12 || rects[8].W < min(320, size.X*.35) {
			t.Fatalf("%v: Timeline and Assets must remain usable beside each other: %+v", size, rects)
		}
		if rects[8].H > size.Y*.22 {
			t.Fatalf("%v: Timeline takes too much vertical space from the canvas: %+v", size, rects[8])
		}
		if w.Tree.FindPanel("art.layers") == w.Tree.FindPanel("art.palette") {
			t.Fatal("layers and palette are still in one tab group")
		}
		if w.Tree.FindPanel("art.color") == w.Tree.FindPanel("art.palette") {
			t.Fatal("color picker and palette must have separate docks")
		}
		if rects[3].X != rects[7].X || rects[3].W != rects[7].W || rects[3].Y+rects[3].H > rects[7].Y || rects[7].Y+rects[7].H > rects[5].Y {
			t.Fatalf("%v: palette, color picker and preview must be stacked in that order: %+v", size, rects)
		}
		if w.Tree.Has("art.selection") || w.Tree.Has("art.info") || w.Tree.Has("art.asset_preview") {
			t.Fatal("optional Art panels overcrowd the factory tab strip")
		}
	}
	for _, id := range []string{"art.selection", "art.info", "art.asset_preview"} {
		w.Show(id)
		if !w.Tree.Has(id) {
			t.Fatalf("optional panel %s cannot be opened", id)
		}
	}
}

func TestAssetClickKeepsSharedInspectorTabFromHidingAssets(t *testing.T) {
	w := workspace.New(t.TempDir(), nil)
	w.Tree = dock.Standard(nil, []string{workspace.Assets}, nil, []string{workspace.Inspector})
	if !assetClickMayRevealInspector(w) {
		t.Fatal("an independently docked inspector should still follow asset selection")
	}
	if !w.Tree.Undock(workspace.Inspector) || assetClickMayRevealInspector(w) {
		t.Fatal("single-click inspection must not reopen a closed panel and move asset tiles")
	}
	w.Tree.DockPanel(workspace.Inspector, w.Tree.FindPanel(workspace.Assets), dock.SideRight)
	w.Tree.DockPanel(workspace.Inspector, w.Tree.FindPanel(workspace.Assets), dock.SideCenter)
	w.Tree.Activate(workspace.Assets)
	if assetClickMayRevealInspector(w) {
		t.Fatal("single-click inspection would hide Assets in the same tab group")
	}
	if !w.Tree.ToggleMaximize(workspace.Assets) || assetClickMayRevealInspector(w) {
		t.Fatal("single-click inspection would disturb maximized Assets")
	}
	w.Tree.Restore()
	w.Tree.DockPanel(workspace.Inspector, w.Tree.FindPanel(workspace.Assets), dock.SideRight)
	if err := w.RegisterPreset(workspace.Preset{ID: workspace.Art}); err != nil {
		t.Fatal(err)
	}
	if assetClickMayRevealInspector(w) {
		t.Fatal("browsing Art should not force open or activate Inspector")
	}
}

// The actual prompt must evaluate the destructive predicate after live text
// input, not only on focus loss. Art's filesystem predicate is tested in the
// render2d editor package; this tests its real modal presentation boundary.
func TestArtSavePromptUpdatesReplaceWhileTyping(t *testing.T) {
	font := text.NewFont(14, 1, 16, 12)
	for r := rune(32); r <= 0x04ff; r++ {
		font.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	ui, state := core.New(font), &input.State{}
	var submitted []string
	q := context.Question{
		Title: "Destination", Fields: []string{"Source path"}, Values: []string{"assets/new.aeart"},
		Buttons: []string{"Save", "Cancel"}, DestructiveLabel: "Replace",
		DestructiveWhen: func(values []string) bool { return len(values) == 1 && values[0] == "./assets/existing.aeart" },
		Submit:          func(values []string) { submitted = append([]string(nil), values...) },
	}
	a := &App{ui: ui, modals: []modal{{kind: modalAsk, ask: q, vals: append([]string(nil), q.Values...), focus: true}}}
	id := core.IDOf("test.art.save.prompt")
	draw := func() string {
		ui.Begin(state, mathx.Vec2{X: 720, Y: 540}, 1)
		ui.OpenModal(id)
		a.drawPrompt(id)
		list := ui.End()
		// The list is the widget's real text command buffer. It is intentionally
		// inspected only in this synthetic UI test, without a GPU or OS input.
		return string(reflect.ValueOf(list).Elem().FieldByName("chars").Bytes())
	}
	state.BeginFrame()
	if labels := draw(); !strings.Contains(labels, "Save") || strings.Contains(labels, "Replace") {
		t.Fatalf("new destination did not display Save: %q", labels)
	}
	state.BeginFrame()
	state.ApplyText([]byte("./assets/existing.aeart"))
	if labels := draw(); !strings.Contains(labels, "Replace") || strings.Contains(labels, "Save") {
		t.Fatalf("typed existing destination did not display Replace in the same frame: %q", labels)
	}
	state.BeginFrame()
	state.ApplyKeyDown(input.KeyEnter, 0, 0, false)
	draw()
	if len(submitted) != 1 || submitted[0] != "./assets/existing.aeart" {
		t.Fatalf("Enter submitted the wrong live path: %v", submitted)
	}
}
