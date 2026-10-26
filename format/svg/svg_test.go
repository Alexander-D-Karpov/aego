package svg

import (
	"image/color"
	"testing"
)

func at(t *testing.T, r *Result, x, y int) color.NRGBA {
	t.Helper()
	return r.Image.NRGBAAt(x, y)
}

func render(t *testing.T, src string, w, h int) *Result {
	t.Helper()
	r, err := Render([]byte(src), w, h)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestShapesFillAndViewBox(t *testing.T) {
	r := render(t, `<svg viewBox="0 0 10 10"><rect x="0" y="0" width="5" height="10" fill="#ff0000"/><circle cx="7.5" cy="5" r="2" fill="blue"/></svg>`, 20, 20)
	if c := at(t, r, 2, 10); c != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatalf("rect pixel %v", c)
	}
	if c := at(t, r, 15, 10); c != (color.NRGBA{0, 0, 255, 255}) {
		t.Fatalf("circle centre %v", c)
	}
	if c := at(t, r, 19, 0); c.A != 0 {
		t.Fatalf("corner outside the shapes %v", c)
	}
}

func TestPathTransformGroupsAndStroke(t *testing.T) {
	src := `<svg viewBox="0 0 16 16">
	  <g fill="#00ff00" transform="translate(8 0)"><path d="M0 0h8v8H0z"/></g>
	  <path d="M0 12 L16 12" stroke="#000" stroke-width="2" fill="none"/>
	  <path d="M2 2 a2 2 0 1 0 4 0 a2 2 0 1 0 -4 0" fill="#f00"/>
	</svg>`
	r := render(t, src, 16, 16)
	if c := at(t, r, 12, 4); c != (color.NRGBA{0, 255, 0, 255}) {
		t.Fatalf("group fill with translate %v", c)
	}
	if c := at(t, r, 3, 4); c.A != 0 {
		t.Fatalf("translate did not move the square %v", c)
	}
	if c := at(t, r, 8, 12); c != (color.NRGBA{0, 0, 0, 255}) {
		t.Fatalf("stroke pixel %v", c)
	}
	if c := at(t, r, 8, 9); c.A != 0 {
		t.Fatalf("stroke too wide %v", c)
	}
	if c := at(t, r, 4, 2); c.R != 255 || c.A != 255 {
		t.Fatalf("arc circle %v", c)
	}
}

func TestOpacityEdgesAndSkipped(t *testing.T) {
	r := render(t, `<svg viewBox="0 0 4 4"><rect width="4" height="4" fill="#fff" opacity="0.5"/><rect x="0.5" width="1" height="4" fill="#000"/><filter/><text>hi</text></svg>`, 4, 4)
	if c := at(t, r, 3, 1); c.A < 126 || c.A > 129 {
		t.Fatalf("half opacity alpha %d", c.A)
	}
	if c := at(t, r, 0, 1); c.R < 80 || c.R > 90 || c.A < 188 || c.A > 194 {
		t.Fatalf("half-covered pixel is not blended: %v", c)
	}
	if len(r.Skipped) != 2 {
		t.Fatalf("skipped %v", r.Skipped)
	}
}

func TestBadInput(t *testing.T) {
	for _, src := range []string{`<rect/>`, `<svg><path d="10 10"/></svg>`, `<svg><rect fill="#zz"/></svg>`, `<svg><g transform="spin(3)"/></svg>`} {
		if _, err := Render([]byte(src), 8, 8); err == nil {
			t.Errorf("%s: no error", src)
		}
	}
}
