package settings

import (
	"strconv"
	"strings"
	"unicode"

	"aego/core/mathx"
	"aego/schema"
)

const (
	ServiceID      = "engine.settings"
	PresetCustom   = "Custom"
	DefaultConfirm = 15
)

type Flags uint16

const (
	User Flags = 1 << iota
	Project
	Restart
	Transient
	Accessibility
	Confirm
	Controls
)

func (f Flags) Has(o Flags) bool {
	return f&o == o
}

const (
	CategoryDisplay       = "Display"
	CategoryGraphics      = "Graphics"
	CategoryAudio         = "Audio"
	CategoryInput         = "Input"
	CategoryAccessibility = "Accessibility"
	CategoryGameplay      = "Gameplay"
	CategoryLanguage      = "Language"
)

var Categories = []string{CategoryDisplay, CategoryGraphics, CategoryAudio, CategoryInput, CategoryAccessibility, CategoryGameplay, CategoryLanguage}

type Layer uint8

const (
	LayerDefault Layer = iota
	LayerProject
	LayerUser
	LayerSession
)

var layerNames = [...]string{"default", "project", "user", "session"}

func (l Layer) String() string {
	if int(l) < len(layerNames) {
		return layerNames[l]
	}
	return ""
}

type Choice struct {
	Name  string
	Label string
	Value schema.Value
}

type Desc struct {
	ID          string
	Owner       string
	Kind        schema.Kind
	Default     schema.Value
	Title       string
	Doc         string
	Category    string
	Order       int
	Flags       Flags
	Min, Max    float64
	HasMin      bool
	HasMax      bool
	Step        float64
	Unit        string
	Choices     []Choice
	ChoicesFrom schema.HintSource
	ChoicesWhen func(*Store) []Choice
	EnabledWhen func(*Store) bool
	Validate    func(schema.Value) error
	Supports    func(schema.Value) bool
	Apply       func(schema.Value) error
	Presets     map[string]schema.Value
	Preset      bool
}

func (d *Desc) Path() string {
	return strings.TrimPrefix(d.ID, d.Owner+".")
}

func (d *Desc) Label() string {
	if d.Title != "" {
		return d.Title
	}
	p := d.Path()
	if i := strings.LastIndexByte(p, '.'); i >= 0 {
		p = p[i+1:]
	}
	return humanize(p)
}

func humanize(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case i == 0:
			b.WriteRune(unicode.ToUpper(r))
		case unicode.IsUpper(r):
			b.WriteByte(' ')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ValidPath(id string) bool {
	if id == "" {
		return false
	}
	for _, seg := range strings.Split(id, ".") {
		if seg == "" {
			return false
		}
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			switch {
			case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			case c >= '0' && c <= '9' && i > 0:
			default:
				return false
			}
		}
	}
	return true
}

func supported(k schema.Kind) bool {
	switch k {
	case schema.KindBool, schema.KindInt64, schema.KindFloat64, schema.KindString,
		schema.KindEnum, schema.KindVec2, schema.KindColor, schema.KindList:
		return true
	}
	return false
}

func isInt(k schema.Kind) bool {
	return k == schema.KindInt32 || k == schema.KindInt64 || k == schema.KindEnum
}

func isFloat(k schema.Kind) bool {
	return k == schema.KindFloat32 || k == schema.KindFloat64
}

func numOf(v schema.Value) float64 {
	if isFloat(v.Kind) {
		return v.F
	}
	return float64(v.I)
}

func Normalize(k schema.Kind, v schema.Value) (schema.Value, bool) {
	switch k {
	case schema.KindBool:
		return schema.Bool(v.I != 0), v.Kind == schema.KindBool
	case schema.KindInt64:
		if isInt(v.Kind) {
			return schema.Int64(v.I), true
		}
	case schema.KindEnum:
		if isInt(v.Kind) {
			return schema.Enum(v.I), true
		}
	case schema.KindFloat64:
		if isFloat(v.Kind) || isInt(v.Kind) {
			return schema.Float64(numOf(v)), true
		}
	case schema.KindList:
		if v.Kind != schema.KindList {
			return v, false
		}
		for _, x := range v.List {
			if x.Kind != schema.KindString {
				return v, false
			}
		}
		return v, true
	default:
		return v, v.Kind == k
	}
	return v, false
}

func Equal(a, b schema.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case schema.KindFloat32, schema.KindFloat64:
		return a.F == b.F
	case schema.KindString:
		return a.S == b.S
	case schema.KindVec2:
		return a.V == b.V
	case schema.KindColor:
		return a.C == b.C
	case schema.KindList:
		if len(a.List) != len(b.List) {
			return false
		}
		for i := range a.List {
			if !Equal(a.List[i], b.List[i]) {
				return false
			}
		}
		return true
	}
	return a.I == b.I
}

func Strings(items ...string) schema.Value {
	out := schema.Value{Kind: schema.KindList, List: make([]schema.Value, len(items))}
	for i, s := range items {
		out.List[i] = schema.String(s)
	}
	return out
}

func clone(v schema.Value) schema.Value {
	if v.Kind == schema.KindList {
		v.List = append([]schema.Value(nil), v.List...)
	}
	return v
}

func zeroOf(d *Desc) schema.Value {
	switch d.Kind {
	case schema.KindBool:
		return schema.Bool(false)
	case schema.KindInt64:
		return schema.Int64(0)
	case schema.KindFloat64:
		return schema.Float64(0)
	case schema.KindEnum:
		if len(d.Choices) > 0 {
			return d.Choices[0].Value
		}
		return schema.Enum(0)
	case schema.KindVec2:
		return schema.Vec2(mathx.Vec2{})
	case schema.KindColor:
		return schema.Color(mathx.Color{})
	case schema.KindList:
		return schema.Value{Kind: schema.KindList}
	}
	return schema.String("")
}

func plain(v schema.Value) string {
	switch v.Kind {
	case schema.KindBool:
		if v.I != 0 {
			return "On"
		}
		return "Off"
	case schema.KindInt32, schema.KindInt64, schema.KindEnum:
		return strconv.FormatInt(v.I, 10)
	case schema.KindFloat32, schema.KindFloat64:
		return strconv.FormatFloat(v.F, 'g', 4, 64)
	case schema.KindVec2:
		return strconv.FormatFloat(float64(v.V.X), 'g', 4, 32) + ", " + strconv.FormatFloat(float64(v.V.Y), 'g', 4, 32)
	case schema.KindColor:
		return strconv.FormatFloat(float64(v.C.R), 'g', 3, 32) + " " + strconv.FormatFloat(float64(v.C.G), 'g', 3, 32) + " " +
			strconv.FormatFloat(float64(v.C.B), 'g', 3, 32) + " " + strconv.FormatFloat(float64(v.C.A), 'g', 3, 32)
	case schema.KindList:
		parts := make([]string, len(v.List))
		for i, x := range v.List {
			parts[i] = x.S
		}
		return strings.Join(parts, ", ")
	}
	return v.S
}
