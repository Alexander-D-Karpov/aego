package shaderx

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"aego/render/shader"
)

const (
	SHD001 = "SHD001"
	SHD002 = "SHD002"
	SHD003 = "SHD003"
	SHD004 = "SHD004"
	SHD005 = "SHD005"
	SHD006 = "SHD006"
	SHD007 = "SHD007"
	SHD008 = "SHD008"
	SHD009 = "SHD009"
)

type Loc = shader.Loc

type Diag struct {
	Code string
	File string
	Line int
	Msg  string
}

func (d Diag) Error() string {
	if d.Line > 0 {
		return fmt.Sprintf("%s:%d: %s: %s", d.File, d.Line, d.Code, d.Msg)
	}
	return fmt.Sprintf("%s: %s: %s", d.File, d.Code, d.Msg)
}

type Includer func(path string) (string, []byte, error)

type srcLine struct {
	text string
	loc  Loc
}

type Unit struct {
	Domain   shader.Domain
	Blend    shader.Blend
	Keywords []string
	Combos   []uint32
	Params   []shader.Param
	Size     int
	Lanes    int
	Vertex   bool
	Files    []string
	body     []srcLine
}

var (
	reInclude  = regexp.MustCompile(`^#\s*include\s+"([^"]+)"$`)
	reUniform  = regexp.MustCompile(`^uniform\s+(\w+)\s+(\w+)\s*;\s*(?://(.*))?$`)
	reAnnot    = regexp.MustCompile(`@(\w+)(?:\(([^)]*)\))?`)
	reIdent    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	reRaw      = regexp.MustCompile(`^uniform\s`)
	reReserved = regexp.MustCompile(`^(?:aego_|AEGO_|gl_)|^(?:tex|ub)\d+$|^(?:main|fragment|vertex)$`)
	reFrag     = regexp.MustCompile(`\bvoid\s+fragment\s*\(\s*\)`)
	reVert     = regexp.MustCompile(`\bvoid\s+vertex\s*\(\s*\)`)
)

var ownPragmas = []string{"aego", "domain", "blend", "variant", "variants", "material", "end"}

type parser struct {
	u        *Unit
	inc      Includer
	stack    []string
	diags    []Diag
	abi      bool
	section  bool
	variants string
	varLoc   Loc
}

func Parse(src []byte, name string, inc Includer) (*Unit, []Diag) {
	u := &Unit{Domain: shader.DomainSprite, Blend: shader.BlendPremultiplied, Files: []string{name}}
	p := &parser{u: u, inc: inc, stack: []string{name}}
	p.file(string(src), 0, 0)
	if p.section {
		p.errf(SHD001, Loc{}, "#pragma material has no #pragma end")
	}
	p.finish()
	return u, p.diags
}

func (u *Unit) Variant(mask uint32) int {
	return slices.Index(u.Combos, mask)
}

func (p *parser) errf(code string, l Loc, format string, args ...any) {
	file := ""
	if l.File >= 0 && int(l.File) < len(p.u.Files) {
		file = p.u.Files[l.File]
	}
	p.diags = append(p.diags, Diag{Code: code, File: file, Line: int(l.Line), Msg: fmt.Sprintf(format, args...)})
}

func (p *parser) file(src string, idx, depth int) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	for i, raw := range lines {
		l := Loc{File: int32(idx), Line: int32(i + 1)}
		t := strings.TrimSpace(raw)
		switch {
		case p.section && t == "#pragma end":
			p.section = false
		case p.section:
			p.param(t, l)
		case strings.HasPrefix(t, "#pragma "):
			p.pragma(strings.Fields(t)[1:], raw, l, idx == 0)
		case strings.HasPrefix(t, "#include"):
			p.include(t, l, depth)
		case reRaw.MatchString(t):
			p.errf(SHD003, l, "declare parameters between #pragma material and #pragma end")
		default:
			p.u.body = append(p.u.body, srcLine{text: raw, loc: l})
		}
	}
}

