package shaderx

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"aego/render/shader"
)

var genLoc = Loc{File: -1}

// GeneratorVersion invalidates import caches when generated ABI glue changes
// without changing a project's source or any of its includes.
const GeneratorVersion = 8

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
layout(location = 3) in vec4 aego_i_color;
layout(location = 9) in vec4 aego_i_affine;
layout(location = %d) in vec4 aego_i_local_rect;`

const spriteMain = `    vec2 corner = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
    vec2 local = (corner - aego_i_sincos_origin.zw) * aego_i_pos_size.zw;
    float s = aego_i_sincos_origin.x;
    float c = aego_i_sincos_origin.y;
    mat2 aego_basis = mat2(1.0 + aego_i_affine.x, aego_i_affine.y, aego_i_affine.z, aego_i_affine.w);
    aego_world = vec3(aego_i_pos_size.xy + aego_basis * vec2(local.x * c - local.y * s, local.x * s + local.y * c), 0.0);
    aego_uv = mix(aego_i_uv.xy, aego_i_uv.zw, corner);
    aego_color = aego_i_color;
    aego_local_rect = aego_i_local_rect;`

const spriteLocalCoordinates = `    if (aego_local_rect.x == aego_local_rect.z || aego_local_rect.y == aego_local_rect.w) {
        // A disabled optional attribute has the generic value (0,0,0,1).
        aego_local_rect = vec4(0.0, 0.0, 1.0, 1.0);
        if (aego_i_uv.x > aego_i_uv.z) { aego_local_rect.xz = vec2(1.0, 0.0); }
        if (aego_i_uv.y > aego_i_uv.w) { aego_local_rect.yw = vec2(1.0, 0.0); }
    }
    aego_local = mix(aego_local_rect.xy, aego_local_rect.zw, corner);
    aego_region = aego_i_uv;`

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
    aego_local = aego_i_uv;
    aego_local_rect = vec4(0.0, 0.0, 1.0, 1.0);
    aego_region = aego_uv_rect;`

// Stage outputs cannot have GLSL initializers. Keep initialized private working
// values for custom vertex/helper code, then publish to the unchanged varyings.
// These macros name only the reserved engine ABI, never user material fields.
const vertexVaryings = `out vec2 aego_uv;
out vec4 aego_color;
out vec2 aego_local;
flat out vec4 aego_region;
flat out vec4 aego_local_rect;
vec3 aego_world = vec3(0.0);
vec2 aego_vertex_uv = vec2(0.0);
vec4 aego_vertex_color = vec4(0.0);
vec2 aego_vertex_local = vec2(0.0);
vec4 aego_vertex_region = vec4(0.0);
vec4 aego_vertex_local_rect = vec4(0.0);
#define aego_uv aego_vertex_uv
#define aego_color aego_vertex_color
#define aego_local aego_vertex_local
#define aego_region aego_vertex_region
#define aego_local_rect aego_vertex_local_rect`

const publishVertexVaryings = `#undef aego_uv
#undef aego_color
#undef aego_local
#undef aego_region
#undef aego_local_rect
    aego_uv = aego_vertex_uv;
    aego_color = aego_vertex_color;
    aego_local = aego_vertex_local;
    aego_region = aego_vertex_region;
    aego_local_rect = aego_vertex_local_rect;`

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
	if u.Domain == shader.DomainPostFX {
		b.gen(shader.PostProcessUniforms)
		if !frag {
			b.gen(shader.PostProcessVertex)
		} else {
			u.uniforms(&b)
			b.gen(strings.Replace(shader.PostProcessFragment, "uniform sampler2D tex0;", "", 1))
			b.gen("#define MainTex tex0\n#define aego_uv uv\n#define aego_out color")
			for _, l := range u.body {
				b.add(l.text, l.loc)
			}
			b.gen("vec4 aego_effect(vec4 c,vec2 localUV,vec2 size,vec4 p[4]){fragment();return color;}")
			b.gen(shader.PostProcessMain)
		}
		return b.sb.String(), b.locs
	}
	b.gen(shader.FrameBlock)
	b.gen(shader.ViewBlock)
	if u.Features&shader.FeatureLighting != 0 {
		b.gen("#define AEGO_LIGHTING 1")
		if frag {
			b.gen(shader.LightingBlock)
		}
		if u.Features&shader.FeatureSurfaceChannels != 0 {
			b.gen("#define AEGO_SURFACE_CHANNELS 1")
			b.gen(shader.SurfaceBlock)
			if frag {
				b.gen(shader.EmissionBinding)
			}
		}
	}
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
			if !q.Normal && !q.Emission {
				b.add(fmt.Sprintf("uniform sampler2D tex%d;", q.Slot), loc(q))
			}
			b.gen(fmt.Sprintf("#define %s tex%d", q.Name, q.Slot))
		}
	}
}

