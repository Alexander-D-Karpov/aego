package shaderx

import (
	"crypto/sha256"
	"fmt"
	"math"
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

type Diag = shader.Diag

type Includer func(path string) (string, []byte, error)

// OwnerIncluder resolves an include in the namespace of its current source.
// The returned canonical name becomes the owner of that include's children.
type OwnerIncluder func(owner, path string) (string, []byte, error)

type srcLine struct {
	text string
	loc  Loc
}

type Unit struct {
	Features     shader.Features
	Domain       shader.Domain
	Blend        shader.Blend
	Keywords     []string
	Combos       []uint32
	Params       []shader.Param
	Size         int
	Lanes        int
	Vertex       bool
	Files        []string
	Dependencies []shader.Dependency
	body         []srcLine
}

var (
	reInclude = regexp.MustCompile(`^#\s*include\s+"([^"]+)"$`)
	reFrag    = regexp.MustCompile(`\bvoid\s+fragment\s*\(\s*\)`)
	reVert    = regexp.MustCompile(`\bvoid\s+vertex\s*\(\s*\)`)
)

var ownPragmas = []string{"aego", "domain", "postfx", "blend", "lighting", "variant", "variants", "material", "end", "pixel_coverage"}

type parser struct {
	u           *Unit
	inc         OwnerIncluder
	stack       []string
	diags       []Diag
	abi         bool
	section     bool
	variants    string
	varLoc      Loc
	expanded    int
	conditional int
	braces      int
	pending     string
	pendingLoc  Loc
	seen        map[string]bool
}

func Parse(src []byte, name string, inc Includer) (*Unit, []Diag) {
	var owned OwnerIncluder
	if inc != nil {
		owned = func(_ string, path string) (string, []byte, error) { return inc(path) }
	}
	return ParseWithIncludes(src, name, owned)
}

// ParseWithIncludes uses the same bounded parser as Parse while giving the
// resolver the exact source/include containing each directive.
func ParseWithIncludes(src []byte, name string, inc OwnerIncluder) (*Unit, []Diag) {
	u := &Unit{Domain: shader.DomainSprite, Blend: shader.BlendPremultiplied, Files: []string{name}}
	p := &parser{u: u, inc: inc, stack: []string{name}, seen: make(map[string]bool)}
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
	if len(p.diags) >= 64 {
		return
	}
	file := ""
	if l.File >= 0 && int(l.File) < len(p.u.Files) {
		file = p.u.Files[l.File]
	}
	p.diags = append(p.diags, Diag{Code: code, File: file, Line: int(l.Line), Msg: fmt.Sprintf(format, args...)})
}

func (p *parser) file(src string, idx, depth int) {
	p.expanded += len(src)
	if p.expanded > shader.MaxExpandedBytes {
		p.errf(SHD004, Loc{File: int32(idx), Line: 1}, "expanded shader exceeds %d bytes", shader.MaxExpandedBytes)
		return
	}
	p.u.Dependencies = append(p.u.Dependencies, shader.Dependency{Path: p.u.Files[idx], Hash: sha256.Sum256([]byte(src))})
	src = strings.ReplaceAll(src, "\r\n", "\n")
	rawLines := strings.Split(src, "\n")
	lines, complete := lexicalLines(src)
	if !complete {
		p.errf(SHD001, Loc{File: int32(idx), Line: int32(len(lines))}, "unterminated block comment")
	}
	baseCondition := p.conditional
	for i, line := range lines {
		l := Loc{File: int32(idx), Line: int32(i + 1)}
		t := strings.TrimSpace(line.code)
		raw := rawLines[i]
		if strings.HasPrefix(t, "#") {
			directive := strings.Fields(strings.TrimSpace(strings.TrimPrefix(t, "#")))
			if len(directive) == 0 {
				p.errf(SHD001, l, "empty preprocessor directive")
				continue
			}
			switch directive[0] {
			case "if", "ifdef", "ifndef":
				if p.section {
					p.errf(SHD003, l, "material declarations cannot be conditional")
				}
				p.conditional++
			case "endif":
				if p.conditional <= baseCondition {
					p.errf(SHD001, l, "unmatched #endif")
				} else {
					p.conditional--
				}
			case "else", "elif":
				if p.conditional <= baseCondition {
					p.errf(SHD001, l, "unmatched conditional branch")
				}
			case "version", "extension":
				p.errf(SHD008, l, "#%s is outside the portable Aego profile", directive[0])
				continue
			}
			switch directive[0] {
			case "pragma":
				if len(directive) > 1 && slices.Contains(ownPragmas, directive[1]) {
					if p.conditional != 0 || p.braces != 0 {
						p.errf(SHD003, l, "Aego metadata must be unconditional and at global scope")
						continue
					}
					if p.pending != "" {
						p.errf(SHD001, p.pendingLoc, "unfinished material declaration")
						p.pending = ""
					}
					if directive[1] == "end" && p.section {
						p.section = false
						continue
					}
					p.pragma(directive[1:], raw, l, idx == 0)
					continue
				}
			case "include":
				if p.section {
					p.errf(SHD003, l, "includes cannot declare material fields")
					continue
				}
				if p.braces != 0 {
					p.errf(SHD002, l, "includes belong at global scope")
					continue
				}
				p.include("#include "+strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(t, "#")), "include")), l, depth)
				continue
			}
			if p.section {
				p.errf(SHD003, l, "preprocessor directives cannot change material schema")
				continue
			}
			p.u.body = append(p.u.body, srcLine{text: raw, loc: l})
			continue
		}
		if p.section {
			if t == "" {
				continue
			}
			if p.pending == "" {
				p.pendingLoc = l
			}
			if len(p.pending)+len(t) > 256 {
				p.errf(SHD004, p.pendingLoc, "material declaration is too long")
				p.pending = ""
				continue
			}
			p.pending += " " + t
			for {
				at := strings.IndexByte(p.pending, ';')
				if at < 0 {
					break
				}
				declaration := p.pending[:at+1]
				p.pending = strings.TrimSpace(p.pending[at+1:])
				note := ""
				if p.pending == "" {
					note = line.comment
				}
				p.param(declaration, note, p.pendingLoc)
				p.pendingLoc = l
			}
			continue
		}
		for _, tok := range tokens(t) {
			if tok.text == "uniform" {
				p.errf(SHD003, l, "declare parameters between #pragma material and #pragma end")
			}
			if tok.text == "{" {
				p.braces++
			}
			if tok.text == "}" {
				p.braces--
			}
		}
		p.u.body = append(p.u.body, srcLine{text: raw, loc: l})
	}
	if p.conditional != baseCondition {
		p.errf(SHD001, Loc{File: int32(idx), Line: int32(len(lines))}, "unterminated conditional directive")
		p.conditional = baseCondition
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
	if f[0] != "variant" && f[0] != "end" {
		if p.seen[f[0]] {
			p.errf(SHD001, l, "#pragma %s is declared twice", f[0])
			return
		}
		p.seen[f[0]] = true
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
	case "pixel_coverage":
		if len(f) != 1 {
			p.errf(SHD001, l, "expected #pragma pixel_coverage")
		}
		p.u.Features |= shader.FeaturePixelCoverage
	case "postfx":
		if p.seen["domain"] {
			p.errf(SHD001, l, "use exactly one domain declaration")
			return
		}
		if len(f) == 2 && arg == "scene_linear" {
			p.u.Features |= shader.FeaturePostSceneLinear
		} else if len(f) != 1 && !(len(f) == 2 && arg == "display_linear") {
			p.errf(SHD001, l, "expected #pragma postfx [scene_linear|display_linear]")
			return
		}
		p.u.Domain = shader.DomainPostFX
	case "domain":
		if p.seen["postfx"] {
			p.errf(SHD001, l, "use exactly one domain declaration")
			return
		}
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
	case "lighting":
		if len(f) != 2 || arg != "forward2d" {
			p.errf(SHD001, l, "expected #pragma lighting forward2d")
			return
		}
		p.u.Features |= shader.FeatureLighting
	case "variant":
		for _, k := range f[1:] {
			switch {
			case !identifier(k) || reserved(k) || len(k) > shader.MaxNameLen:
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
	name, src, err := p.inc(p.u.Files[l.File], m[1])
	if err != nil {
		p.errf(SHD002, l, "cannot include %s: %v", m[1], err)
		return
	}
	if slices.Contains(p.stack, name) {
		p.errf(SHD005, l, "include cycle: %s", strings.Join(append(slices.Clone(p.stack), name), " → "))
		return
	}
	if slices.Contains(p.u.Files, name) {
		for _, previous := range p.u.Dependencies {
			if previous.Path == name && previous.Hash != sha256.Sum256(src) {
				p.errf(SHD002, l, "source changed while resolving includes; retry import")
				break
			}
		}
		return
	}
	idx := len(p.u.Files)
	if idx >= shader.MaxIncludeFiles {
		p.errf(SHD004, l, "a shader has at most %d source files", shader.MaxIncludeFiles)
		return
	}
	p.u.Files = append(p.u.Files, name)
	p.stack = append(p.stack, name)
	p.file(string(src), idx, depth+1)
	p.stack = p.stack[:len(p.stack)-1]
}

func (p *parser) param(text, note string, l Loc) {
	if len(p.u.Params) >= shader.MaxParams {
		p.errf(SHD004, l, "a shader has at most %d parameters", shader.MaxParams)
		return
	}
	ts := tokens(text)
	if len(ts) != 4 || ts[0].text != "uniform" || ts[3].text != ";" || !identifier(ts[1].text) || !identifier(ts[2].text) {
		p.errf(SHD001, l, "expected uniform <type> <Name>; // @annotations (no arrays, structures or initializers)")
		return
	}
	kind, name := ts[1].text, ts[2].text
	k, ok := shader.ParseKind(kind)
	if !ok {
		p.errf(SHD003, l, "%s has type %s; parameters are float, int, bool, vec2, vec3, vec4 or sampler2D", name, kind)
		return
	}
	switch {
	case len(name) > shader.MaxNameLen:
		p.errf(SHD004, l, "parameter name %s is longer than %d characters", name, shader.MaxNameLen)
	case reserved(name):
		p.errf(SHD003, l, "%s is a reserved name", name)
	case slices.ContainsFunc(p.u.Params, func(q shader.Param) bool { return q.Name == name }):
		p.errf(SHD003, l, "parameter %s is declared twice", name)
	default:
		q := shader.Param{Name: name, Kind: k, File: int(l.File), Line: int(l.Line)}
		p.annotate(&q, note, l)
		p.u.Params = append(p.u.Params, q)
	}
}

func (p *parser) annotate(q *shader.Param, text string, l Loc) {
	hasDefault := false
	as, valid := annotations(text)
	if !valid {
		p.errf(SHD003, l, "malformed parameter annotation")
		return
	}
	seen := map[string]bool{}
	for _, a := range as {
		arg := a[1]
		if seen[a[0]] {
			p.errf(SHD003, l, "duplicate annotation @%s", a[0])
			continue
		}
		seen[a[0]] = true
		switch a[0] {
		case "default":
			if q.Kind == shader.KindInt {
				v, err := strconv.ParseInt(arg, 10, 32)
				if err != nil {
					p.errf(SHD003, l, "@default of %s requires an exact int32", q.Name)
					continue
				}
				q.IntDefault = int32(v)
				q.Default[0] = float32(v)
				hasDefault = true
				continue
			}
			if q.Kind == shader.KindBool {
				switch arg {
				case "true", "1":
					q.Default[0] = 1
				case "false", "0":
					q.Default[0] = 0
				default:
					p.errf(SHD003, l, "@default of %s requires true or false", q.Name)
				}
				hasDefault = true
				continue
			}
			if q.Kind == shader.KindTexture {
				p.errf(SHD003, l, "texture defaults are material asset references")
				continue
			}
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
		case "normal":
			if q.Kind != shader.KindTexture || arg != "" {
				p.errf(SHD003, l, "@normal applies to sampler2D and takes no arguments")
			}
			q.Normal = true
		case "emission":
			if q.Kind != shader.KindTexture || arg != "" {
				p.errf(SHD003, l, "@emission applies to sampler2D and takes no arguments")
			}
			q.Emission = true
		case "doc":
			q.Doc = unquote(arg)
		case "display":
			q.Display = unquote(arg)
		case "unit":
			q.Unit = unquote(arg)
		case "hidden":
			q.Hidden = true
		case "instance":
			if q.Kind == shader.KindTexture {
				p.errf(SHD003, l, "@instance applies to numeric and boolean parameters")
				continue
			}
			q.Instance = true
		case "former":
			for _, name := range strings.Split(arg, ",") {
				if len(q.Former) == shader.MaxParams {
					p.errf(SHD004, l, "a parameter has at most %d former names", shader.MaxParams)
					break
				}
				name = unquote(strings.TrimSpace(name))
				if !identifier(name) || reserved(name) || len(name) > shader.MaxNameLen || name == q.Name || slices.Contains(q.Former, name) {
					p.errf(SHD003, l, "invalid former parameter name %q", name)
					continue
				}
				q.Former = append(q.Former, name)
			}
		case "enum":
			if q.Kind != shader.KindInt {
				p.errf(SHD003, l, "@enum applies to int")
				continue
			}
			for _, e := range strings.Split(arg, ",") {
				e = strings.TrimSpace(e)
				if len(q.Enum) == 64 {
					p.errf(SHD004, l, "an enum has at most 64 names")
					break
				}
				if !identifier(e) || reserved(e) || slices.Contains(q.Enum, e) {
					p.errf(SHD003, l, "invalid or duplicate enum name %q", e)
					continue
				}
				q.Enum = append(q.Enum, e)
			}
		default:
			p.errf(SHD003, l, "unknown annotation @%s", a[0])
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
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
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
	body := joinBody(u.body)
	if _, _, found := entryRange(body, "fragment"); !found {
		p.errf(SHD001, Loc{}, "the shader has no void fragment()")
	}
	_, _, u.Vertex = entryRange(body, "vertex")
	if u.Features&shader.FeaturePixelCoverage != 0 && (u.Domain != shader.DomainSprite || u.Vertex || u.Features&shader.FeatureLighting != 0) {
		p.errf(SHD007, Loc{}, "pixel coverage needs an unlit sprite shader without a vertex override")
	}
	if u.Domain == shader.DomainPostFX {
		if u.Vertex || u.Features&^shader.FeaturePostSceneLinear != 0 || p.seen["blend"] {
			p.errf(SHD007, Loc{}, "postfx supplies the full-screen vertex, blend and output stages; surface lighting is unavailable")
		}
		for _, q := range u.Params {
			if q.Instance || q.Normal || q.Emission || q.Name == "MainTex" {
				p.errf(SHD007, loc(q), "invalid surface binding or reserved MainTex in postfx")
			}
		}
	}
	for _, q := range u.Params {
		if slices.Contains(u.Keywords, q.Name) {
			p.errf(SHD003, loc(q), "%s is both a parameter and a variant keyword", q.Name)
		}
	}
	aliases := map[string]string{}
	for _, q := range u.Params {
		aliases[q.Name] = q.Name
	}
	for _, q := range u.Params {
		for _, name := range q.Former {
			if prev, ok := aliases[name]; ok {
				p.errf(SHD003, loc(q), "former name %s conflicts with parameter %s", name, prev)
			} else {
				aliases[name] = q.Name
			}
		}
	}
	if p.pending != "" {
		p.errf(SHD001, p.pendingLoc, "unfinished material declaration")
	}
	if p.braces != 0 {
		p.errf(SHD001, Loc{}, "unbalanced shader scopes")
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
	if u.Domain != shader.DomainPostFX {
		u.Features |= shader.FeatureAffine
	}
	if u.Features&shader.FeatureLighting != 0 {
		u.Features |= shader.FeatureSurfaceChannels
	}
	if len(u.Params) > shader.MaxParams {
		p.errf(SHD004, Loc{}, "a shader has at most %d parameters", shader.MaxParams)
	}
	var kinds []shader.Kind
	var idx []int
	tex, lane, comp, lanes := 0, 0, 0, false
	instances := 0
	normal, emission := false, false
	for i := range u.Params {
		q := &u.Params[i]
		switch {
		case q.Kind == shader.KindTexture:
			if q.Normal && q.Emission {
				p.errf(SHD004, loc(*q), "a sampler cannot be both @normal and @emission")
			}
			if q.Emission {
				if emission || u.Features&shader.FeatureLighting == 0 {
					p.errf(SHD004, loc(*q), "a forward2d shader may declare one @emission sampler")
				}
				emission = true
				q.Slot = shader.TexEmission
				continue
			}
			if q.Normal {
				if normal || u.Features&shader.FeatureLighting == 0 {
					p.errf(SHD004, loc(*q), "a forward2d shader may declare one @normal sampler")
				}
				normal = true
				q.Slot = shader.TexNormal
				continue
			}
			if tex == shader.MaxUserTextures {
				p.errf(SHD004, loc(*q), "a shader has at most %d textures besides the sprite", shader.MaxUserTextures)
				continue
			}
			q.Slot = shader.TexUser + tex
			tex++
		case q.Instance:
			instances++
			if instances > shader.MaxInstanceParams {
				p.errf(SHD004, loc(*q), "at most %d parameters allow instance overrides", shader.MaxInstanceParams)
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
	text := joinBody(p.u.body)
	for _, t := range tokens(text) {
		if !unsupportedToken(t.text) {
			continue
		}
		l := Loc{}
		if t.line > 0 && t.line <= len(p.u.body) {
			l = p.u.body[t.line-1].loc
		}
		p.errf(SHD008, l, "%s is outside the portable GLSL 330 / ESSL 300 profile", t.text)
	}
}

func (u *Unit) Descriptor() shader.Descriptor {
	return shader.Descriptor{Features: u.Features, Domain: u.Domain, Blend: u.Blend, Keywords: u.Keywords, Combos: u.Combos, Params: u.Params, Size: u.Size, Lanes: u.Lanes, Vertex: u.Vertex, Files: u.Files, Dependencies: u.Dependencies}
}

func joinBody(body []srcLine) string {
	parts := make([]string, len(body))
	for i, l := range body {
		parts[i] = l.text
	}
	return strings.Join(parts, "\n")
}
