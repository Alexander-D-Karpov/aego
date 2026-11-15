package editor

import (
	stdctx "context"
	"reflect"
	"testing"

	"aego/art"
	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/platform/input"
)

func TestArtAuxiliaryPanelsRegisterSeparatelyAndFollowFocusedDocument(t *testing.T) {
	first := artActionFixture(t)
	second := artActionFixture(t)
	ws := workspace.New(t.TempDir(), nil)
	reg := ext.New(ext.Deps{Editor: first.ctx, Workspace: ws})
	d, err := newArtDocks(reg)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{"art.tools", "art.options", "art.properties", "art.palette", "art.color", "art.layers", "art.channels", "art.selection", "art.info", "art.timeline", "art.preview", "art.asset_preview"}
	for _, id := range ids {
		registered, ok := ws.Panel(id)
		if !ok || registered.ID() != id || registered.Title() == "" {
			t.Fatalf("panel %q was not registered independently", id)
		}
	}
	for _, id := range ids {
		panel := &artDockPanel{d: d, id: id}
		if panel.Document() != nil {
			t.Fatalf("%s retained a document before one was focused", id)
		}
		d.active = first
		if panel.Document() != first.doc {
			t.Fatalf("%s did not follow the first canonical document", id)
		}
		d.active = second
		if panel.Document() != second.doc {
			t.Fatalf("%s did not follow the second canonical document", id)
		}
		d.active = nil
	}
}

func TestArtCanvasDoesNotRecreateClosedAuthoringDocks(t *testing.T) {
	w := workspace.New(t.TempDir(), nil)
	for _, id := range []string{"art.layers", "art.timeline"} {
		if err := w.Register(workspace.Desc{Panel: &artDockPanel{id: id}, Slot: workspace.SlotRight}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{workspace.Design, workspace.Art, workspace.Animation, workspace.VFX} {
		// A non-nil empty Slots map is required for an explicit empty layout.
		// A nil map falls back to the panels' default slots instead.
		if err := w.RegisterPreset(workspace.Preset{ID: id, Explicit: true, Slots: map[string]workspace.Slot{}}); err != nil {
			t.Fatal(err)
		}
	}
	w.LoadLayout()
	if w.Visible("art.layers") || w.Visible("art.timeline") {
		t.Fatal("Design fixture unexpectedly opened Art docks")
	}
	p := &artPanel{reg: ext.New(ext.Deps{Workspace: w})}
	if p.usesArtDocks() {
		t.Fatal("Design without Art panels should retain its inline editor")
	}
	w.Show("art.layers")
	w.Show("art.timeline")
	if !p.usesArtDocks() {
		t.Fatal("other workspaces should use explicitly opened Art docks")
	}
	w.Toggle("art.timeline")
	if p.usesArtDocks() {
		t.Fatal("non-authoring workspaces lost their inline fallback")
	}
	for _, id := range []string{workspace.Art, workspace.Animation, workspace.VFX} {
		if !w.Switch(id) || !p.usesArtDocks() {
			t.Fatalf("%s recreated embedded controls when its docks were closed", id)
		}
	}
	if !w.Switch(workspace.Art) {
		t.Fatal("Art is unavailable")
	}
	if _, err := w.SaveAs("My Art"); err != nil {
		t.Fatal(err)
	}
	if !p.usesArtDocks() {
		t.Fatal("a named Art workspace recreated the closed docks")
	}
	if (&artPanel{}).usesArtDocks() {
		t.Fatal("a lightweight host without docking lost its inline editor")
	}
}

func TestAddForegroundSwatchUsesSharedHistoryAndRecoveryGuard(t *testing.T) {
	p := artActionFixture(t)
	p.view.color = mathx.ColorRGBA8(41, 83, 127, 203)
	before := p.model.Session.Document.Info().Palette
	cursor := p.model.History.Cursor()
	p.addForegroundSwatch()
	after := p.model.Session.Document.Info().Palette
	if p.err != nil || len(after) != len(before)+1 || after[len(before)].Color != artColorBytes(p.view.color) || p.view.palette != len(before) || p.model.History.Cursor() != cursor+1 {
		t.Fatal("adding the foreground did not use one canonical history step", p.err)
	}
	if err := p.model.SessionUndo(); err != nil || !reflect.DeepEqual(before, p.model.Session.Document.Info().Palette) {
		t.Fatal("Undo did not restore the palette", err)
	}
	p.model.PendingRecovery = true
	cursor = p.model.History.Cursor()
	p.addForegroundSwatch()
	if p.err == nil || p.model.History.Cursor() != cursor || !reflect.DeepEqual(before, p.model.Session.Document.Info().Palette) {
		t.Fatal("palette edit bypassed the pending recovery guard")
	}
}

func TestArtToolsDoNotDuplicateDedicatedFields(t *testing.T) {
	for _, tc := range []struct {
		options, color bool
		wantSize       bool
		wantColor      bool
	}{
		{false, false, true, true},
		{true, false, false, true},
		{false, true, true, false},
		{true, true, false, false},
	} {
		size, color := artInlineToolFields(tc.options, tc.color)
		if size != tc.wantSize || color != tc.wantColor {
			t.Fatalf("options=%v color=%v: size=%v color=%v", tc.options, tc.color, size, color)
		}
	}
}

func TestMaximizedArtCanvasKeepsInlineToolSelector(t *testing.T) {
	p := artActionFixture(t)
	w := workspace.New(t.TempDir(), nil)
	for _, panel := range []workspace.Panel{p, &artDockPanel{id: "art.tools"}} {
		if err := w.Register(workspace.Desc{Panel: panel, Slot: workspace.SlotCenter}); err != nil {
			t.Fatal(err)
		}
	}
	w.LoadLayout()
	w.Show(p.ID())
	w.Show("art.tools")
	p.reg = ext.New(ext.Deps{Workspace: w})
	if p.inlineTools() {
		t.Fatal("visible Tools dock duplicated its inline selector")
	}
	if !w.Tree.Maximize(p.ID()) || !p.inlineTools() || !w.Visible("art.tools") {
		t.Fatal("maximized canvas lost its tool selector or changed the saved layout")
	}
	w.Tree.Restore()
	if p.inlineTools() {
		t.Fatal("restoring the Tools dock kept the duplicate selector")
	}
	w.Toggle("art.tools")
	if !p.inlineTools() {
		t.Fatal("closing the Tools dock lost the inline selector")
	}
}

func TestColorPickerDoesNotDuplicatePaletteGrid(t *testing.T) {
	p := artActionFixture(t)
	panel := &artDockPanel{d: &artDocks{active: p}, id: "art.color"}
	ui, state := p.ctx.UI, &input.State{}
	area := mathx.Rect{W: 300, H: 900}
	draw := func() float32 {
		state.BeginFrame()
		ui.Begin(state, mathx.Vec2{X: area.W, Y: area.H}, 1)
		panel.Draw(&services.Frame{UI: ui}, area)
		ui.End()
		return p.view.partScroll["art.color"].content
	}
	before := draw()
	for i := 0; i < 16; i++ {
		swatch := art.Swatch{Color: art.Color{R: uint8(i), G: 123, A: 255}}
		if _, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "palette-add", Swatch: &swatch}); err != nil {
			t.Fatal(err)
		}
	}
	if after := draw(); after != before {
		t.Fatalf("Color Picker grew with the palette: before=%g after=%g", before, after)
	}
}

