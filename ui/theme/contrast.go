package theme

import "aego/core/mathx"

// Over is c drawn with its alpha over the opaque bg.
func Over(c, bg mathx.Color) mathx.Color {
	a := c.A
	return mathx.Color{R: c.R*a + bg.R*(1-a), G: c.G*a + bg.G*(1-a), B: c.B*a + bg.B*(1-a), A: 1}
}

// Luminance is the relative luminance of a linear colour.
func Luminance(c mathx.Color) float32 {
	return 0.2126*c.R + 0.7152*c.G + 0.0722*c.B
}

// Contrast is the WCAG contrast ratio of fg over bg.
func Contrast(fg, bg mathx.Color) float32 {
	a, b := Luminance(Over(fg, bg)), Luminance(bg)
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}
