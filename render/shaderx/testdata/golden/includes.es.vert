#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
#define AEGO_ABI 1
#define AEGO_FRAGMENT 0
#define AEGO_VERTEX 1
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
layout(location = 0) in vec4 aego_i_pos_size;
layout(location = 1) in vec4 aego_i_sincos_origin;
layout(location = 2) in vec4 aego_i_uv;
layout(location = 3) in vec4 aego_i_color;
out vec2 aego_uv;
out vec4 aego_color;
out vec2 aego_local;
vec3 aego_world;
void main() {
    vec2 corner = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
    vec2 local = (corner - aego_i_sincos_origin.zw) * aego_i_pos_size.zw;
    float s = aego_i_sincos_origin.x;
    float c = aego_i_sincos_origin.y;
    aego_world = vec3(aego_i_pos_size.xy + vec2(local.x * c - local.y * s, local.x * s + local.y * c), 0.0);
    aego_uv = mix(aego_i_uv.xy, aego_i_uv.zw, corner);
    aego_color = aego_i_color;
    aego_local = corner;
    gl_Position = aego_view_proj * vec4(aego_world, 1.0);
}
