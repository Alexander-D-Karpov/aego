package theme

import (
	"strings"

	"aego/core/mathx"
)

type GutterMetrics struct {
	MarkerW   float32
	NumberPad float32
	CodePad   float32
}

type CodeTheme struct {
	Background         mathx.Color
	GutterBg           mathx.Color
	Gutter             GutterMetrics
	LineNo             mathx.Color
	LineNoActive       mathx.Color
	CurrentLine        mathx.Color
	CurrentBlock       mathx.Color
	Guide              mathx.Color
	IndentGuide        mathx.Color
	IndentGuideActive  mathx.Color
	GuideRainbow       [6]mathx.Color
	GuideRainbowActive [6]mathx.Color
	BracketMatch       mathx.Color
	BracketMismatch    mathx.Color
	Rainbow            [6]mathx.Color
	Occurrence         mathx.Color
	OccurrenceDim      mathx.Color
	OccurrenceWrite    mathx.Color
	Plain              mathx.Color
	Keyword            mathx.Color
	Type               mathx.Color
	BuiltinType        mathx.Color
	Function           mathx.Color
	Method             mathx.Color
	Field              mathx.Color
	Property           mathx.Color
	Param              mathx.Color
	Local              mathx.Color
	Const              mathx.Color
	EnumMember         mathx.Color
	String             mathx.Color
	Number             mathx.Color
	Comment            mathx.Color
	Attribute          mathx.Color
	Namespace          mathx.Color
	Script             mathx.Color
	Signal             mathx.Color
	Coroutine          mathx.Color
	Builtin            mathx.Color
	Operator           mathx.Color
	Punctuation        mathx.Color
	Error              mathx.Color
	// MinimapView frames the visible part of the file on the minimap.
	MinimapView mathx.Color
	GuideColumn int
}

type Radius struct {
	Button float32
	Field  float32
	Menu   float32
	Popup  float32
	Tab    float32
	Card   float32
}

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
	Code         CodeTheme
	Radius       Radius
	Shadow       mathx.Color
	ShadowBlur   float32
	ShadowY      float32
	TreeGuide    mathx.Color
	// Highlight marks the hovered column of a chart.
	Highlight  mathx.Color
	FieldGizmo mathx.Color
	Record     mathx.Color
	StateEntry mathx.Color
	StateAny   mathx.Color
	// Series colours the lines and bars of charts, in order.
	Series [8]mathx.Color
	// Events colours animation events by mode: gameplay, presentation, preview.
	Events [3]mathx.Color
	// Agent colours the Agent panel: the user's messages, tool steps and the two sides of a change (11b5a).
	Agent AgentTheme
	// Viewport colours stay the same in every theme, like the scene they frame (D345).
	ViewportBg   mathx.Color
	SceneBg      mathx.Color
	ViewportText mathx.Color
	ViewportLine mathx.Color
	GridMinor    mathx.Color
	GridMajor    mathx.Color
	Dark         bool

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
	t.Radius = Radius{}
	t.Shadow = mathx.Color{}
	return t
}

func Mix(a, b mathx.Color, t float32) mathx.Color {
	ch := func(x, y float32) float32 {
		s := mathx.LinearToSRGB(mathx.Clamp(x, 0, 1))
		d := mathx.LinearToSRGB(mathx.Clamp(y, 0, 1))
		return mathx.SRGBToLinear(s + (d-s)*t)
	}
	return mathx.Color{R: ch(a.R, b.R), G: ch(a.G, b.G), B: ch(a.B, b.B), A: 1}
}

func mixAll(bg mathx.Color, cs [6]mathx.Color, t float32) [6]mathx.Color {
	var out [6]mathx.Color
	for i, c := range cs {
		out[i] = Mix(bg, c, t)
	}
	return out
}

