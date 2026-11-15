package integration

import (
	"strings"
	"testing"

	"aego/app/edit"
	"aego/asset"
	"aego/core/mathx"
	"aego/editor/shell"
	"aego/modules/animation"
	"aego/platform"
	"aego/platform/input"
	"aego/scene"
)

func createMenuPoint(s *edit.Session) mathx.Vec2 {
	ui := s.App.Context().UI
	width := func(label string) float32 { return ui.TextSize(label).X + 2*ui.Theme.Padding + 4 }
	return mathx.Vec2{X: 4 + width("File") + width("Edit") + width("Create")/2, Y: ui.Theme.BarHeight / 2}
}

func openCreateMenu(t *testing.T, s *edit.Session) {
	t.Helper()
	click(t, s, createMenuPoint(s))
	ui := s.App.Context().UI
	if !ui.PopupOpen(ui.ID("menu/Create")) || !s.App.AddPalette().Active() {
		t.Fatal("Create did not open its menu popup")
	}
	if ui.ModalActive() {
		t.Fatal("Create menu must not open a modal palette")
	}
}

func TestCreateMenuSearchCreatesUnderSelectionAndUndoes(t *testing.T) {
	s := workspaceSession(t)
	sc, parent := emptyNode(t, s)
	ctx := s.App.Context()
	before := len(sc.PreOrder())
	openCreateMenu(t, s)
	push(t, s, typed("empty")...)
	if got := s.App.AddPalette().Palette().Query(); got != "empty" {
		t.Fatalf("search got %q", got)
	}
	push(t, s, key(input.KeyEnter, 0)...)
	if s.App.AddPalette().Active() || ctx.UI.AnyPopup() || ctx.UI.Focus() != 0 {
		t.Fatal("picking an item must close the menu and release search focus")
	}
	selected := ctx.Selected()
	if len(selected) != 1 || sc.Parent(sc.Handle(selected[0])) != parent || len(sc.PreOrder()) != before+1 {
		t.Fatal("Create must select one new child of the original selection")
	}
	if err := ctx.Doc().History.Undo(); err != nil {
		t.Fatal(err)
	}
	if len(sc.PreOrder()) != before {
		t.Fatal("one undo must remove the created node")
	}
}

func TestCreateMenuDismissalAndMenuSwitching(t *testing.T) {
	s := workspaceSession(t)
	ui := s.App.Context().UI
	assertClosed := func() {
		t.Helper()
		if s.App.AddPalette().Active() || ui.PopupOpen(ui.ID("menu/Create")) || ui.Focus() != 0 {
			t.Fatalf("Create remained active=%v, popup=%v, focus=%v", s.App.AddPalette().Active(), ui.PopupOpen(ui.ID("menu/Create")), ui.Focus())
		}
	}
	openCreateMenu(t, s)
	push(t, s, key(input.KeyEscape, 0)...)
	assertClosed()
	openCreateMenu(t, s)
	click(t, s, createMenuPoint(s))
	assertClosed()
	openCreateMenu(t, s)
	click(t, s, mathx.Vec2{X: 700, Y: 500})
	assertClosed()
	openCreateMenu(t, s)
	outside := mathx.Vec2{X: 700, Y: 500}
	push(t, s,
		platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: outside}},
		platform.Event{Kind: platform.EvMouseDown, Mouse: input.MouseEvent{Pos: outside, Button: input.MouseRight}},
		platform.Event{Kind: platform.EvMouseUp, Mouse: input.MouseEvent{Pos: outside, Button: input.MouseRight}},
	)
	assertClosed()
	push(t, s, key(input.KeyEscape, 0)...)
	openCreateMenu(t, s)
	push(t, s, platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: mathx.Vec2{X: 8, Y: ui.Theme.BarHeight / 2}}})
	s.Step()
	assertClosed()
	if !ui.PopupOpen(ui.ID("menu/File")) {
		t.Fatal("hovering File must switch to its menu")
	}
	push(t, s, platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: createMenuPoint(s)}})
	if !ui.PopupOpen(ui.ID("menu/Create")) || !s.App.AddPalette().Active() || ui.ModalActive() {
		t.Fatal("hovering Create must switch back to its non-modal menu")
	}
}

func TestCreateMenuActionsAndSeparatePalette(t *testing.T) {
	s := workspaceSession(t)
	sc, parent := emptyNode(t, s)
	openCreateMenu(t, s)
	push(t, s, typed("clip")...)
	push(t, s, key(input.KeyEnter, 0)...)
	if _, ok := scene.Get[animation.AnimationPlayer](sc, parent); !ok {
		t.Fatal("the Create menu must include and run registered actions")
	}
	if err := s.App.Context().Doc().History.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, ok := scene.Get[animation.AnimationPlayer](sc, parent); ok {
		t.Fatal("action undo did not remove the added component")
	}
	openCreateMenu(t, s)
	push(t, s, key(input.KeyN, input.PrimaryMod()|input.ModShift)...)
	if !s.App.AddPalette().Active() || !s.App.Context().UI.ModalActive() || s.App.Context().UI.AnyPopup() {
		t.Fatal("the Create shortcut must still open the separate modal palette")
	}
}

