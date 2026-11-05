package palette

import (
	"fmt"
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
)

// The click on a button that opens the palette must not count as a click outside it.
func TestOpeningClickKeepsItOpen(t *testing.T) {
	c := core.New(nil)
	in := &input.State{}
	at := mathx.Vec2{X: 50, Y: 15}
	in.BeginFrame()
	in.ApplyMouseMove(at, mathx.Vec2{})
	in.ApplyMouseDown(at, input.MouseLeft)
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	p := New(core.IDOf("pal"))
	p.SetItems([]Item{{Key: "a", Label: "Alpha"}})
	p.Show(c)
	if res := p.Draw(c, mathx.Rect{X: 10, Y: 40, W: 400, H: 300}); res.Close {
		t.Fatal("the opening press closed the palette")
	}
	c.End()
	in.BeginFrame()
	in.ApplyMouseUp(at, input.MouseLeft)
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	if res := p.Draw(c, mathx.Rect{X: 10, Y: 40, W: 400, H: 300}); res.Close {
		t.Fatal("releasing the opening press closed the palette")
	}
	c.End()
	in.BeginFrame()
	in.ApplyMouseDown(at, input.MouseLeft)
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	if res := p.Draw(c, mathx.Rect{X: 10, Y: 40, W: 400, H: 300}); !res.Close {
		t.Fatal("a later click outside must close it")
	}
}

func TestCategoryExpansionScrollsItsContentsIntoView(t *testing.T) {
	for _, method := range []string{"mouse", "keyboard"} {
		t.Run(method, func(t *testing.T) {
			ui := core.New(text.NewFont(14, 1, 18, 14))
			ui.Theme = theme.Editor()
			p := New(core.IDOf("category-scroll"))
			p.Title = "Add Component"
			var items []Item
			for i := range 10 {
				items = append(items, Item{Key: fmt.Sprint(i), Label: fmt.Sprint(i), Category: fmt.Sprintf("Category %02d", i)})
			}
			for i := range 8 {
				items = append(items, Item{Key: fmt.Sprint(10 + i), Label: fmt.Sprint(i), Category: "Category 09"})
			}
			p.SetItems(items)
			in := &input.State{}
			r := mathx.Rect{X: 10, Y: 10, W: 400, H: 200}
			draw := func() {
				ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
				p.Draw(ui, r)
				ui.End()
			}
			in.BeginFrame()
			ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
			p.Show(ui)
			p.Draw(ui, r)
			ui.End()
			rowH := ui.Theme.RowHeight + rowPad
			p.sel, p.follow = 9, true
			in.BeginFrame()
			draw()
			if method == "mouse" {
				listY := r.Y + ui.Theme.Padding + 2*(ui.Theme.RowHeight+ui.Theme.Spacing)
				at := mathx.Vec2{X: r.X + 30, Y: listY + 9*rowH - p.scroll + rowH/2}
				in.BeginFrame()
				in.ApplyMouseMove(at, mathx.Vec2{})
				in.ApplyMouseDown(at, input.MouseLeft)
			} else {
				in.BeginFrame()
				in.ApplyKeyDown(input.KeyEnter, 0, 0, false)
			}
			draw()
			in.BeginFrame()
			draw() // allow the virtual list to clamp the new scroll offset
			if !p.Open["Category 09"] || p.rows[p.sel].label != "Category 09" {
				t.Fatal("the selected category did not open")
			}
			if y := float32(p.sel)*rowH - p.scroll; y < 0 || y > rowH {
				t.Fatalf("category should be near the top, y=%v scroll=%v", y, p.scroll)
			}
			if remaining := float32(len(p.rows))*rowH - p.scroll; remaining <= rowH {
				t.Fatal("no category items are visible")
			}
		})
	}
}

func TestDescriptionFollowsHoverWithoutBreakingKeyboard(t *testing.T) {
	ui := core.New(text.NewFont(14, 1, 18, 14))
	ui.Theme = theme.Editor()
	p := New(core.IDOf("hover-description"))
	p.Title = "Add Component"
	p.Open["Scripts"] = true
	p.SetItems([]Item{{Label: "First", Category: "Scripts"}, {Label: "Second", Category: "Scripts"}})
	described := -1
	p.Describe = func(_ *core.Context, _ mathx.Rect, item int) { described = item }
	in := &input.State{}
	r := mathx.Rect{X: 10, Y: 10, W: 500, H: 300}
	draw := func() {
		ui.Begin(in, mathx.V2(800, 600), 1)
		p.Draw(ui, r)
		ui.End()
	}
	in.BeginFrame()
	ui.Begin(in, mathx.V2(800, 600), 1)
	p.Show(ui)
	p.Draw(ui, r)
	ui.End()
	listY := r.Y + ui.Theme.Padding + 2*(ui.Theme.RowHeight+ui.Theme.Spacing)
	rowH := ui.Theme.RowHeight + rowPad
	in.BeginFrame()
	in.ApplyMouseMove(mathx.V2(r.X+70, listY+2*rowH+rowH/2), mathx.V2(50, 50))
	draw()
	if described != 1 || p.Selected() != 1 {
		t.Fatalf("hover described %d, selected %d; want second item", described, p.Selected())
	}
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyUp, 0, 0, false)
	draw()
	if described != 0 || p.Selected() != 0 {
		t.Fatalf("keyboard was overridden by a stationary mouse: description %d, selected %d", described, p.Selected())
	}
}
