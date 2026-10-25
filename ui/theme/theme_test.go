package theme

import (
	"reflect"
	"testing"

	"aego/core/mathx"
)

func TestCodeThemeComplete(t *testing.T) {
	colorT := reflect.TypeOf(mathx.Color{})
	for name, th := range map[string]Theme{"default": Default(), "editor": Editor(), "light": Light()} {
		v := reflect.ValueOf(th.Code)
		for i := 0; i < v.NumField(); i++ {
			f, field := v.Field(i), v.Type().Field(i)
			switch {
			case f.Type() == colorT:
				if f.Interface().(mathx.Color).A == 0 {
					t.Errorf("%s: Code.%s is empty", name, field.Name)
				}
			case f.Kind() == reflect.Array && f.Type().Elem() == colorT:
				for j := 0; j < f.Len(); j++ {
					if f.Index(j).Interface().(mathx.Color).A == 0 {
						t.Errorf("%s: Code.%s[%d] is empty", name, field.Name, j)
					}
				}
			case f.Kind() == reflect.Float32:
				if f.Float() == 0 {
					t.Errorf("%s: Code.%s is zero", name, field.Name)
				}
			}
		}
	}
}

type pair struct {
	name   string
	fg, bg mathx.Color
	min    float32
}

func pairs(t Theme) []pair {
	c := t.Code
	out := []pair{
		{"Text/Panel", t.Text, t.Panel, 7},
		{"Text/PanelAlt", t.Text, t.PanelAlt, 7},
		{"Text/CardBg", t.Text, t.CardBg, 7},
		{"Text/Background", t.Text, t.Background, 7},
		{"Text/Selection", t.Text, t.Selection, 4.5},
		{"Text/ButtonActive", t.Text, t.ButtonActive, 4.5},
		{"Text/Widget", t.Text, t.Widget, 4.5},
		{"TextDim/Panel", t.TextDim, t.Panel, 4.5},
		{"TextDim/CardBg", t.TextDim, t.CardBg, 4.5},
		{"TextDisabled/Panel", t.TextDisabled, t.Panel, 2},
		{"Unit/CardBg", t.Unit, t.CardBg, 3},
		{"Accent/Panel", t.Accent, t.Panel, 3},
		{"Error/Panel", t.Error, t.Panel, 3},
		{"Warn/Panel", t.Warn, t.Panel, 3},
		{"Ok/Panel", t.Ok, t.Panel, 3},
		{"Border/Panel", t.Border, t.Panel, 1.1},
		{"LineNoActive/Gutter", c.LineNoActive, c.GutterBg, 4.5},
		{"LineNo/Gutter", c.LineNo, c.GutterBg, 2},
		{"Comment/Code", c.Comment, c.Background, 3},
	}
	for _, x := range []struct {
		name string
		col  mathx.Color
	}{
		{"Plain", c.Plain}, {"Keyword", c.Keyword}, {"Type", c.Type}, {"Function", c.Function},
		{"Field", c.Field}, {"Param", c.Param}, {"Const", c.Const}, {"String", c.String},
		{"Number", c.Number}, {"Attribute", c.Attribute}, {"Namespace", c.Namespace},
		{"Signal", c.Signal}, {"Coroutine", c.Coroutine}, {"Builtin", c.Builtin},
		{"Operator", c.Operator}, {"Punctuation", c.Punctuation}, {"Error", c.Error},
	} {
		out = append(out, pair{x.name + "/Code", x.col, c.Background, 4.5})
	}
	for i, s := range t.Series {
		out = append(out, pair{"Series" + string(rune('0'+i)) + "/Panel", s, t.Panel, 2.5})
	}
	return out
}

// Both themes keep text and syntax colours readable by WCAG ratios.
func TestThemeContrast(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   Theme
	}{{"dark", Editor()}, {"light", Light()}} {
		for _, p := range pairs(tc.th) {
			if got := Contrast(p.fg, p.bg); got < p.min {
				t.Errorf("%s %s: contrast %.2f, want at least %.1f", tc.name, p.name, got, p.min)
			}
		}
	}
}

func TestModeRoundTrip(t *testing.T) {
	for _, m := range []Mode{ModeSystem, ModeDark, ModeLight} {
		if ParseMode(m.String()) != m {
			t.Fatalf("%v does not round-trip", m)
		}
	}
	if !For(ModeSystem, true).Dark || For(ModeSystem, false).Dark || !For(ModeDark, false).Dark || For(ModeLight, true).Dark {
		t.Fatal("For picks the wrong theme")
	}
}
