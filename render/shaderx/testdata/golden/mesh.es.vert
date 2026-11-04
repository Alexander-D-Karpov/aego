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
#define aego_clip_to_pixel_y aego_view_params.w
#define aego_camera_pos aego_camera.xyz
layout(std140) uniform ub2 {
    vec4 Fog;
};
uniform sampler2D tex0;
#define aego_main tex0
layout(location = 0) in vec3 aego_i_pos;
layout(location = 1) in vec3 aego_i_normal;
layout(location = 2) in vec2 aego_i_uv;
layout(location = 3) in vec4 aego_i_color;
layout(std140) uniform ub3 {
    mat4 aego_model;
    vec4 aego_tint;
    vec4 aego_uv_rect;
};
out vec2 aego_uv;
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
#define aego_local_rect aego_vertex_local_rect
void main() {
    aego_world = (aego_model * vec4(aego_i_pos, 1.0)).xyz;
    aego_uv = mix(aego_uv_rect.xy, aego_uv_rect.zw, aego_i_uv);
    aego_color = aego_i_color * aego_tint;
    aego_local = aego_i_uv;
    aego_local_rect = vec4(0.0, 0.0, 1.0, 1.0);
    aego_region = aego_uv_rect;
#undef aego_uv
#undef aego_color
#undef aego_local
#undef aego_region
#undef aego_local_rect
    aego_uv = aego_vertex_uv;
    aego_color = aego_vertex_color;
    aego_local = aego_vertex_local;
    aego_region = aego_vertex_region;
    aego_local_rect = aego_vertex_local_rect;
    gl_Position = aego_view_proj * vec4(aego_world, 1.0);
}
