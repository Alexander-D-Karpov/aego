package shader

// Lighting optionally extends ABI 1: array elements are vec4 with std140 stride 16 independent of Go struct layout; main/normal/user[6]/shadow-array/cookie-array/emission use 0/1/2..7/8/9/10.
const (
	LightingMaxLights = 8
	LightingSize      = 16 + 5*LightingMaxLights*16
	SurfaceSize       = 80
)

const LightingBlock = `layout(std140) uniform ub4 {
    vec4 aego_ambient_count;
    vec4 aego_light_pose[8];
    vec4 aego_light_color[8];
    vec4 aego_light_cone[8];
    vec4 aego_light_shape[8];
    vec4 aego_light_cookie[8];
};
uniform sampler2D tex1;
uniform highp sampler2DArray tex8;
uniform highp sampler2DArray tex9;
#define aego_normal_map tex1
#define aego_shadow_masks tex8
#define aego_cookies tex9`

// The first two fields retain the AESH3 Mesh2D normal layout. AESH4 extends it
// with independent emission mapping and a per-batch semantic discriminator.
const SurfaceBlock = `layout(std140) uniform ub5 {
    vec4 aego_surface_normal_uv;
    vec4 aego_surface_normal_local;
    vec4 aego_surface_emission_uv;
    vec4 aego_surface_emission_local;
    vec4 aego_surface_channel_modes;
};`

const EmissionBinding = `uniform sampler2D tex10;
#define aego_emission_map tex10`

var SurfaceOffsets = [...]struct {
	Name   string
	Offset int
}{
	{"aego_surface_normal_uv", 0},
	{"aego_surface_normal_local", 16},
	{"aego_surface_emission_uv", 32},
	{"aego_surface_emission_local", 48},
	{"aego_surface_channel_modes", 64},
}

// PutLightingVec4 writes an explicitly chosen std140 element, not a struct copy.
func PutLightingVec4(dst []byte, offset int, value [4]float32) {
	for i, v := range value {
		put(dst[offset+i*4:], v)
	}
}

// LightingOffsets exposes the expected layout for CPU tests and linked GPU
// reflection. The shader may optimize whole unused groups, but never move them.
var LightingOffsets = [...]struct {
	Name   string
	Offset int
}{
	{"aego_ambient_count", 0},
	{"aego_light_pose[0]", 16},
	{"aego_light_color[0]", 144},
	{"aego_light_cone[0]", 272},
	{"aego_light_shape[0]", 400},
	{"aego_light_cookie[0]", 528},
}