func (p *parser) pragma(f []string, raw string, l Loc, main bool) {
	if len(f) == 0 || !slices.Contains(ownPragmas, f[0]) {
		p.u.body = append(p.u.body, srcLine{text: raw, loc: l})
		return
	}
	if !main {
		p.errf(SHD002, l, "#pragma %s belongs in the shader file, not in an include", f[0])
		return
	}
	arg := ""
	if len(f) > 1 {
		arg = f[1]
	}
	switch f[0] {
	case "aego":
		if len(f) != 3 || f[1] != "abi" {
			p.errf(SHD001, l, "expected #pragma aego abi %d", shader.ABI)
			return
		}
		if n, err := strconv.Atoi(f[2]); err != nil || n != shader.ABI {
			p.errf(SHD001, l, "this engine reads shader ABI %d, the file asks for %s", shader.ABI, f[2])
			return
		}
		p.abi = true
	case "domain":
		d, ok := shader.ParseDomain(arg)
		if !ok {
			p.errf(SHD001, l, "unknown domain %q: sprite, mesh2d or postfx", arg)
			return
		}
		p.u.Domain = d
	case "blend":
		b, ok := shader.ParseBlend(arg)
		if !ok {
			p.errf(SHD001, l, "unknown blend %q: premultiplied, opaque or additive", arg)
			return
		}
		p.u.Blend = b
	case "variant":
		for _, k := range f[1:] {
			switch {
			case !reIdent.MatchString(k) || reReserved.MatchString(k):
				p.errf(SHD006, l, "%q is not a valid variant keyword", k)
			case slices.Contains(p.u.Keywords, k):
				p.errf(SHD006, l, "variant keyword %s is declared twice", k)
			case len(p.u.Keywords) == shader.MaxKeywords:
				p.errf(SHD006, l, "a shader has at most %d variant keywords", shader.MaxKeywords)
			default:
				p.u.Keywords = append(p.u.Keywords, k)
			}
		}
	case "variants":
		p.variants, p.varLoc = strings.Join(f[1:], ""), l
	case "material":
		p.section = true
	case "end":
		p.errf(SHD001, l, "#pragma end without #pragma material")
	}
}

func (p *parser) include(t string, l Loc, depth int) {
	m := reInclude.FindStringSubmatch(t)
	switch {
	case m == nil:
		p.errf(SHD002, l, `expected #include "path"`)
		return
	case p.inc == nil:
		p.errf(SHD002, l, "includes are not available here")
		return
	case depth+1 > shader.MaxIncludeDepth:
		p.errf(SHD002, l, "includes nest deeper than %d", shader.MaxIncludeDepth)
		return
	}
	name, src, err := p.inc(m[1])
	if err != nil {
		p.errf(SHD002, l, "cannot include %s: %v", m[1], err)
		return
	}
	if slices.Contains(p.stack, name) {
		p.errf(SHD005, l, "include cycle: %s", strings.Join(append(slices.Clone(p.stack), name), " → "))
		return
	}
	if slices.Contains(p.u.Files, name) {
		return
	}
	idx := len(p.u.Files)
	p.u.Files = append(p.u.Files, name)
	p.stack = append(p.stack, name)
	p.file(string(src), idx, depth+1)
	p.stack = p.stack[:len(p.stack)-1]
}

