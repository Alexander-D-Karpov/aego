//go:build aego_generate_effects

package art

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"os"
	"path/filepath"
	"testing"
)

// This is an opt-in source-art generator, not a runtime brush or renderer.
// Every mark goes through the same validated Edit operations as the editor.
func TestGenerateEffectTemplates(t *testing.T) {
	if os.Getenv("AEGO_GENERATE_EFFECT_TEMPLATES") != "1" {
		t.Skip("set AEGO_GENERATE_EFFECT_TEMPLATES=1 to regenerate source art")
	}
	for _, template := range EffectTemplates() {
		t.Run(template.ID, func(t *testing.T) {
			doc, err := New(32, 32, RGBA, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256([]byte("aego.effect-template.v1/" + template.ID))
			doc.id = hex.EncodeToString(sum[:16])
			doc.current.Properties[EffectTemplateProperty] = template.ID
			e, err := doc.Begin(doc.Revision(), "Create effect starter")
			if err != nil {
				t.Fatal(err)
			}
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			palette := []Swatch{
				{Name: "Transparent"},
				{Color{77, 49, 77, 255}, "Shadow"},
				{Color{145, 106, 120, 255}, "Smoke"},
				{Color{221, 165, 115, 255}, "Dust"},
				{Color{247, 93, 54, 255}, "Flame"},
				{Color{255, 171, 66, 255}, "Amber"},
				{Color{255, 238, 151, 255}, "Glow"},
				{Color{248, 255, 239, 255}, "White"},
				{Color{91, 71, 173, 255}, "Violet"},
				{Color{68, 173, 212, 255}, "Blue"},
				{Color{136, 246, 221, 255}, "Cyan"},
			}
			must(e.ReplacePalette(palette, false))
			base := e.Info().Layers[0]
			base.Name = "Silhouette"
			must(e.SetLayer(base))
			detail, err := e.AddLayer("Highlights", Raster, "")
			must(err)
			first, last := e.Info().Frames[0].ID, e.Info().Frames[0].ID
			must(e.SetDuration(first, 100))
			for frame := 0; frame < 8; frame++ {
				if frame > 0 {
					last, err = e.AddFrame(last, 100)
					must(err)
				}
				body, err := e.EnsureCel(base.ID, last, Albedo)
				must(err)
				highlight, err := e.EnsureCel(detail, last, Albedo)
				must(err)
				var glow ID
				if template.ID != "dust" && template.ID != "smoke" {
					glow, err = e.EnsureCel(detail, last, Emission)
					must(err)
				}
				drawEffectStarterFrame(t, e, template.ID, frame, body, highlight, glow, palette)
			}
			_, err = e.SetTag(Tag{Name: template.ID, From: first, To: last, Direction: Forward})
			must(err)
			pivot := image.Pt(16, 16)
			if template.ID == "fire" || template.ID == "dust" || template.ID == "smoke" {
				pivot.Y = 27
			}
			_, err = e.SetSlice(Slice{Name: "pivot", Rect: image.Rect(0, 0, 32, 32), Pivot: pivot})
			must(err)
			change, err := e.Prepare()
			must(err)
			must(change.Apply())
			data, err := doc.Encode()
			must(err)
			must(os.WriteFile(filepath.Join("res", "effects", template.ID+SourceExtension), data, 0644))
		})
	}
}

func drawEffectStarterFrame(t *testing.T, e *Edit, template string, f int, body, detail, emission ID, p []Swatch) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	brush := func(index, alpha int) Brush {
		c := p[index].Color
		c.A = uint8(alpha)
		return Brush{Paint: Paint{Color: c}, Size: 1}
	}
	ellipse := func(cel ID, x, y, rx, ry, color, alpha int) {
		must(e.Shape(cel, "ellipse", image.Pt(x-rx, y-ry), image.Pt(x+rx, y+ry), brush(color, alpha), nil, true))
	}
	line := func(cel ID, x0, y0, x1, y1, color, alpha int) {
		must(e.Line(cel, image.Pt(x0, y0), image.Pt(x1, y1), brush(color, alpha), nil))
	}
	polygon := func(cel ID, points []image.Point, color, alpha int) {
		must(e.Polygon(cel, points, brush(color, alpha), nil, true))
	}
	glowEllipse := func(x, y, rx, ry, color, alpha int) {
		ellipse(detail, x, y, rx, ry, color, alpha)
		if emission != "" {
			ellipse(emission, x, y, rx, ry, color, alpha)
		}
	}
	switch template {
	case "dust":
		a, y, r := 255-f*24, 24-f, 3+f/2
		ellipse(body, 10-f/2, y+1, r+1, r-1, 1, a)
		ellipse(body, 20+f/2, y, r+2, r, 2, a)
		ellipse(body, 16, y-2, r+1, r+1, 3, a)
		ellipse(detail, 14, y-4, r-1, r-1, 6, a*2/3)
		line(detail, 5, 27, 10+f, 27, 3, a)
	case "smoke":
		x, y, a, r := 15+(f%3)-1, 24-f*2, 220-f*18, 4+f/2
		ellipse(body, x-3, y+2, r, r-1, 1, a)
		ellipse(body, x+3, y, r, r, 2, a)
		ellipse(detail, x, y-3, r-1, r-2, 3, a/2)
		ellipse(detail, x+2, y-4, max(1, r-3), max(1, r-4), 6, a/3)
	case "spark":
		r, a, x, y := 10-f, 255-f*20, 16+f/3, 18-f
		polygon(body, []image.Point{{x, y - r}, {x + 2, y - 2}, {x + r, y}, {x + 2, y + 2}, {x, y + r}, {x - 2, y + 2}, {x - r, y}, {x - 2, y - 2}}, 5, a)
		glowEllipse(x, y, 2, 2, 7, a)
		line(detail, x-r, y-r, x-r+2, y-r+2, 6, a)
		line(detail, x+r-2, y+r-2, x+r, y+r, 4, a)
	case "explosion":
		r, a := 3+min(f, 5)*2, 255-max(0, f-3)*45
		polygon(body, []image.Point{{16, 16 - r}, {19, 11}, {16 + r, 12}, {23, 17}, {16 + r, 16 + r}, {17, 23}, {12, 16 + r}, {10, 20}, {16 - r, 14}, {11, 13}}, 4, a)
		ellipse(body, 16, 16, r-1, max(1, r-2), 5, a)
		glowEllipse(15, 15, max(1, 4-f/2), max(1, 5-f/2), 6, a)
		glowEllipse(15, 14, 1, 2, 7, a)
		if f > 2 {
			ellipse(detail, 6, 10, 1, 1, 5, a)
			ellipse(detail, 24, 24, 1, 1, 4, a)
			line(detail, 24, 5, 27, 3+f, 6, a)
		}
	case "fire":
		w := (f % 4) - 2
		polygon(body, []image.Point{{9, 27}, {6, 21}, {10, 13}, {12, 18}, {17 + w, 3}, {20, 14}, {23, 10 + f%3}, {26, 22}, {22, 27}}, 4, 255)
		polygon(body, []image.Point{{11, 26}, {10, 22}, {15, 12 + f%3}, {16, 19}, {21 - w, 11}, {23, 23}, {20, 27}}, 5, 255)
		flame := []image.Point{{13, 27}, {12, 23}, {16 - w, 17}, {17, 22}, {19, 19}, {21, 25}, {19, 27}}
		polygon(detail, flame, 6, 255)
		polygon(emission, flame, 6, 255)
		glowEllipse(16, 25, 2, 2, 7, 255)
		line(detail, 10+f, 6, 10+f, 4, 5, 210)
	case "hit-flash":
		r, a := 12-f, 255-f*25
		star := []image.Point{{16, 16 - r}, {18, 13}, {16 + r, 7}, {21, 15}, {16 + r, 17}, {20, 19}, {24, 16 + r}, {17, 21}, {16, 16 + r}, {14, 20}, {16 - r, 24}, {12, 17}, {16 - r, 16}, {13, 14}, {7, 16 - r}, {15, 12}}
		polygon(body, star, 5, a)
		glowEllipse(16, 16, max(1, 6-f/2), max(1, 6-f/2), 7, a)
		line(detail, 4, 3+f, 7, 7+f, 6, a)
		line(detail, 25, 27-f, 28, 30-f, 6, a)
	case "magic-projectile":
		y := 16 + (f % 4) - 2
		polygon(body, []image.Point{{2, y - 4}, {20, y - 7}, {28, y - 3}, {29, y + 3}, {21, y + 8}, {2, y + 3}, {11, y}}, 8, 220)
		ellipse(body, 22, y, 7, 7, 9, 255)
		glowEllipse(23, y-1, 4, 4, 10, 255)
		glowEllipse(24, y-2, 2, 2, 7, 255)
		line(detail, 2+f%3, y-2, 15, y-2, 10, 220)
		line(emission, 5, y+2, 18-f%3, y+2, 9, 220)
		line(detail, 9, y+7, 13, y+8, 9, 190)
	case "trail":
		y := 16 + (f % 4) - 2
		polygon(body, []image.Point{{2, y + 1}, {10, y - 4}, {20, y - 5}, {29, y - 2}, {28, y + 5}, {20, y + 3}, {11, y + 2}}, 8, 190)
		polygon(body, []image.Point{{3, y + 1}, {12, y - 2}, {23, y - 3}, {28, y - 1}, {27, y + 3}, {18, y + 1}}, 9, 230)
		glowEllipse(26, y, 2, 2, 10, 255)
		line(detail, 9, y, 25, y-1, 10, 240)
		line(emission, 13, y, 26, y-1, 10, 240)
		line(detail, 4+f%3, y+5, 10+f%3, y+5, 9, 150)
	default:
		t.Fatalf("missing artwork for %q", template)
	}
}
