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
layout(std140) uniform ub2 {
    float Speed;
    vec2 Offset;
    vec3 Glow;
    vec4 Tint;
    int Steps;
    bool Pulse;
};
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



void fragment() {
    float w = Pulse ? 0.5 + 0.5 * sin(aego_time * Speed) : 1.0;
    vec4 c = aego_sample() * Tint;
    aego_out = vec4(c.rgb * w + Glow * 0.1 * float(Steps), c.a) + vec4(Offset, 0.0, 0.0) * 0.0;
}

void main() {
    fragment();
}