func TestArtRecentColorsAreBoundedAndKeepIndexedIdentity(t *testing.T) {
	v := &artView{}
	for i := 0; i < 15; i++ {
		v.rememberColor(art.Color{R: uint8(i), A: 255})
	}
	if len(v.recentColors) != 12 || v.recentColors[0].R != 14 || v.recentColors[11].R != 3 {
		t.Fatalf("unexpected recent color history: %+v", v.recentColors)
	}
	v.rememberColor(art.Color{R: 6, A: 255})
	if len(v.recentColors) != 12 || v.recentColors[0].R != 6 {
		t.Fatal("reselecting a recent color did not move it to the front")
	}
	seen := 0
	for _, color := range v.recentColors {
		if color.R == 6 {
			seen++
		}
	}
	if seen != 1 {
		t.Fatal("recent colors contain a duplicate")
	}
	shared := art.Color{R: 15, G: 60, B: 90, A: 128}
	palette := []art.Swatch{{}, {Color: shared}, {Color: shared}}
	if got := artPaletteColorIndex(palette, shared, 2); got != 2 {
		t.Fatal("choosing a recent color silently changed the duplicate palette index", got)
	}
	if got := artPaletteColorIndex(palette, shared, 0); got != 1 {
		t.Fatal("palette color lookup did not find the matching entry", got)
	}
	if got := artPaletteColorIndex(palette, art.Color{R: 15, G: 60, B: 90, A: 255}, 2); got != -1 {
		t.Fatal("a different alpha was treated as an existing indexed color", got)
	}
}