func Code() CodeTheme {
	bg := mathx.ColorRGB8(0x1A, 0x1D, 0x22)
	white := mathx.Color{R: 1, G: 1, B: 1, A: 1}
	blue := mathx.ColorRGB8(0x4D, 0x78, 0xB8)
	warm := mathx.ColorRGB8(0xE5, 0x9B, 0x4B)
	rainbow := [6]mathx.Color{
		mathx.ColorRGB8(0xE5, 0xC0, 0x7B),
		mathx.ColorRGB8(0xC6, 0x78, 0xDD),
		mathx.ColorRGB8(0x61, 0xAF, 0xEF),
		mathx.ColorRGB8(0x98, 0xC3, 0x79),
		mathx.ColorRGB8(0xD1, 0x9A, 0x66),
		mathx.ColorRGB8(0x56, 0xB6, 0xC2),
	}
	return CodeTheme{
		Background:         bg,
		GutterBg:           mathx.ColorRGB8(0x17, 0x19, 0x1D),
		Gutter:             GutterMetrics{MarkerW: 1, NumberPad: 1, CodePad: 1},
		LineNo:             mathx.ColorRGB8(0x5C, 0x61, 0x6B),
		LineNoActive:       mathx.ColorRGB8(0xB0, 0xB5, 0xBF),
		CurrentLine:        Mix(bg, white, 0.035),
		CurrentBlock:       Mix(bg, white, 0.012),
		Guide:              Mix(bg, white, 0.05),
		IndentGuide:        Mix(bg, white, 0.07),
		IndentGuideActive:  Mix(bg, white, 0.16),
		GuideRainbow:       mixAll(bg, rainbow, 0.18),
		GuideRainbowActive: mixAll(bg, rainbow, 0.4),
		BracketMatch:       mathx.ColorRGB8(0xE5, 0xC0, 0x7B),
		BracketMismatch:    mathx.ColorRGB8(0xE5, 0x53, 0x4B),
		Rainbow:            rainbow,
		Occurrence:         Mix(bg, blue, 0.22),
		OccurrenceDim:      Mix(bg, blue, 0.12),
		OccurrenceWrite:    Mix(bg, warm, 0.55),
		Plain:              mathx.ColorRGB8(0xD8, 0xDB, 0xE2),
		Keyword:            mathx.ColorRGB8(0xC6, 0x78, 0xDD),
		Type:               mathx.ColorRGB8(0x56, 0xB6, 0xC2),
		BuiltinType:        mathx.ColorRGB8(0x56, 0xB6, 0xC2),
		Function:           mathx.ColorRGB8(0x61, 0xAF, 0xEF),
		Method:             mathx.ColorRGB8(0x61, 0xAF, 0xEF),
		Field:              mathx.ColorRGB8(0xE0, 0x6C, 0x75),
		Property:           mathx.ColorRGB8(0xE0, 0x6C, 0x75),
		Param:              mathx.ColorRGB8(0xE8, 0xA8, 0x7C),
		Local:              mathx.ColorRGB8(0xD8, 0xDB, 0xE2),
		Const:              mathx.ColorRGB8(0xD1, 0x9A, 0x66),
		EnumMember:         mathx.ColorRGB8(0xD1, 0x9A, 0x66),
		String:             mathx.ColorRGB8(0x98, 0xC3, 0x79),
		Number:             mathx.ColorRGB8(0xD1, 0x9A, 0x66),
		Comment:            mathx.ColorRGB8(0x6B, 0x72, 0x80),
		Attribute:          mathx.ColorRGB8(0xE5, 0xC0, 0x7B),
		Namespace:          mathx.ColorRGB8(0xE0, 0x7B, 0x39),
		Script:             mathx.ColorRGB8(0x56, 0xB6, 0xC2),
		Signal:             mathx.ColorRGB8(0xF2, 0x9E, 0x74),
		Coroutine:          mathx.ColorRGB8(0x7F, 0xCF, 0xC9),
		Builtin:            mathx.ColorRGB8(0xE5, 0x8E, 0xC6),
		Operator:           mathx.ColorRGB8(0xAB, 0xB2, 0xBF),
		Punctuation:        mathx.ColorRGB8(0x8B, 0x92, 0x9E),
		Error:              mathx.ColorRGB8(0xE5, 0x53, 0x4B),
		MinimapView:        mathx.Color{R: 1, G: 1, B: 1, A: 0.06},
		GuideColumn:        100,
	}
}

