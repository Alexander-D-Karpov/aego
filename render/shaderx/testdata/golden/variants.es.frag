#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
#define AEGO_ABI 1
#define AEGO_FRAGMENT 1
#define AEGO_VERTEX 0
#define OUTLINE 1
#define GRAY 1
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
#define aego_clip_to_pixel_y aego_view_params.w
#define aego_camera_pos aego_camera.xyz
uniform sampler2D tex0;
#define aego_main tex0
in vec2 aego_uv;
in vec4 aego_color;
in vec2 aego_local;
flat in vec4 aego_region;
flat in vec4 aego_local_rect;
out vec4 aego_out;
// All returned colours are linear and already premultiplied. Additional opacity
// scales the returned vec4 once; never multiply its RGB by its alpha again.
vec2 aego_texel() {
    return 1.0 / vec2(textureSize(aego_main, 0));
}

// Atlas coordinates are clamped to the texel centres of this region. Samples
// outside it are transparent, so neighbouring atlas tiles cannot enter outlines.
vec4 aego_sample_at(vec2 uv) {
    vec2 lo = min(aego_region.xy, aego_region.zw);
    vec2 hi = max(aego_region.xy, aego_region.zw);
    if (any(lessThan(uv, lo)) || any(greaterThan(uv, hi))) {
        return vec4(0.0);
    }
    vec2 halfTexel = min(aego_texel() * 0.5, (hi - lo) * 0.5);
    return texture(aego_main, clamp(uv, lo + halfTexel, hi - halfTexel)) * aego_color;
}

vec4 aego_sample() {
    return aego_sample_at(aego_uv);
}

// Local UV addresses the original, untrimmed sprite and follows its flips.
// Material textures such as noise use aego_local directly with their own sampler.
// Transparent cropped margins are outside local_rect, not stretched atlas data.
vec4 aego_sample_local(vec2 uv) {
    vec2 extent = aego_local_rect.zw - aego_local_rect.xy;
    if (extent.x == 0.0 || extent.y == 0.0) {
        return vec4(0.0);
    }
    vec2 regionLocal = (uv - aego_local_rect.xy) / extent;
    if (any(lessThan(regionLocal, vec2(0.0))) || any(greaterThan(regionLocal, vec2(1.0)))) {
        return vec4(0.0);
    }
    return aego_sample_at(mix(aego_region.xy, aego_region.zw, regionLocal));
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


void fragment() {
    vec4 c = aego_sample();
#if OUTLINE
    float a = texture(aego_main, aego_uv + vec2(aego_texel().x, 0.0)).a;
    c = mix(vec4(0.0, 0.0, 0.0, 1.0), c, step(0.5, c.a + (1.0 - a)));
#endif
#if GRAY
    c.rgb = vec3(aego_luma(c.rgb));
#endif
    aego_out = c;
}

void main() {
    fragment();
}
