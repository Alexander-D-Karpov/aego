package widgets

import (
	"testing"

	"aego/core/curve"
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
)

// clickOpen presses and releases b at a point on separate frames and reports whether popup id is open on the frame after.
func clickOpen(b input.Button, at mathx.Vec2, id core.ID, draw func(c *core.Context)) bool {
	c, in := core.New(nil), &input.State{}
	frame := func(press, release bool) {
		in.BeginFrame()
		in.ApplyMouseMove(at, mathx.Vec2{})
		if press {
			in.ApplyMouseDown(at, b)
		}
		if release {
			in.ApplyMouseUp(at, b)
		}
		c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		draw(c)
		c.End()
	}
	frame(false, false)
	frame(true, false)
	frame(false, true)
	frame(false, false)
	return c.PopupOpen(id)
}

func TestComboOpensWithTheMouse(t *testing.T) {
	r := mathx.Rect{X: 10, Y: 10, W: 120, H: 20}
	id := core.IDOf("combo")
	cur := 0
	if !clickOpen(input.MouseLeft, mathx.Vec2{X: 50, Y: 20}, id, func(c *core.Context) { Combo(c, id, r, &cur, []string{"a", "b"}, false) }) {
		t.Fatal("a click on the combo must leave its list open")
	}
}

func TestIconComboMouseSelection(t *testing.T) {
	r := mathx.Rect{X: 10, Y: 10, W: 120, H: 20}
	id := core.IDOf("icon-combo")
	cur := 0
	c, in := core.New(nil), &input.State{}
	options := []IconOption{{Icon: "tool.pencil", Label: "Pencil"}, {Icon: "tool.eraser", Label: "Eraser"}}
	changed := false
	step := func(at mathx.Vec2, press, release bool) {
		in.BeginFrame()
		in.ApplyMouseMove(at, mathx.Vec2{})
		if press {
			in.ApplyMouseDown(at, input.MouseLeft)
		}
		if release {
			in.ApplyMouseUp(at, input.MouseLeft)
		}
		c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		changed = IconCombo(c, id, r, &cur, options) || changed
		c.End()
	}
	field := mathx.Vec2{X: 50, Y: 20}
	step(field, true, false)
	step(field, false, true)
	step(field, false, false) // establish the popup's measured bounds
	if !c.PopupOpen(id) {
		t.Fatal("icon combo did not open")
	}
	second := mathx.Vec2{X: r.X + c.Theme.Padding + 8, Y: r.Y + r.H + c.Theme.Padding + c.Theme.RowHeight + c.Theme.Spacing + c.Theme.RowHeight/2}
	step(second, true, false)
	step(second, false, true)
	if !changed || cur != 1 || c.PopupOpen(id) {
		t.Fatalf("selection failed: changed=%v index=%d open=%v", changed, cur, c.PopupOpen(id))
	}
}

func TestContextMenuOpensWithTheMouse(t *testing.T) {
	r := mathx.Rect{X: 0, Y: 0, W: 400, H: 300}
	id := core.IDOf("menu")
	if !clickOpen(input.MouseRight, mathx.Vec2{X: 100, Y: 100}, id, func(c *core.Context) {
		if BeginContextMenu(c, id, r) {
			MenuItem(c, "Item", "", true, false)
			EndContextMenu(c)
		}
	}) {
		t.Fatal("a right click must leave the context menu open")
	}
}

func TestCurveFieldOpensWithTheMouse(t *testing.T) {
	r := mathx.Rect{X: 10, Y: 10, W: 120, H: 20}
	id := core.IDOf("curve")
	cv := curve.Line(0, 0, 1, 1)
	st := NewCurveState()
	if !clickOpen(input.MouseLeft, mathx.Vec2{X: 50, Y: 20}, id, func(c *core.Context) { CurveField(c, id, r, &cv, false, st) }) {
		t.Fatal("a click on the curve field must leave its editor open")
	}
}

func TestPopupClosesOnALaterClickOutside(t *testing.T) {
	r := mathx.Rect{X: 10, Y: 10, W: 120, H: 20}
	id := core.IDOf("combo")
	cur := 0
	c, in := core.New(nil), &input.State{}
	step := func(at mathx.Vec2, press, release bool) {
		in.BeginFrame()
		in.ApplyMouseMove(at, mathx.Vec2{})
		if press {
			in.ApplyMouseDown(at, input.MouseLeft)
		}
		if release {
			in.ApplyMouseUp(at, input.MouseLeft)
		}
		c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		Combo(c, id, r, &cur, []string{"a", "b"}, false)
		c.End()
	}
	at, far := mathx.Vec2{X: 50, Y: 20}, mathx.Vec2{X: 700, Y: 500}
	step(at, true, false)
	step(at, false, true)
	step(far, true, false)
	if c.PopupOpen(id) {
		t.Fatal("a click outside must close the list")
	}
}

// A curve graph with a time range from outside shows that range, not the span of its keys.
func TestCurveGraphTakesAnOutsideTimeRange(t *testing.T) {
	c, in := core.New(nil), &input.State{}
	cv := curve.Line(0.5, 0, 1, 1)
	st := NewCurveState()
	st.SetTimeRange(0, 2)
	in.BeginFrame()
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	CurveGraph(c, core.IDOf("g"), mathx.Rect{X: 10, Y: 10, W: 300, H: 120}, &cv, st)
	c.End()
	if st.view[0] != 0 || st.view[1] != 2 {
		t.Fatalf("the graph shows %v..%v, want 0..2", st.view[0], st.view[1])
	}
}