func (p *parser) param(t string, l Loc) {
	if t == "" || strings.HasPrefix(t, "//") {
		return
	}
	m := reUniform.FindStringSubmatch(t)
	if m == nil {
		p.errf(SHD001, l, "expected uniform <type> <Name>; // @annotations")
		return
	}
	k, ok := shader.ParseKind(m[1])
	if !ok {
		p.errf(SHD003, l, "%s has type %s; parameters are float, int, bool, vec2, vec3, vec4 or sampler2D", m[2], m[1])
		return
	}
	name := m[2]
	switch {
	case len(name) > shader.MaxNameLen:
		p.errf(SHD004, l, "parameter name %s is longer than %d characters", name, shader.MaxNameLen)
	case reReserved.MatchString(name):
		p.errf(SHD003, l, "%s is a reserved name", name)
	case slices.ContainsFunc(p.u.Params, func(q shader.Param) bool { return q.Name == name }):
		p.errf(SHD003, l, "parameter %s is declared twice", name)
	default:
		q := shader.Param{Name: name, Kind: k, File: int(l.File), Line: int(l.Line)}
		p.annotate(&q, m[3], l)
		p.u.Params = append(p.u.Params, q)
	}
}

func (p *parser) annotate(q *shader.Param, text string, l Loc) {
	hasDefault := false
	for _, a := range reAnnot.FindAllStringSubmatch(text, -1) {
		arg := strings.TrimSpace(a[2])
		switch a[1] {
		case "default":
			n, ok := numbers(arg, q.Default[:])
			if !ok || (n != 1 && n != q.Kind.Lanes()) {
				p.errf(SHD003, l, "@default of %s needs 1 or %d number(s)", q.Name, q.Kind.Lanes())
				continue
			}
			for c := n; c < q.Kind.Lanes(); c++ {
				q.Default[c] = q.Default[0]
			}
			hasDefault = true
		case "range":
			var r [4]float32
			if n, ok := numbers(arg, r[:]); !ok || n != 2 || r[0] > r[1] {
				p.errf(SHD003, l, "@range of %s needs min and max", q.Name)
				continue
			}
			q.Min, q.Max, q.HasRange = r[0], r[1], true
		case "color":
			if q.Kind != shader.KindVec3 && q.Kind != shader.KindVec4 {
				p.errf(SHD003, l, "@color applies to vec3 and vec4")
				continue
			}
			q.Color = true
		case "texture":
			if q.Kind != shader.KindTexture {
				p.errf(SHD003, l, "@texture applies to sampler2D")
			}
		case "doc":
			q.Doc = unquote(arg)
		case "display":
			q.Display = unquote(arg)
		case "unit":
			q.Unit = unquote(arg)
		case "hidden":
			q.Hidden = true
		case "instance":
			if q.Kind == shader.KindInt || q.Kind == shader.KindBool || q.Kind == shader.KindTexture {
				p.errf(SHD003, l, "@instance applies to float and vector parameters")
				continue
			}
			q.Instance = true
		case "enum":
			if q.Kind != shader.KindInt {
				p.errf(SHD003, l, "@enum applies to int")
				continue
			}
			for _, e := range strings.Split(arg, ",") {
				q.Enum = append(q.Enum, strings.TrimSpace(e))
			}
		default:
			p.errf(SHD003, l, "unknown annotation @%s", a[1])
		}
	}
	if q.Color && !hasDefault {
		q.Default = [4]float32{1, 1, 1, 1}
	}
}

func numbers(s string, dst []float32) (int, bool) {
	parts := strings.Split(s, ",")
	if s == "" || len(parts) > len(dst) {
		return 0, false
	}
	for i, x := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(x), 32)
		if err != nil {
			return 0, false
		}
		dst[i] = float32(v)
	}
	return len(parts), true
}

func unquote(s string) string {
	if v, err := strconv.Unquote(s); err == nil {
		return v
	}
	return s
}

func (p *parser) finish() {
	u := p.u
	if !p.abi {
		p.errf(SHD001, Loc{Line: 1}, "the shader must start with #pragma aego abi %d", shader.ABI)
	}
	if u.Domain == shader.DomainPostFX {
		p.errf(SHD007, Loc{}, "the postfx domain is reserved and has no renderer yet")
	}
	body := joinBody(u.body)
	if !reFrag.MatchString(body) {
		p.errf(SHD001, Loc{}, "the shader has no void fragment()")
	}
	u.Vertex = reVert.MatchString(body)
	for _, q := range u.Params {
		if slices.Contains(u.Keywords, q.Name) {
			p.errf(SHD003, loc(q), "%s is both a parameter and a variant keyword", q.Name)
		}
	}
	p.combos()
	p.layout()
	p.forbidden()
}

