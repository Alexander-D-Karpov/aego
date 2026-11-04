#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
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
#define aego_clip_to_pixel_y aego_view_params.w
#define aego_camera_pos aego_camera.xyz
layout(std140) uniform ub2 {
    float Strength;
};
uniform sampler2D tex0;
#define aego_main tex0
uniform sampler2D tex2;
#define Mask tex2
uniform sampler2D tex3;
#define Detail tex3
in vec2 aego_uv;
in vec4 aego_color;
in vec2 aego_local;
flat in vec4 aego_region;
flat in vec4 aego_local_rect;
out vec4 aego_out;


void fragment() {
    vec4 base = texture(aego_main, aego_uv) * aego_color;
    float m = texture(Mask, aego_uv).r;
    vec3 d = texture(Detail, aego_local).rgb;
    aego_out = vec4(mix(base.rgb, base.rgb * d, m * Strength), base.a);
}

void main() {
    fragment();
}
