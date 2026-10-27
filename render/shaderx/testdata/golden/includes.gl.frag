#version 330 core
#define AEGO_ABI 1
#define AEGO_FRAGMENT 1
#define AEGO_VERTEX 0
layout(std140) uniform ub0 {
    vec4 aego_frame_data;
};
#define aego_time aego_frame_data.x
#define aego_delta aego_frame_data.y
#define aego_unscaled aego_frame_data.z
#define aego_frame_index aego_frame_data.w
layout(std140) uniform ub1 {
    mat4 aego_view_proj;
    vec4 aego_viewport;
    vec4 aego_view_params;
    vec4 aego_camera;
};
#define aego_pixel_scale aego_view_params.x
#define aego_pixels_per_unit aego_view_params.y
#define aego_zoom aego_view_params.z
#define aego_camera_pos aego_camera.xyz
uniform sampler2D tex0;
#define aego_main tex0
in vec2 aego_uv;
in vec4 aego_color;
in vec2 aego_local;
out vec4 aego_out;
vec4 aego_sample() {
    return texture(aego_main, aego_uv) * aego_color;
}

vec4 aego_sample_at(vec2 uv) {
    return texture(aego_main, uv) * aego_color;
}

vec2 aego_texel() {
    return 1.0 / vec2(textureSize(aego_main, 0));
}

float aego_hash(vec2 p) {
    p = fract(p * vec2(123.34, 456.21));
    p += dot(p, p + 45.32);
    return fract(p.x * p.y);
}

float aego_noise(vec2 p) {
    vec2 i = floor(p);
    vec2 f = fract(p);
    vec2 u = f * f * (3.0 - 2.0 * f);
    return mix(mix(aego_hash(i), aego_hash(i + vec2(1.0, 0.0)), u.x),
               mix(aego_hash(i + vec2(0.0, 1.0)), aego_hash(i + vec2(1.0, 1.0)), u.x), u.y);
}

vec3 aego_srgb_to_linear(vec3 c) {
    return mix(c / 12.92, pow((c + 0.055) / 1.055, vec3(2.4)), step(vec3(0.04045), c));
}

vec3 aego_linear_to_srgb(vec3 c) {
    return mix(c * 12.92, 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055, step(vec3(0.0031308), c));
}

vec4 aego_premultiply(vec4 c) {
    return vec4(c.rgb * c.a, c.a);
}

vec4 aego_unpremultiply(vec4 c) {
    return c.a > 0.0 ? vec4(c.rgb / c.a, c.a) : vec4(0.0);
}

float aego_luma(vec3 c) {
    return dot(c, vec3(0.2126, 0.7152, 0.0722));
}

float wave(float x) {
    return 0.75 + 0.25 * sin(x * 6.2831);
}


void fragment() {
    vec4 c = aego_sample();
    float n = aego_noise(aego_local * 8.0 + aego_time);
    aego_out = aego_premultiply(vec4(aego_linear_to_srgb(c.rgb) * wave(n), c.a));
}

void main() {
    fragment();
}