func TestArtSpaceDragPansWithoutPainting(t *testing.T) {
	p := artActionFixture(t)
	ui, state := p.ctx.UI, &input.State{}
	area := mathx.Rect{W: 400, H: 300}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: area.W, Y: area.H}, 1)
		p.canvas(&services.Frame{UI: ui, Focused: true}, area)
		ui.End()
	}
	state.BeginFrame()
	draw()
	before, origin, history := p.model.Session.Document.Info(), p.view.viewport.Center, p.model.History.Cursor()
	start := mathx.Vec2{X: 200, Y: 150}
	state.BeginFrame()
	state.ApplyKeyDown(input.KeySpace, 0, 0, false)
	state.ApplyMouseMove(start, mathx.Vec2{})
	state.ApplyMouseDown(start, input.MouseLeft)
	draw()
	if !p.view.panDragging || p.model.Session.Info().Gesture {
		t.Fatal("Space-drag started a drawing gesture")
	}
	state.BeginFrame()
	state.ApplyMouseMove(start.Add(mathx.Vec2{X: 40, Y: 20}), mathx.Vec2{X: 40, Y: 20})
	draw()
	if p.view.viewport.Center == origin {
		t.Fatal("Space-drag did not move the viewport")
	}
	state.BeginFrame()
	state.ApplyKeyUp(input.KeySpace, 0, 0)
	draw()
	if !p.view.panDragging {
		t.Fatal("releasing Space prematurely ended mouse capture")
	}
	state.BeginFrame()
	state.ApplyMouseUp(start.Add(mathx.Vec2{X: 40, Y: 20}), input.MouseLeft)
	draw()
	if p.view.panDragging || p.model.History.Cursor() != history || !reflect.DeepEqual(before, p.model.Session.Document.Info()) {
		t.Fatal("panning changed the drawing or history")
	}
}

func TestSpaceClickOnUnfocusedArtCanvasDoesNotPaint(t *testing.T) {
	p := artActionFixture(t)
	ui, state := p.ctx.UI, &input.State{}
	area := mathx.Rect{W: 400, H: 300}
	before, history := p.model.Session.Document.Info(), p.model.History.Cursor()
	point := mathx.Vec2{X: 200, Y: 150}
	state.BeginFrame()
	state.ApplyKeyDown(input.KeySpace, 0, 0, false)
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	ui.Begin(state, mathx.Vec2{X: area.W, Y: area.H}, 1)
	p.canvas(&services.Frame{UI: ui, Focused: false}, area)
	ui.End()
	if p.model.Session.Info().Gesture || p.model.History.Cursor() != history || !reflect.DeepEqual(before, p.model.Session.Document.Info()) {
		t.Fatal("Space-click on an unfocused canvas began painting")
	}
}

// Synthetic widget input; not an OS/GPU or finished-layout acceptance test.
func TestArtLayersPanelUsesSharedHistoryAndIndependentScroll(t *testing.T) {
	p := artActionFixture(t)
	d := &artDocks{active: p}
	ui := p.ctx.UI
	state := &input.State{}
	area := mathx.Rect{W: 300, H: 300}
	draw := func(id string) {
		ui.Begin(state, mathx.Vec2{X: 300, Y: 300}, 1)
		(&artDockPanel{d: d, id: id}).Draw(&services.Frame{UI: ui}, area)
		ui.End()
	}
	state.BeginFrame()
	draw("art.palette")
	state.BeginFrame()
	draw("art.layers")
	if p.view.partScroll["art.palette"] == nil || p.view.partScroll["art.layers"] == nil || p.view.partScroll["art.palette"] == p.view.partScroll["art.layers"] {
		t.Fatal("independent dock panels share scroll state")
	}
	beforeLayers, beforeHistory := len(p.model.Session.Document.Info().Layers), p.model.History.Cursor()
	pad, height := ui.Theme.Padding, ui.Theme.RowHeight
	point := mathx.Vec2{X: pad + (area.W-2*pad)/6, Y: pad + height/2}
	state.BeginFrame()
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	draw("art.layers")
	state.BeginFrame()
	state.ApplyMouseUp(point, input.MouseLeft)
	draw("art.layers")
	if p.err != nil || len(p.model.Session.Document.Info().Layers) != beforeLayers+1 || p.model.History.Cursor() != beforeHistory+1 {
		t.Fatalf("layer dock did not mutate the shared history once: %v", p.err)
	}
	if err := p.model.SessionUndo(); err != nil {
		t.Fatal(err)
	}
	if len(p.model.Session.Document.Info().Layers) != beforeLayers {
		t.Fatal("Undo did not restore the canonical document")
	}
}
