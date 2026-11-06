package theme

import "aego/core/mathx"

// LightCode is the code editor palette of the light theme.
func LightCode() CodeTheme {
	bg := mathx.ColorRGB8(0xFA, 0xFB, 0xFC)
	black := mathx.Color{A: 1}
	blue := mathx.ColorRGB8(0x33, 0x66, 0xD6)
	warm := mathx.ColorRGB8(0xC2, 0x5E, 0x1E)
	rainbow := [6]mathx.Color{
		mathx.ColorRGB8(0xA0, 0x66, 0x00),
		mathx.ColorRGB8(0xA6, 0x26, 0xA4),
		mathx.ColorRGB8(0x33, 0x66, 0xD6),
		mathx.ColorRGB8(0x38, 0x7F, 0x37),
		mathx.ColorRGB8(0x98, 0x68, 0x01),
		mathx.ColorRGB8(0x0A, 0x76, 0xA8),
	}
	return CodeTheme{
		Background:         bg,
		GutterBg:           mathx.ColorRGB8(0xF3, 0xF4, 0xF6),
		Gutter:             GutterMetrics{MarkerW: 1, NumberPad: 1, CodePad: 1},
		LineNo:             mathx.ColorRGB8(0x9A, 0xA1, 0xAB),
		LineNoActive:       mathx.ColorRGB8(0x3B, 0x42, 0x4C),
		CurrentLine:        Mix(bg, black, 0.04),
		CurrentBlock:       Mix(bg, black, 0.015),
		Guide:              Mix(bg, black, 0.06),
		IndentGuide:        Mix(bg, black, 0.08),
		IndentGuideActive:  Mix(bg, black, 0.2),
		GuideRainbow:       mixAll(bg, rainbow, 0.2),
		GuideRainbowActive: mixAll(bg, rainbow, 0.45),
		BracketMatch:       mathx.ColorRGB8(0xA0, 0x66, 0x00),
		BracketMismatch:    mathx.ColorRGB8(0xC9, 0x37, 0x2C),
		Rainbow:            rainbow,
		Occurrence:         Mix(bg, blue, 0.16),
		OccurrenceDim:      Mix(bg, blue, 0.08),
		OccurrenceWrite:    Mix(bg, warm, 0.25),
		Plain:              mathx.ColorRGB8(0x24, 0x29, 0x2F),
		Keyword:            mathx.ColorRGB8(0xA6, 0x26, 0xA4),
		Type:               mathx.ColorRGB8(0x0A, 0x76, 0xA8),
		BuiltinType:        mathx.ColorRGB8(0x0A, 0x76, 0xA8),
		Function:           mathx.ColorRGB8(0x33, 0x66, 0xD6),
		Method:             mathx.ColorRGB8(0x33, 0x66, 0xD6),
		Field:              mathx.ColorRGB8(0xC8, 0x44, 0x3A),
		Property:           mathx.ColorRGB8(0xC8, 0x44, 0x3A),
		Param:              mathx.ColorRGB8(0xB0, 0x58, 0x2C),
		Local:              mathx.ColorRGB8(0x24, 0x29, 0x2F),
		Const:              mathx.ColorRGB8(0x98, 0x68, 0x01),
		EnumMember:         mathx.ColorRGB8(0x98, 0x68, 0x01),
		String:             mathx.ColorRGB8(0x38, 0x7F, 0x37),
		Number:             mathx.ColorRGB8(0x98, 0x68, 0x01),
		Comment:            mathx.ColorRGB8(0x8A, 0x8F, 0x98),
		Attribute:          mathx.ColorRGB8(0xA0, 0x66, 0x00),
		Namespace:          mathx.ColorRGB8(0xB0, 0x53, 0x1A),
		Script:             mathx.ColorRGB8(0x0A, 0x76, 0xA8),
		Signal:             mathx.ColorRGB8(0xB4, 0x56, 0x1D),
		Coroutine:          mathx.ColorRGB8(0x0B, 0x7A, 0x71),
		Builtin:            mathx.ColorRGB8(0xCA, 0x12, 0x43),
		Operator:           mathx.ColorRGB8(0x4A, 0x50, 0x59),
		Punctuation:        mathx.ColorRGB8(0x6A, 0x71, 0x7C),
		Error:              mathx.ColorRGB8(0xC9, 0x37, 0x2C),
		MinimapView:        mathx.Color{A: 0.07},
		GuideColumn:        100,
	}
}

