package palette

import (
	"fmt"
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
	"aego/ui/widgets"
)

func TestPopupLayoutSearchAndKeyboard(t *testing.T) {
	ui := core.New(text.NewFont(14, 1, 18, 14))
	ui.Theme = theme.Editor()
	p := New(core.IDOf("create-menu-test"))
	var items []Item
	for i := range 20 {
		items = append(items, Item{Key: fmt.Sprint(i), Label: fmt.Sprintf("Item %02d", i), Category: "Nodes"})
	}
	p.SetItems(items)
	in := &input.State{}
	var cursor core.Cursor
	var result Result
	rowH := ui.Theme.RowHeight + rowPad
	draw := func() {
		ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		mb := widgets.BeginMenuBar(ui, mathx.Rect{W: 800, H: 28})
		if mb.Menu("Create", 280) {
			if !p.Active() {
				p.ShowPopup(ui)
			}
			cursor = *ui.Cursor()
			result = p.DrawPopup(ui, 8*rowH)
			mb.EndMenu()
		} else if p.Active() {
			p.Hide(ui)
		}
		ui.End()
	}
	at := mathx.Vec2{X: 10, Y: 10}
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	in.ApplyMouseDown(at, input.MouseLeft)
	draw()
	in.BeginFrame()
	in.ApplyMouseUp(at, input.MouseLeft)
	draw()
	in.BeginFrame()
	draw()
	if !p.Active() || ui.ModalActive() || !ui.PopupOpen(ui.ID("menu/Create")) {
		t.Fatal("menu must be an open non-modal popup")
	}
	if cursor.X != 4+ui.Theme.Padding || cursor.Y != 28+ui.Theme.Padding || cursor.W != 280-2*ui.Theme.Padding {
		t.Fatalf("popup content is not anchored under the menu: %+v", cursor)
	}
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyEnter, 0, 0, false)
	draw()
	if p.subCategory != "Nodes" || p.subSelected != 0 || !ui.PopupOpen(core.IDSeq(p.id, 4)) {
		t.Fatal("Enter must open the selected category's submenu")
	}
	in.BeginFrame()
	in.ApplyKeyUp(input.KeyEnter, 0, 0)
	draw()
	if p.listH != rowH {
		t.Fatalf("root list height %v, want one category row", p.listH)
	}
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyEnd, 0, 0, false)
	draw()
	if p.subSelected != 19 || p.subScroll <= 0 {
		t.Fatalf("keyboard navigation must scroll to the last item: selected=%d scroll=%v", p.subSelected, p.subScroll)
	}
	in.BeginFrame()
	in.ApplyText([]byte("Item 19"))
	draw()
	if p.Query() != "Item 19" || p.Selected() != 19 {
		t.Fatalf("search=%q selected=%d", p.Query(), p.Selected())
	}
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyEnter, 0, input.ModShift, false)
	draw()
	if result.Pick != 19 || !result.Keep {
		t.Fatalf("Shift+Enter result: %+v", result)
	}
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyEscape, 0, 0, false)
	draw()
	if p.Active() || ui.PopupOpen(ui.ID("menu/Create")) || ui.Focus() != 0 {
		t.Fatal("Escape must close the popup and release its search focus")
	}
}

func TestHidingPopupDoesNotCloseAnotherModalOrStealFocus(t *testing.T) {
	ui := core.New(nil)
	p := New(core.IDOf("create-menu-test"))
	p.ShowPopup(ui)
	otherModal, otherFocus := core.IDOf("other-modal"), core.IDOf("other-focus")
	ui.OpenModal(otherModal)
	ui.SetFocus(otherFocus)
	p.Hide(ui)
	if !ui.ModalOpen(otherModal) || ui.Focus() != otherFocus {
		t.Fatal("hiding the popup interfered with another modal or its focus")
	}
}

func TestPopupFitsBelowMenuOnShortScreen(t *testing.T) {
	ui := core.New(text.NewFont(14, 1, 18, 14))
	ui.Theme = theme.Editor()
	p := New(core.IDOf("short-create-menu"))
	p.Hint = "Enter create"
	var items []Item
	for i := range 20 {
		items = append(items, Item{Key: fmt.Sprint(i), Label: fmt.Sprint(i), Category: fmt.Sprintf("Category %d", i)})
	}
	p.SetItems(items)
	const height = 180
	barH := ui.Theme.BarHeight
	available := height - barH - 2*(ui.Theme.Padding+ui.Theme.RowHeight+ui.Theme.Spacing)
	var y float32
	in := &input.State{}
	draw := func() {
		ui.Begin(in, mathx.Vec2{X: 400, Y: height}, 1)
		mb := widgets.BeginMenuBar(ui, mathx.Rect{W: 400, H: barH})
		if mb.Menu("Create", 280) {
			if !p.Active() {
				p.ShowPopup(ui)
			}
			y = ui.Cursor().Y
			p.DrawPopup(ui, min(8*(ui.Theme.RowHeight+rowPad), available))
			mb.EndMenu()
		}
		ui.End()
	}
	at := mathx.Vec2{X: 10, Y: 10}
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	in.ApplyMouseDown(at, input.MouseLeft)
	draw()
	in.BeginFrame()
	in.ApplyMouseUp(at, input.MouseLeft)
	draw()
	if y != barH+ui.Theme.Padding || p.listH != available {
		t.Fatalf("short-screen popup y=%v list height=%v, want %v and %v", y, p.listH, barH+ui.Theme.Padding, available)
	}
}

func TestPopupCategoryHoverAndMouseSelection(t *testing.T) {
	ui := core.New(text.NewFont(14, 1, 18, 14))
	ui.Theme = theme.Editor()
	p := New(core.IDOf("hover-create-menu"))
	p.SetItems([]Item{{Key: "a", Label: "One", Category: "First"}, {Key: "b", Label: "Two", Category: "Second"}})
	in := &input.State{}
	var listY float32
	var result Result
	draw := func() {
		ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		mb := widgets.BeginMenuBar(ui, mathx.Rect{W: 800, H: 28})
		if mb.Menu("Create", 280) {
			if !p.Active() {
				p.ShowPopup(ui)
			}
			listY = ui.Cursor().Y + ui.Theme.RowHeight + ui.Theme.Spacing
			result = p.DrawPopup(ui, 200)
			mb.EndMenu()
		}
		ui.End()
	}
	at := mathx.Vec2{X: 10, Y: 10}
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	in.ApplyMouseDown(at, input.MouseLeft)
	draw()
	in.BeginFrame()
	in.ApplyMouseUp(at, input.MouseLeft)
	draw()
	in.BeginFrame()
	draw()
	rowH := ui.Theme.RowHeight + rowPad
	at = mathx.Vec2{X: 40, Y: listY + rowH/2}
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	draw()
	if p.subCategory != "First" || !ui.PopupOpen(core.IDSeq(p.id, 4)) {
		t.Fatal("hover must open the first submenu without a click")
	}
	at.Y += rowH
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	draw()
	if p.subCategory != "Second" {
		t.Fatalf("hover must switch categories, got %q", p.subCategory)
	}
	in.BeginFrame()
	draw() // popup measures its content before it accepts clicks
	at = mathx.Vec2{X: p.subList.X + 40, Y: p.subList.Y + rowH/2}
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	in.ApplyMouseDown(at, input.MouseLeft)
	draw()
	if result.Pick != 1 {
		t.Fatalf("clicking a submenu item picked %d, want 1", result.Pick)
	}
}