// Instance parameters are real typed GLSL variables. Their values are resolved
// from the prepared effective per-instance lanes before either entry point runs;
// local variables and structure fields with the same spelling keep GLSL scope.
func (u *Unit) laneDefines(b *builder) {
	for _, q := range u.Params {
		if q.Instance {
			zero := "0.0"
			switch q.Kind {
			case shader.KindInt:
				zero = "0"
			case shader.KindBool:
				zero = "false"
			default:
				if q.Kind.Lanes() > 1 {
					zero = q.Kind.GLSL() + "(0.0)"
				}
			}
			// A constant initializer also makes helper-function reads definite
			// to drivers whose warnings do not follow the main() call graph.
			// assignLanes still installs the effective value before either entry.
			b.gen(fmt.Sprintf("%s %s = %s;", q.Kind.GLSL(), q.Name, zero))
		}
	}
}

func (u *Unit) assignLanes(b *builder) {
	for _, q := range u.Params {
		if !q.Instance {
			continue
		}
		access := fmt.Sprintf("aego_lane%d.%s", q.Lane, swizzle[q.Comp:q.Comp+q.Kind.Lanes()])
		switch q.Kind {
		case shader.KindInt:
			access = "int(" + access + ")"
		case shader.KindBool:
			access = "(" + access + " != 0u)"
		default:
			access = "uintBitsToFloat(" + access + ")"
		}
		b.gen(fmt.Sprintf("    %s = %s;", q.Name, access))
	}
}

func (u *Unit) fragment(b *builder) {
	if u.Features&shader.FeaturePixelCoverage != 0 {
		b.gen("in vec2 aego_coverage;")
	}
	b.gen("in vec2 aego_uv;\nin vec4 aego_color;\nin vec2 aego_local;\nflat in vec4 aego_region;\nflat in vec4 aego_local_rect;")
	if u.Features&shader.FeatureLighting != 0 {
		b.gen("in vec3 aego_surface_world;\nflat in vec4 aego_surface_basis;\nflat in vec4 aego_normal_region;\nflat in vec4 aego_normal_local_rect;")
		if u.Features&shader.FeatureSurfaceChannels != 0 {
			b.gen("flat in vec4 aego_emission_region;\nflat in vec4 aego_emission_local_rect;")
		}
		if u.Domain == shader.DomainMesh2D {
			b.gen("#define AEGO_MESH_SURFACE 1\nin vec2 aego_surface_local_position;")
		}
	}
	for i := 0; i < u.Lanes; i++ {
		b.gen(fmt.Sprintf("flat in uvec4 aego_lane%d;", i))
	}
	u.laneDefines(b)
	b.gen("out vec4 aego_out;")
	for _, l := range strip(u.body, reVert) {
		b.add(l.text, l.loc)
	}
	b.gen("void main() {")
	if u.Features&shader.FeaturePixelCoverage != 0 {
		b.gen("    if (any(lessThan(aego_coverage,vec2(0))) || any(greaterThanEqual(aego_coverage,vec2(1)))) discard;")
	}
	u.assignLanes(b)
	b.gen("    fragment();\n}")
}

