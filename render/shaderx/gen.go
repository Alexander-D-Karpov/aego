package shaderx

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"aego/render/shader"
)

var genLoc = Loc{File: -1}

type builder struct {
	sb   strings.Builder
	locs []Loc
}

func (b *builder) gen(s string) {
	for _, l := range strings.Split(s, "\n") {
		b.add(l, genLoc)
	}
}

func (b *builder) add(s string, l Loc) {
	b.sb.WriteString(s)
	b.sb.WriteByte('\n')
	b.locs = append(b.locs, l)
}

const spriteInputs = `layout(location = 0) in vec4 aego_i_pos_size;
layout(location = 1) in vec4 aego_i_sincos_origin;
layout(location = 2) in vec4 aego_i_uv;
layout(location = 3) in vec4 aego_i_color;`

const spriteMain = `    vec2 corner = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
    vec2 local = (corner - aego_i_sincos_origin.zw) * aego_i_pos_size.zw;
    float s = aego_i_sincos_origin.x;
    float c = aego_i_sincos_origin.y;
    aego_world = vec3(aego_i_pos_size.xy + vec2(local.x * c - local.y * s, local.x * s + local.y * c), 0.0);
    aego_uv = mix(aego_i_uv.xy, aego_i_uv.zw, corner);
    aego_color = aego_i_color;
    aego_local = corner;`

const meshInputs = `layout(location = 0) in vec3 aego_i_pos;
layout(location = 1) in vec3 aego_i_normal;
layout(location = 2) in vec2 aego_i_uv;
layout(location = 3) in vec4 aego_i_color;
layout(std140) uniform ub3 {
    mat4 aego_model;
    vec4 aego_tint;
    vec4 aego_uv_rect;
};`

const meshMain = `    aego_world = (aego_model * vec4(aego_i_pos, 1.0)).xyz;
    aego_uv = mix(aego_uv_rect.xy, aego_uv_rect.zw, aego_i_uv);
    aego_color = aego_i_color * aego_tint;
    aego_local = aego_i_uv;`

const swizzle = "xyzw"

func (u *Unit) Source(frag, es bool, mask uint32) (string, []Loc) {
	var b builder
	if es {
		b.gen(strings.TrimSuffix(Header, "\n"))
	} else {
		b.gen(SourceVersion)
	}
	f, v := 0, 1
	if frag {
		f, v = 1, 0
	}
	b.gen(fmt.Sprintf("#define AEGO_ABI %d\n#define AEGO_FRAGMENT %d\n#define AEGO_VERTEX %d", shader.ABI, f, v))
	for i, k := range u.Keywords {
		b.gen(fmt.Sprintf("#define %s %d", k, mask>>i&1))
	}
	b.gen(shader.FrameBlock)
	b.gen(shader.ViewBlock)
	u.uniforms(&b)
	if frag {
		u.fragment(&b)
	} else {
		u.vertex(&b)
	}
	return b.sb.String(), b.locs
}

func (u *Unit) uniforms(b *builder) {
	if u.Size > 0 {
		b.gen(fmt.Sprintf("layout(std140) uniform ub%d {", shader.SlotMaterial))
		for _, q := range u.Params {
			if q.Instance || q.Kind == shader.KindTexture {
				continue
			}
			b.add(fmt.Sprintf("    %s %s;", q.Kind.GLSL(), q.Name), loc(q))
		}
		b.gen("};")
	}
	b.gen(fmt.Sprintf("uniform sampler2D tex%d;\n#define aego_main tex%d", shader.TexMain, shader.TexMain))
	for _, q := range u.Params {
		if q.Kind == shader.KindTexture {
			b.add(fmt.Sprintf("uniform sampler2D tex%d;", q.Slot), loc(q))
			b.gen(fmt.Sprintf("#define %s tex%d", q.Name, q.Slot))
		}
	}
}

func (u *Unit) laneDefines(b *builder) {
	for _, q := range u.Params {
		if q.Instance {
			b.gen(fmt.Sprintf("#define %s aego_lane%d.%s", q.Name, q.Lane, swizzle[q.Comp:q.Comp+q.Kind.Lanes()]))
		}
	}
}

func (u *Unit) fragment(b *builder) {
	b.gen("in vec2 aego_uv;\nin vec4 aego_color;\nin vec2 aego_local;")
	for i := 0; i < u.Lanes; i++ {
		b.gen(fmt.Sprintf("flat in vec4 aego_lane%d;", i))
	}
	u.laneDefines(b)
	b.gen("out vec4 aego_out;")
	for _, l := range strip(u.body, reVert) {
		b.add(l.text, l.loc)
	}
	b.gen("void main() {\n    fragment();\n}")
}

func (u *Unit) vertex(b *builder) {
	mesh := u.Domain == shader.DomainMesh2D
	if mesh {
		b.gen(meshInputs)
	} else {
		b.gen(spriteInputs)
	}
	for i := 0; i < u.Lanes; i++ {
		b.gen(fmt.Sprintf("layout(location = %d) in vec4 aego_i_lane%d;\nflat out vec4 aego_lane%d;", shader.LaneLocation+i, i, i))
	}
	u.laneDefines(b)
	b.gen("out vec2 aego_uv;\nout vec4 aego_color;\nout vec2 aego_local;\nvec3 aego_world;")
	if u.Vertex {
		for _, l := range strip(u.body, reFrag) {
			b.add(l.text, l.loc)
		}
	}
	b.gen("void main() {")
	if mesh {
		b.gen(meshMain)
	} else {
		b.gen(spriteMain)
	}
	for i := 0; i < u.Lanes; i++ {
		b.gen(fmt.Sprintf("    aego_lane%d = aego_i_lane%d;", i, i))
	}
	if u.Vertex {
		b.gen("    vertex();")
	}
	b.gen("    gl_Position = aego_view_proj * vec4(aego_world, 1.0);\n}")
}

func strip(body []srcLine, entry *regexp.Regexp) []srcLine {
	text := []byte(joinBody(body))
	at := entry.FindIndex(text)
	if at == nil {
		return body
	}
	open := bytes.IndexByte(text[at[1]:], '{')
	if open < 0 {
		return body
	}
	end := closing(text, at[1]+open)
	for i := at[0]; i < end; i++ {
		if text[i] != '\n' {
			text[i] = ' '
		}
	}
	out := slices.Clone(body)
	for i, l := range strings.Split(string(text), "\n") {
		if i < len(out) {
			out[i].text = l
		}
	}
	return out
}

func closing(text []byte, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch {
		case text[i] == '/' && i+1 < len(text) && text[i+1] == '/':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case text[i] == '/' && i+1 < len(text) && text[i+1] == '*':
			j := bytes.Index(text[i+2:], []byte("*/"))
			if j < 0 {
				return len(text)
			}
			i += j + 3
		case text[i] == '{':
			depth++
		case text[i] == '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(text)
}
