package layout

import (
	"testing"

	"aego/core/mathx"
	"aego/ui/core"
)

func newCtx(t *testing.T) *core.Context {
	t.Helper()
	c := core.New(nil)
	c.Begin(nil, mathx.Vec2{X: 800, Y: 600}, 1)
	return c
}

func TestPaddingRestoresCursor(t *testing.T) {
	c := newCtx(t)
	cur := c.Cursor()
	startX, startW := cur.X, cur.W
	Padding(c, 10, func() {
		if cur.X != startX+10 || cur.W != startW-20 {
			t.Fatalf("inside padding: x=%v w=%v", cur.X, cur.W)
		}
	})
	if cur.X != startX || cur.W != startW {
		t.Fatalf("cursor not restored: x=%v w=%v", cur.X, cur.W)
	}
}

func TestIndent(t *testing.T) {
	c := newCtx(t)
	cur := c.Cursor()
	startX, startW := cur.X, cur.W
	Indent(c, 16, func() {
		if cur.X != startX+16 || cur.W != startW-16 {
			t.Fatal("indent not applied")
		}
		Indent(c, 16, func() {
			if cur.X != startX+32 {
				t.Fatal("nested indent not applied")
			}
		})
	})
	if cur.X != startX || cur.W != startW {
		t.Fatal("indent not restored")
	}
}

func TestRowSplitsWidthAndAlignsY(t *testing.T) {
	c := newCtx(t)
	cur := c.Cursor()
	cur.W = 300
	startY := cur.Y
	var widths []float32
	var xs []float32
	Row(c, []float32{1, 2, 1}, func(i int) {
		widths = append(widths, cur.W)
		xs = append(xs, cur.X)
		if cur.Y != startY {
			t.Fatalf("column %d does not start at the row top", i)
		}
		cur.Next(float32(10 * (i + 1)))
	})
	if len(widths) != 3 {
		t.Fatalf("row visited %d columns", len(widths))
	}
	gap := c.Theme.Spacing
	available := float32(300) - gap*2
	if mathx.Abs(widths[1]-available/2) > 0.01 {
		t.Fatalf("middle column width = %v", widths[1])
	}
	if mathx.Abs(widths[0]-widths[2]) > 0.01 {
		t.Fatal("equal weights gave different widths")
	}
	if xs[1] <= xs[0] || xs[2] <= xs[1] {
		t.Fatalf("columns not laid out left to right: %v", xs)
	}
	if cur.W != 300 {
		t.Fatal("row did not restore the width")
	}
	if cur.Y <= startY {
		t.Fatal("row must advance past the tallest column")
	}
}

func TestRowIgnoresEmptyWeights(t *testing.T) {
	c := newCtx(t)
	called := false
	Row(c, nil, func(int) { called = true })
	Row(c, []float32{0, 0}, func(int) { called = true })
	if called {
		t.Fatal("row with no weight must not call the body")
	}
}

func TestSpacer(t *testing.T) {
	c := newCtx(t)
	cur := c.Cursor()
	y := cur.Y
	Spacer(c, 24)
	if cur.Y != y+24 {
		t.Fatalf("cursor y = %v", cur.Y)
	}
}

func TestScrollAreaClipsAndAdvances(t *testing.T) {
	c := newCtx(t)
	cur := c.Cursor()
	startY := cur.Y
	offset := float32(0)
	inner := 0
	ScrollArea(c, core.IDOf("scroll"), 100, &offset, func() {
		for i := 0; i < 20; i++ {
			cur.Next(0)
			inner++
		}
	})
	if inner != 20 {
		t.Fatal("body did not run")
	}
	if cur.Y < startY+100 {
		t.Fatalf("scroll area must advance by its height: %v", cur.Y-startY)
	}
	if c.List().Empty() {
		t.Fatal("clip commands were not recorded")
	}
}
