package material

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"aego/core/ids"
	"aego/render/shader"
)

const (
	MAT001 = "MAT001"
	MAT002 = "MAT002"
	MAT003 = "MAT003"
	MAT004 = "MAT004"
)

type Value struct {
	Name      string
	Lanes     [4]float32
	Int       int32
	Kind      shader.Kind
	Typed     bool
	Color     bool
	Symbol    string
	Reference ids.AssetID
	N         int
	Asset     bool
	Line      int
}

type Problem struct {
	Code string
	Line int
	Msg  string
	Warn bool
}

func Validate(sh *Shader, keywords []string, vals []Value) []Problem {
	var out []Problem
	var mask uint32
	known := true
	for _, k := range keywords {
		i := slices.Index(sh.Keywords, k)
		if i < 0 {
			out = append(out, Problem{Code: MAT003, Msg: fmt.Sprintf("%s has no variant keyword %s", sh.Name, k)})
			known = false
			continue
		}
		mask |= 1 << i
	}
	if known && sh.Variant(mask) < 0 {
		out = append(out, Problem{Code: MAT003, Msg: fmt.Sprintf("%s does not list the variant %s", sh.Name, strings.Join(keywords, "+"))})
	}
	for _, v := range vals {
		p, ok := sh.Schema.Param(v.Name)
		switch {
		case !ok:
			out = append(out, Problem{Code: MAT001, Line: v.Line, Warn: true, Msg: fmt.Sprintf("%s has no parameter %s; the value stays in the file", sh.Name, v.Name)})
		case (p.Kind == shader.KindTexture) != v.Asset:
			want := "a value"
			if p.Kind == shader.KindTexture {
				want = "a texture"
			}
			out = append(out, Problem{Code: MAT002, Line: v.Line, Msg: fmt.Sprintf("%s takes %s", v.Name, want)})
		case !v.Asset && v.Typed && v.Kind != p.Kind:
			out = append(out, Problem{Code: MAT002, Line: v.Line, Msg: fmt.Sprintf("%s takes %s, got %s", v.Name, p.Kind.GLSL(), v.Kind.GLSL())})
		case !v.Asset && v.N != p.Kind.Lanes():
			out = append(out, Problem{Code: MAT002, Line: v.Line, Msg: fmt.Sprintf("%s takes %d number(s), the file has %d", v.Name, p.Kind.Lanes(), v.N)})
		case !v.Asset && !validNumber(v, p):
			out = append(out, Problem{Code: MAT002, Line: v.Line, Msg: fmt.Sprintf("%s has an invalid %s value", v.Name, p.Kind.GLSL())})
		case !v.Asset && p.HasRange && outside(v, p):
			out = append(out, Problem{Code: MAT002, Line: v.Line, Warn: true, Msg: fmt.Sprintf("%s is outside %g…%g", v.Name, p.Min, p.Max)})
		}
	}
	return out
}

func outside(v Value, p *shader.Param) bool {
	for c := 0; c < v.N; c++ {
		if v.Lanes[c] < p.Min || v.Lanes[c] > p.Max {
			return true
		}
	}
	return false
}

func CheckDomain(sh *Shader, want shader.Domain) (Problem, bool) {
	if sh.Domain == want {
		return Problem{}, true
	}
	return Problem{Code: MAT004, Msg: fmt.Sprintf("%s is a %s shader, this place needs %s", sh.Name, sh.Domain, want)}, false
}

func validNumber(v Value, p *shader.Param) bool {
	if p.Kind == shader.KindInt && v.Typed {
		return true
	}
	for i := 0; i < v.N; i++ {
		if math.IsNaN(float64(v.Lanes[i])) || math.IsInf(float64(v.Lanes[i]), 0) {
			return false
		}
	}
	if p.Kind == shader.KindInt {
		n := float64(v.Lanes[0])
		return n >= math.MinInt32 && n <= math.MaxInt32 && n == math.Trunc(n)
	}
	if p.Kind == shader.KindBool {
		return v.Lanes[0] == 0 || v.Lanes[0] == 1
	}
	return true
}
