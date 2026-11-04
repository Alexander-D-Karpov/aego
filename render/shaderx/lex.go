package shaderx

import "strings"

// token is deliberately smaller than a GLSL compiler token. It is sufficient
// for declarations, directives, balanced scopes and locating entry definitions.
// Semantic validation is still performed by the pinned reference compiler.
type token struct {
	text       string
	start, end int
	line       int
}

type lexicalLine struct {
	code    string
	comment string
}

// lexicalLines blanks comments while preserving every byte offset and newline.
// A comment cannot introduce pragmas, declarations or an entry point.
func lexicalLines(src string) ([]lexicalLine, bool) {
	lines := strings.Split(src, "\n")
	out := make([]lexicalLine, len(lines))
	block := false
	for n, raw := range lines {
		b := []byte(raw)
		quote := byte(0)
		for i := 0; i < len(b); i++ {
			if block {
				if b[i] == '*' && i+1 < len(b) && b[i+1] == '/' {
					b[i], b[i+1] = ' ', ' '
					i++
					block = false
				} else {
					b[i] = ' '
				}
				continue
			}
			if quote != 0 {
				if b[i] == '\\' {
					i++
				} else if b[i] == quote {
					quote = 0
				}
				continue
			}
			if b[i] == '"' || b[i] == '\'' {
				quote = b[i]
				continue
			}
			if b[i] == '/' && i+1 < len(b) {
				if b[i+1] == '/' {
					out[n].comment = raw[i+2:]
					for j := i; j < len(b); j++ {
						b[j] = ' '
					}
					break
				}
				if b[i+1] == '*' {
					b[i], b[i+1] = ' ', ' '
					i++
					block = true
				}
			}
		}
		out[n].code = string(b)
	}
	return out, !block
}

func identifier(s string) bool {
	if len(s) == 0 || !identStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !identPart(s[i]) {
			return false
		}
	}
	return true
}
func identStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func identPart(c byte) bool  { return identStart(c) || c >= '0' && c <= '9' }

func tokens(src string) []token {
	lines, _ := lexicalLines(src)
	var b strings.Builder
	for i, l := range lines {
		if i != 0 {
			b.WriteByte('\n')
		}
		b.WriteString(l.code)
	}
	s := b.String()
	var out []token
	line := 1
	for i := 0; i < len(s); {
		if s[i] == '\n' {
			line++
			i++
			continue
		}
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\r' {
			i++
			continue
		}
		start, at := i, line
		if identStart(s[i]) {
			i++
			for i < len(s) && identPart(s[i]) {
				i++
			}
		} else if s[i] == '"' || s[i] == '\'' {
			q := s[i]
			i++
			for i < len(s) && s[i] != q {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				if s[i] == '\n' {
					line++
				}
				i++
			}
			if i < len(s) {
				i++
			}
		} else {
			i++
		}
		out = append(out, token{text: s[start:i], start: start, end: i, line: at})
	}
	return out
}

// entryRange recognizes a global function definition, not a prototype, call or
// matching text in a comment. The returned offsets are into the original text.
func entryRange(src, name string) (int, int, bool) {
	ts := tokens(src)
	depth := 0
	for i := 0; i < len(ts); i++ {
		if ts[i].text == "#" {
			line := ts[i].line
			for i+1 < len(ts) && (ts[i+1].line == line || ts[i].text == "\\") {
				i++
				line = ts[i].line
			}
			continue
		}
		if depth == 0 && i+4 < len(ts) && ts[i].text == "void" && ts[i+1].text == name && ts[i+2].text == "(" {
			j := i + 3
			if ts[j].text == "void" {
				j++
			}
			if j+1 < len(ts) && ts[j].text == ")" && ts[j+1].text == "{" {
				n := 1
				for k := j + 2; k < len(ts); k++ {
					switch ts[k].text {
					case "{":
						n++
					case "}":
						n--
					}
					if n == 0 {
						return ts[i].start, ts[k].end, true
					}
				}
				return ts[i].start, len(src), false
			}
		}
		switch ts[i].text {
		case "{":
			depth++
		case "}":
			depth--
		}
	}
	return 0, 0, false
}

func reserved(s string) bool {
	if unsupportedToken(s) {
		return true
	}
	switch s {
	case "attribute", "const", "uniform", "varying", "layout", "centroid", "flat", "smooth", "in", "out", "inout",
		"break", "continue", "do", "for", "while", "switch", "case", "default", "if", "else", "return", "discard",
		"void", "bool", "int", "uint", "float", "true", "false", "struct", "precision", "lowp", "mediump", "highp", "invariant",
		"vec2", "vec3", "vec4", "bvec2", "bvec3", "bvec4", "ivec2", "ivec3", "ivec4", "uvec2", "uvec3", "uvec4",
		"mat2", "mat3", "mat4", "mat2x2", "mat2x3", "mat2x4", "mat3x2", "mat3x3", "mat3x4", "mat4x2", "mat4x3", "mat4x4",
		"sampler2D", "sampler3D", "samplerCube", "sampler2DShadow", "samplerCubeShadow", "sampler2DArray", "sampler2DArrayShadow",
		"isampler2D", "isampler3D", "isamplerCube", "isampler2DArray", "usampler2D", "usampler3D", "usamplerCube", "usampler2DArray":
		return true
	}
	if strings.HasPrefix(s, "aego_") || strings.HasPrefix(s, "AEGO_") || strings.HasPrefix(s, "gl_") || strings.Contains(s, "__") {
		return true
	}
	if s == "main" || s == "fragment" || s == "vertex" {
		return true
	}
	for _, prefix := range []string{"tex", "ub"} {
		if rest, ok := strings.CutPrefix(s, prefix); ok && rest != "" {
			all := true
			for i := range rest {
				if rest[i] < '0' || rest[i] > '9' {
					all = false
				}
			}
			if all {
				return true
			}
		}
	}
	return false
}

// annotations handles balanced arguments and quoted text. A ')' inside a doc
// string is ordinary text, and malformed/duplicate annotations are diagnosed.
func annotations(src string) ([][2]string, bool) {
	var out [][2]string
	for i := 0; i < len(src); {
		if src[i] != '@' {
			i++
			continue
		}
		i++
		start := i
		for i < len(src) && identPart(src[i]) {
			i++
		}
		if start == i {
			return out, false
		}
		name := src[start:i]
		for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
			i++
		}
		arg := ""
		if i < len(src) && src[i] == '(' {
			i++
			start = i
			depth := 1
			quote := byte(0)
			for i < len(src) {
				c := src[i]
				if quote != 0 {
					if c == '\\' {
						i += 2
						continue
					}
					if c == quote {
						quote = 0
					}
				} else {
					switch c {
					case '"', '\'':
						quote = c
					case '(':
						depth++
					case ')':
						depth--
					}
					if depth == 0 {
						break
					}
				}
				i++
			}
			if i >= len(src) || depth != 0 {
				return out, false
			}
			arg = strings.TrimSpace(src[start:i])
			i++
		}
		out = append(out, [2]string{name, arg})
	}
	return out, true
}