func (u *Unit) vertex(b *builder) {
	coverage := u.Features&shader.FeaturePixelCoverage != 0
	if coverage {
		b.gen("out vec2 aego_coverage;")
	}
	mesh := u.Domain == shader.DomainMesh2D
	if mesh {
		b.gen(meshInputs)
	} else {
		b.gen(fmt.Sprintf(spriteInputs, shader.TrimLocation))
	}
	for i := 0; i < u.Lanes; i++ {
		b.gen(fmt.Sprintf("layout(location = %d) in uvec4 aego_i_lane%d;\nflat out uvec4 aego_lane%d;", shader.LaneLocation+i, i, i))
	}
	u.laneDefines(b)
	b.gen(vertexVaryings)
	if u.Features&shader.FeatureLighting != 0 {
		b.gen("out vec3 aego_surface_world;\nflat out vec4 aego_surface_basis;\nflat out vec4 aego_normal_region;\nflat out vec4 aego_normal_local_rect;")
		if u.Features&shader.FeatureSurfaceChannels != 0 {
			b.gen("flat out vec4 aego_emission_region;\nflat out vec4 aego_emission_local_rect;")
		}
		if mesh {
			if u.Features&shader.FeatureSurfaceChannels == 0 {
				b.gen("layout(std140) uniform ub5 { vec4 aego_surface_normal_uv; vec4 aego_surface_normal_local; };")
			}
			b.gen("out vec2 aego_surface_local_position;")
		} else {
			b.gen("layout(location = 10) in vec4 aego_i_normal_uv;\nlayout(location = 11) in vec4 aego_i_normal_local;")
			if u.Features&shader.FeatureSurfaceChannels != 0 {
				b.gen("layout(location = 12) in vec4 aego_i_emission_uv;\nlayout(location = 13) in vec4 aego_i_emission_local;")
			}
		}
	}
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
		if coverage {
			b.gen("    if (aego_i_local_rect.x >= 2.0) aego_local_rect.x -= 2.0;")
		}
		b.gen(spriteLocalCoordinates)
	}
	for i := 0; i < u.Lanes; i++ {
		b.gen(fmt.Sprintf("    aego_lane%d = aego_i_lane%d;", i, i))
	}
	u.assignLanes(b)
	if u.Vertex {
		b.gen("    vertex();")
	}
	if u.Features&shader.FeatureLighting != 0 {
		b.gen("    aego_surface_world = aego_world;")
		if mesh {
			b.gen("    aego_surface_basis = vec4(aego_model[0].xy,aego_model[1].xy);\n    aego_normal_region = aego_surface_normal_uv;\n    aego_normal_local_rect = aego_surface_normal_local;\n    aego_surface_local_position = aego_i_pos.xy;")
			if u.Features&shader.FeatureSurfaceChannels != 0 {
				b.gen("    aego_emission_region = aego_surface_emission_uv;\n    aego_emission_local_rect = aego_surface_emission_local;")
			}
		} else {
			b.gen("    aego_surface_basis = vec4(aego_basis[0],aego_basis[1]);\n    aego_normal_region = aego_i_normal_uv;\n    aego_normal_local_rect = aego_i_normal_local;")
			if u.Features&shader.FeatureSurfaceChannels != 0 {
				b.gen("    aego_emission_region = aego_i_emission_uv;\n    aego_emission_local_rect = aego_i_emission_local;")
			}
		}
	}
	b.gen(publishVertexVaryings)
	b.gen("    gl_Position = aego_view_proj * vec4(aego_world, 1.0);")
	if coverage {
		b.gen(pixelCoverage)
	}
	b.gen("}")
}

func strip(body []srcLine, entry *regexp.Regexp) []srcLine {
	text := []byte(joinBody(body))
	name := "fragment"
	if entry == reVert {
		name = "vertex"
	}
	for {
		start, end, found := entryRange(string(text), name)
		if !found {
			break
		}
		for i := start; i < end; i++ {
			if text[i] != '\n' {
				text[i] = ' '
			}
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

// Rasterizing on integer pixel edges gives coverage and texture interpolation
// the same pixel centres across drivers with different subpixel precision.
// Leave very large or highly stretched bounds to the regular rasterizer.
const pixelCoverage = `    aego_coverage = vec2(.5);
    if (aego_i_local_rect.x >= 2.0 && gl_Position.w == 1.0) {
        vec2 halfView = .5 * aego_viewport.zw;
        vec2 dx = (aego_view_proj * vec4(aego_basis * (vec2(c,s)*aego_i_pos_size.z),0,0)).xy * halfView;
        vec2 dy = (aego_view_proj * vec4(aego_basis * (vec2(-s,c)*aego_i_pos_size.w),0,0)).xy * halfView;
        mat2 edges = mat2(dx,dy);
        float area = abs(determinant(edges));
        vec2 origin = (gl_Position.xy + 1.0)*halfView - edges*corner;
        vec2 lo = floor(origin + min(dx,vec2(0)) + min(dy,vec2(0)));
        vec2 hi = ceil(origin + max(dx,vec2(0)) + max(dy,vec2(0)));
        vec2 extent = hi-lo;
        if (area > .000001 && extent.x*extent.y <= max(64.0,8.0*area) && all(greaterThan(halfView,vec2(0)))) {
            vec2 point = mix(lo,hi,corner);
            aego_coverage = inverse(edges)*(point-origin);
            aego_uv = mix(aego_i_uv.xy,aego_i_uv.zw,aego_coverage);
            aego_local = mix(aego_local_rect.xy,aego_local_rect.zw,aego_coverage);
            gl_Position.xy = point/halfView - 1.0;
        }
    }`
