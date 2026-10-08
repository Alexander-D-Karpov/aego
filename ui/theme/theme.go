package theme

import "aego/core/mathx"

type Theme struct {
	Text         mathx.Color
	TextDim      mathx.Color
	TextDisabled mathx.Color
	Background   mathx.Color
	Panel        mathx.Color
	PanelAlt     mathx.Color
	Header       mathx.Color
	Widget       mathx.Color
	WidgetHot    mathx.Color
	WidgetDown   mathx.Color
	Accent       mathx.Color
	AccentDim    mathx.Color
	Selection    mathx.Color
	SelectionDim mathx.Color
	Border       mathx.Color
	Separator    mathx.Color
	Error        mathx.Color
	Warn         mathx.Color
	Ok           mathx.Color
	DropTarget   mathx.Color
	Overlay      mathx.Color
	Tab          mathx.Color
	TabHot       mathx.Color
	TabActive    mathx.Color
	AxisX        mathx.Color
	AxisY        mathx.Color
	AxisZ        mathx.Color
	AxisHot      mathx.Color
	PlayTint     mathx.Color
	CardBg       mathx.Color
	CardHeader   mathx.Color
	Badge        mathx.Color
	Unit         mathx.Color
	BarBg        mathx.Color
	BarBorder    mathx.Color
	ButtonHover  mathx.Color
	ButtonActive mathx.Color

	Padding    float32
	BarHeight  float32
	RowHeight  float32
	Spacing    float32
	Indent     float32
	ScrollbarW float32
	SplitterW  float32
	TabHeight  float32
	MenuHeight float32
	FontSize   float32

	Mixed string
}

func Default() Theme {
	t := Editor()
	t.Text = mathx.ColorRGB8(0xEA, 0xEA, 0xEA)
	t.TextDim = mathx.ColorRGB8(0x9A, 0x9A, 0x9A)
	t.Panel = mathx.Color{R: 0.02, G: 0.02, B: 0.03, A: 0.78}
	t.Widget = mathx.Color{R: 0.16, G: 0.17, B: 0.20, A: 0.95}
	t.WidgetHot = mathx.Color{R: 0.24, G: 0.26, B: 0.31, A: 0.95}
	t.WidgetDown = mathx.Color{R: 0.10, G: 0.11, B: 0.14, A: 0.95}
	t.Accent = mathx.ColorRGB8(0xE0, 0x7B, 0x39)
	t.Border = mathx.Color{R: 1, G: 1, B: 1, A: 0.12}
	t.Padding = 6
	t.RowHeight = 16
	t.Spacing = 4
	t.FontSize = 13
	return t
}

func Editor() Theme {
	return Theme{
		Text:         mathx.ColorRGB8(0xE4, 0xE6, 0xEB),
		TextDim:      mathx.ColorRGB8(0x93, 0x98, 0xA3),
		TextDisabled: mathx.ColorRGB8(0x5C, 0x61, 0x6B),
		Background:   mathx.ColorRGB8(0x15, 0x17, 0x1B),
		Panel:        mathx.ColorRGB8(0x1E, 0x21, 0x26),
		PanelAlt:     mathx.ColorRGB8(0x23, 0x26, 0x2C),
		Header:       mathx.ColorRGB8(0x18, 0x1A, 0x1E),
		Widget:       mathx.ColorRGB8(0x2B, 0x2F, 0x36),
		WidgetHot:    mathx.ColorRGB8(0x36, 0x3B, 0x44),
		WidgetDown:   mathx.ColorRGB8(0x22, 0x25, 0x2B),
		Accent:       mathx.ColorRGB8(0xE0, 0x7B, 0x39),
		AccentDim:    mathx.Color{R: 0.75, G: 0.2, B: 0.05, A: 0.5},
		Selection:    mathx.ColorRGB8(0x2F, 0x5A, 0x8C),
		SelectionDim: mathx.ColorRGB8(0x2A, 0x3A, 0x4F),
		Border:       mathx.Color{R: 1, G: 1, B: 1, A: 0.08},
		Separator:    mathx.Color{R: 1, G: 1, B: 1, A: 0.06},
		Error:        mathx.ColorRGB8(0xE5, 0x53, 0x4B),
		Warn:         mathx.ColorRGB8(0xE5, 0xB5, 0x4B),
		Ok:           mathx.ColorRGB8(0x6B, 0xC4, 0x6D),
		DropTarget:   mathx.Color{R: 0.2, G: 0.35, B: 0.6, A: 0.45},
		Overlay:      mathx.Color{A: 0.45},
		Tab:          mathx.ColorRGB8(0x1B, 0x1D, 0x22),
		TabHot:       mathx.ColorRGB8(0x26, 0x29, 0x30),
		TabActive:    mathx.ColorRGB8(0x1E, 0x21, 0x26),
		AxisX:        mathx.ColorRGB8(0xE5, 0x53, 0x4B),
		AxisY:        mathx.ColorRGB8(0x6B, 0xC4, 0x6D),
		AxisZ:        mathx.ColorRGB8(0x4B, 0x8B, 0xE5),
		AxisHot:      mathx.ColorRGB8(0xF5, 0xD7, 0x4B),
		PlayTint:     mathx.Color{R: 0.88, G: 0.48, B: 0.22, A: 0.85},
		CardBg:       mathx.ColorRGB8(0x22, 0x25, 0x2B),
		CardHeader:   mathx.ColorRGB8(0x29, 0x2D, 0x34),
		Badge:        mathx.ColorRGB8(0x34, 0x38, 0x40),
		Unit:         mathx.ColorRGB8(0x7A, 0x80, 0x8C),
		BarBg:        mathx.ColorRGB8(0x1A, 0x1C, 0x21),
		BarBorder:    mathx.Color{R: 1, G: 1, B: 1, A: 0.06},
		ButtonHover:  mathx.ColorRGB8(0x30, 0x34, 0x3C),
		ButtonActive: mathx.ColorRGB8(0x2F, 0x5A, 0x8C),
		BarHeight:    28,
		Padding:      6,
		RowHeight:    22,
		Spacing:      2,
		Indent:       14,
		ScrollbarW:   8,
		SplitterW:    4,
		TabHeight:    26,
		MenuHeight:   24,
		FontSize:     14,
		Mixed:        "—",
	}
}