func TestCreateMenuMouseAndKeepOpen(t *testing.T) {
	s := workspaceSession(t)
	sc, parent := emptyNode(t, s)
	ctx := s.App.Context()
	before := len(sc.PreOrder())
	openCreateMenu(t, s)
	push(t, s, typed("empty")...)
	push(t, s, key(input.KeyD, input.PrimaryMod())...)
	p := s.App.AddPalette().Palette()
	if i := p.Selected(); i < 0 || !p.Item(i).Starred {
		t.Fatal("the favorite shortcut must work in the menu")
	}
	push(t, s, key(input.KeyEnter, input.ModShift)...)
	if !p.Active() || !ctx.UI.PopupOpen(ctx.UI.ID("menu/Create")) || p.Query() != "empty" {
		t.Fatal("Shift+Enter must keep the menu and search open")
	}
	if len(sc.PreOrder()) != before+1 {
		t.Fatal("Shift+Enter did not create a node")
	}
	// The filtered result is the first row under the search field.
	ui := ctx.UI
	row := createMenuPoint(s)
	row.X -= ui.TextSize("Create").X / 2
	row.Y = ui.Theme.BarHeight + ui.Theme.Padding + ui.Theme.RowHeight + ui.Theme.Spacing + (ui.Theme.RowHeight+2)/2
	click(t, s, row)
	if p.Active() || ctx.UI.AnyPopup() || len(sc.PreOrder()) != before+2 {
		t.Fatal("clicking the search result must create and close the menu")
	}
	if selected := ctx.Selected(); len(selected) != 1 || sc.Parent(sc.Handle(selected[0])) != parent {
		t.Fatal("both picks must use the original selected parent")
	}
	for range 2 {
		if err := ctx.Doc().History.Undo(); err != nil {
			t.Fatal(err)
		}
	}
	if len(sc.PreOrder()) != before {
		t.Fatal("each pick must be independently undoable")
	}
}

func TestCreateMenuClosesOnDocumentChangeAndCreatesFilesWithoutScene(t *testing.T) {
	t.Setenv("AEGO_LANG", "en")
	s := workspaceSession(t)
	ctx := s.App.Context()
	openCreateMenu(t, s)
	other := ctx.Docs.New("scenes/other.aescene", scene.New(ctx.Types, nil, scene.Options{}))
	ctx.Docs.SetActive(other.ID)
	s.Step()
	if s.App.AddPalette().Active() || ctx.UI.PopupOpen(ctx.UI.ID("menu/Create")) {
		t.Fatal("the menu must close when its target document changes")
	}
	for len(ctx.Docs.Docs()) > 0 {
		if err := ctx.Docs.Close(ctx.Docs.Docs()[0].ID, true); err != nil {
			t.Fatal(err)
		}
	}
	s.Step()
	if ctx.Doc() != nil {
		t.Fatal("expected no active document")
	}
	s.App.AssetsPanel().SetFolder("assets")
	openCreateMenu(t, s)
	items := s.App.AddPalette().Palette().Items()
	want := map[string]bool{"act:file.animation": false, "act:render2d.pixel_art": false, "act:render2d.shader": false}
	for _, item := range items {
		if !strings.HasPrefix(item.Key, "act:") {
			t.Fatalf("scene template or component is available without a scene: %s", item.Key)
		}
		if _, ok := want[item.Key]; ok {
			want[item.Key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("file action %s is missing without a scene", key)
		}
	}
	push(t, s, key(input.KeyEscape, 0)...)
	if s.App.AddPalette().Active() || ctx.UI.AnyPopup() || ctx.UI.Focus() != 0 {
		t.Fatal("closing the file-only Create menu must release the popup and focus")
	}
	// Hover switching must work with a file-only Create menu as well.
	click(t, s, mathx.Vec2{X: 8, Y: ctx.UI.Theme.BarHeight / 2})
	s.Step()
	push(t, s, platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: createMenuPoint(s)}})
	s.Step()
	if ctx.UI.PopupOpen(ctx.UI.ID("menu/File")) || !ctx.UI.PopupOpen(ctx.UI.ID("menu/Create")) || !s.App.AddPalette().Active() {
		t.Fatal("hovering Create must replace File when project file actions are available")
	}
	push(t, s, typed("animation clip")...)
	pal := s.App.AddPalette().Palette()
	if i := pal.Selected(); i < 0 || pal.Item(i).Key != "act:file.animation" {
		t.Fatalf("animation file action was not selected: %d", i)
	}
	push(t, s, key(input.KeyEnter, 0)...)
	s.Step()
	if !ctx.UI.ModalActive() || ctx.UI.Focus() == 0 {
		t.Fatalf("file creation prompt did not take keyboard focus: modal=%v popup=%v focus=%v", ctx.UI.ModalActive(), ctx.UI.AnyPopup(), ctx.UI.Focus())
	}
	push(t, s, typed("no-scene-clip")...)
	push(t, s, key(input.KeyEnter, 0)...)
	id, ok := ctx.Pipeline.Lookup("assets/no-scene-clip.aeanim")
	if !ok {
		notice, _ := ctx.LastNotice()
		t.Fatalf("Create did not create an animation file without an open scene: notice=%+v modal=%v focus=%v", notice, ctx.UI.ModalActive(), ctx.UI.Focus())
	}
	entry, ok := ctx.Pipeline.Entry(id)
	if !ok || entry.Kind != asset.ArtAnim || ctx.Pipeline.VerifyCurrent(id) != nil || ctx.Doc() != nil {
		t.Fatalf("file-only creation did not import the clip or unexpectedly opened a scene: %+v", entry)
	}
}

func TestCreateMenuClosesWhenMenuBarIsHidden(t *testing.T) {
	s := workspaceSession(t)
	openCreateMenu(t, s)
	ctx := s.App.Context()
	if err := ctx.Commands.Run(shell.CommandFullscreenPlay); err != nil {
		t.Fatal(err)
	}
	s.Step()
	if !s.App.FullScreen() {
		t.Fatal("expected full-screen play")
	}
	if s.App.AddPalette().Active() || ctx.UI.PopupOpen(ctx.UI.ID("menu/Create")) {
		t.Fatal("hiding the menu bar must close its Create popup")
	}
}