func loc(q shader.Param) Loc {
	return Loc{File: int32(q.File), Line: int32(q.Line)}
}

func (p *parser) combos() {
	u := p.u
	if p.variants == "" {
		if 1<<len(u.Keywords) > shader.MaxVariants {
			p.errf(SHD006, Loc{}, "%d variant keywords make %d variants; list the ones in use with #pragma variants", len(u.Keywords), 1<<len(u.Keywords))
			u.Combos = []uint32{0}
			return
		}
		for m := 0; m < 1<<len(u.Keywords); m++ {
			u.Combos = append(u.Combos, uint32(m))
		}
		return
	}
	u.Combos = []uint32{0}
	for _, part := range strings.FieldsFunc(p.variants, func(r rune) bool { return r == ',' || r == '|' }) {
		var m uint32
		ok := true
		for _, k := range strings.Split(part, "+") {
			i := slices.Index(u.Keywords, k)
			if i < 0 {
				p.errf(SHD006, p.varLoc, "#pragma variants names %s, which no #pragma variant declares", k)
				ok = false
				break
			}
			m |= 1 << i
		}
		if ok && !slices.Contains(u.Combos, m) {
			u.Combos = append(u.Combos, m)
		}
	}
	if len(u.Combos) > shader.MaxVariants {
		p.errf(SHD006, p.varLoc, "the shader lists %d variants, at most %d", len(u.Combos), shader.MaxVariants)
	}
}

func (p *parser) layout() {
	u := p.u
	if len(u.Params) > shader.MaxParams {
		p.errf(SHD004, Loc{}, "a shader has at most %d parameters", shader.MaxParams)
	}
	var kinds []shader.Kind
	var idx []int
	tex, lane, comp, lanes := 0, 0, 0, false
	for i := range u.Params {
		q := &u.Params[i]
		switch {
		case q.Kind == shader.KindTexture:
			if tex == shader.MaxUserTextures {
				p.errf(SHD004, loc(*q), "a shader has at most %d textures besides the sprite", shader.MaxUserTextures)
				continue
			}
			q.Slot = shader.TexUser + tex
			tex++
		case q.Instance:
			if u.Domain != shader.DomainSprite {
				p.errf(SHD007, loc(*q), "@instance parameters need the sprite domain")
				continue
			}
			n := q.Kind.Lanes()
			if comp+n > 4 {
				lane, comp = lane+1, 0
			}
			q.Lane, q.Comp = lane, comp
			comp += n
			lanes = true
		default:
			kinds = append(kinds, q.Kind)
			idx = append(idx, i)
		}
	}
	if lanes {
		u.Lanes = lane + 1
		if u.Lanes > shader.MaxLanes {
			p.errf(SHD004, Loc{}, "@instance parameters take %d vec4 lanes, at most %d fit", u.Lanes, shader.MaxLanes)
		}
	}
	offs := make([]int, len(kinds))
	u.Size = shader.Std140(kinds, offs)
	for j, i := range idx {
		u.Params[i].Offset = offs[j]
	}
}

func (p *parser) forbidden() {
	for _, l := range p.u.body {
		code, _, _ := strings.Cut(l.text, "//")
		for _, f := range unsupported {
			if strings.Contains(code, f) {
				p.errf(SHD008, l.loc, "%s has no GLSL ES 3.00 form", strings.TrimSuffix(f, "("))
			}
		}
	}
}

func joinBody(body []srcLine) string {
	parts := make([]string, len(body))
	for i, l := range body {
		parts[i] = l.text
	}
	return strings.Join(parts, "\n")
}