// Light is the light editor theme; the game viewport keeps its own dark frame.
func Light() Theme {
	t := Editor()
	t.Text = mathx.ColorRGB8(0x1F, 0x23, 0x29)
	t.TextDim = mathx.ColorRGB8(0x5E, 0x66, 0x72)
	t.TextDisabled = mathx.ColorRGB8(0x9C, 0xA3, 0xAD)
	t.Background = mathx.ColorRGB8(0xDC, 0xDF, 0xE4)
	t.Panel = mathx.ColorRGB8(0xF3, 0xF4, 0xF6)
	t.PanelAlt = mathx.ColorRGB8(0xEB, 0xED, 0xF0)
	t.Header = mathx.ColorRGB8(0xE4, 0xE6, 0xEA)
	t.Widget = mathx.ColorRGB8(0xE1, 0xE4, 0xE8)
	t.WidgetHot = mathx.ColorRGB8(0xD5, 0xD9, 0xDF)
	t.WidgetDown = mathx.ColorRGB8(0xC9, 0xCE, 0xD6)
	t.Accent = mathx.ColorRGB8(0xC2, 0x5E, 0x1E)
	t.AccentDim = mathx.Color{R: 0.75, G: 0.3, B: 0.1, A: 0.35}
	t.Selection = mathx.ColorRGB8(0xB9, 0xD3, 0xF2)
	t.SelectionDim = mathx.ColorRGB8(0xDC, 0xE7, 0xF5)
	t.Border = mathx.Color{A: 0.12}
	t.Separator = mathx.Color{A: 0.08}
	t.Error = mathx.ColorRGB8(0xC9, 0x37, 0x2C)
	t.Warn = mathx.ColorRGB8(0xA8, 0x6D, 0x12)
	t.Ok = mathx.ColorRGB8(0x2E, 0x8B, 0x3E)
	t.DropTarget = mathx.Color{R: 0.2, G: 0.45, B: 0.85, A: 0.25}
	t.Overlay = mathx.Color{A: 0.25}
	t.Tab = mathx.ColorRGB8(0xE4, 0xE6, 0xEA)
	t.TabHot = mathx.ColorRGB8(0xED, 0xEF, 0xF2)
	t.TabActive = mathx.ColorRGB8(0xF3, 0xF4, 0xF6)
	t.AxisX = mathx.ColorRGB8(0xD2, 0x38, 0x2E)
	t.AxisY = mathx.ColorRGB8(0x2E, 0x9D, 0x41)
	t.AxisZ = mathx.ColorRGB8(0x2F, 0x6F, 0xD6)
	t.AxisHot = mathx.ColorRGB8(0xD9, 0xA2, 0x00)
	t.CardBg = mathx.ColorRGB8(0xFA, 0xFB, 0xFC)
	t.CardHeader = mathx.ColorRGB8(0xEE, 0xF0, 0xF3)
	t.Badge = mathx.ColorRGB8(0xDD, 0xE1, 0xE6)
	t.Unit = mathx.ColorRGB8(0x6E, 0x76, 0x82)
	t.BarBg = mathx.ColorRGB8(0xE6, 0xE8, 0xEB)
	t.BarBorder = mathx.Color{A: 0.08}
	t.ButtonHover = mathx.ColorRGB8(0xE9, 0xEC, 0xF0)
	t.ButtonActive = mathx.ColorRGB8(0xA9, 0xC8, 0xEE)
	t.Code = LightCode()
	t.Shadow = mathx.Color{A: 0.18}
	t.TreeGuide = mathx.Color{A: 0.1}
	t.Highlight = mathx.Color{A: 0.06}
	t.FieldGizmo = mathx.ColorRGB8(0x2F, 0x6F, 0xD6)
	t.Record = mathx.ColorRGB8(0xD2, 0x38, 0x2E)
	t.StateEntry = mathx.ColorRGB8(0x2E, 0x8B, 0x3E)
	t.StateAny = mathx.ColorRGB8(0x7A, 0x5C, 0xC4)
	t.Series = [8]mathx.Color{
		mathx.ColorRGB8(0x2F, 0x6F, 0xD6),
		mathx.ColorRGB8(0x2E, 0x8B, 0x3E),
		mathx.ColorRGB8(0xC2, 0x5E, 0x1E),
		mathx.ColorRGB8(0x7A, 0x4C, 0xB8),
		mathx.ColorRGB8(0x0B, 0x7A, 0x71),
		mathx.ColorRGB8(0xB8, 0x3E, 0x78),
		mathx.ColorRGB8(0x98, 0x7A, 0x00),
		mathx.ColorRGB8(0x5A, 0x6B, 0x82),
	}
	t.Events = [3]mathx.Color{
		mathx.ColorRGB8(0xC2, 0x5E, 0x1E),
		mathx.ColorRGB8(0x2F, 0x6F, 0xD6),
		mathx.ColorRGB8(0x2E, 0x8B, 0x3E),
	}
	t.Dark = false
	t.Agent = agentLight()
	t.SceneBg = mathx.ColorRGB8(0xDC, 0xE1, 0xE7)
	t.SceneText = mathx.ColorRGB8(0x45, 0x50, 0x5E)
	t.SceneText.A = 0.8
	t.SceneLine = mathx.ColorRGB8(0x38, 0x43, 0x50)
	t.GridMinor = mathx.Color{A: 0.09}
	t.GridMajor = mathx.Color{A: 0.17}
	return t
}