func Editor() Theme {
	t := Theme{
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
		Code:         Code(),
		Radius:       Radius{Button: 4, Field: 4, Menu: 4, Popup: 7, Tab: 5, Card: 6},
		Shadow:       mathx.Color{A: 0.45},
		ShadowBlur:   12,
		ShadowY:      4,
		TreeGuide:    mathx.Color{R: 1, G: 1, B: 1, A: 0.07},
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
	t.Highlight = mathx.Color{R: 1, G: 1, B: 1, A: 0.08}
	t.FieldGizmo = mathx.Color{R: 0.35, G: 0.55, B: 1, A: 1}
	t.Record = mathx.Color{R: 0.9, G: 0.25, B: 0.25, A: 1}
	t.StateEntry = mathx.Color{R: 0.3, G: 0.7, B: 0.4, A: 1}
	t.StateAny = mathx.Color{R: 0.55, G: 0.5, B: 0.8, A: 1}
	t.Series = series()
	t.Events = [3]mathx.Color{{R: 0.95, G: 0.6, B: 0.2, A: 1}, {R: 0.4, G: 0.7, B: 1, A: 1}, {R: 0.6, G: 0.9, B: 0.4, A: 1}}
	t.Dark = true
	t.Agent = agentDark()
	viewport(&t)
	return t
}

type AgentTheme struct {
	User       mathx.Color
	Tool       mathx.Color
	DiffAdd    mathx.Color
	DiffRemove mathx.Color
	// Presence marks where the agent works: its avatar and the second selection outline.
	Presence mathx.Color
}

func agentDark() AgentTheme {
	return AgentTheme{
		User:       mathx.ColorRGB8(0x2A, 0x33, 0x40),
		Tool:       mathx.ColorRGB8(0x22, 0x25, 0x2B),
		DiffAdd:    mathx.ColorRGB8(0x7C, 0xC9, 0x7E),
		DiffRemove: mathx.ColorRGB8(0xE5, 0x86, 0x7E),
		Presence:   mathx.ColorRGB8(0xB3, 0x8D, 0xF5),
	}
}

func agentLight() AgentTheme {
	return AgentTheme{
		User:       mathx.ColorRGB8(0xE2, 0xEA, 0xF5),
		Tool:       mathx.ColorRGB8(0xEB, 0xED, 0xF0),
		DiffAdd:    mathx.ColorRGB8(0x23, 0x7A, 0x33),
		DiffRemove: mathx.ColorRGB8(0xB3, 0x30, 0x26),
		Presence:   mathx.ColorRGB8(0x6E, 0x47, 0xC2),
	}
}

func series() [8]mathx.Color {
	return [8]mathx.Color{
		mathx.ColorRGB8(0x4B, 0x8B, 0xE5),
		mathx.ColorRGB8(0x6B, 0xC4, 0x6D),
		mathx.ColorRGB8(0xE0, 0x7B, 0x39),
		mathx.ColorRGB8(0xA0, 0x6B, 0xD5),
		mathx.ColorRGB8(0x3F, 0xB6, 0xB0),
		mathx.ColorRGB8(0xD5, 0x6B, 0x9A),
		mathx.ColorRGB8(0xC9, 0xB0, 0x3F),
		mathx.ColorRGB8(0x7A, 0x8C, 0xA3),
	}
}

// viewport sets the colours of what frames the scene; they are the same in every theme.
func viewport(t *Theme) {
	t.ViewportBg = mathx.Color{R: 0.02, G: 0.02, B: 0.03, A: 1}
	t.SceneBg = mathx.ColorRGB8(0x15, 0x17, 0x1B)
	t.ViewportText = mathx.ColorRGB8(0x93, 0x98, 0xA3)
	t.ViewportText.A = 0.7
	t.ViewportLine = mathx.ColorRGB8(0xE4, 0xE6, 0xEB)
	t.GridMinor = mathx.Color{R: 1, G: 1, B: 1, A: 0.018}
	t.GridMajor = mathx.Color{R: 1, G: 1, B: 1, A: 0.045}
}

// Mode is the theme the user picked; System follows the OS (D345).
type Mode uint8

const (
	ModeSystem Mode = iota
	ModeDark
	ModeLight
)

var modeNames = [...]string{"System", "Dark", "Light"}

func (m Mode) String() string {
	if int(m) < len(modeNames) {
		return modeNames[m]
	}
	return modeNames[0]
}

func ParseMode(s string) Mode {
	for i, n := range modeNames {
		if strings.EqualFold(n, s) {
			return Mode(i)
		}
	}
	return ModeSystem
}

// For is the theme of a mode, given whether the OS is dark.
func For(m Mode, systemDark bool) Theme {
	if m == ModeLight || (m == ModeSystem && !systemDark) {
		return Light()
	}
	return Editor()
}
