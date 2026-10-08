#version 330 core

layout(location = 0) in vec3 a_pos;
layout(location = 1) in vec3 a_normal;
layout(location = 2) in vec2 a_uv;
layout(location = 3) in vec4 a_color;

layout(std140) uniform ub0 {
    mat4 u_view_proj;
};

layout(std140) uniform ub1 {
    mat4 u_world;
    vec4 u_color;
    vec4 u_uv_rect;
};

out vec2 v_uv;
out vec4 v_color;

void main() {
    v_uv = mix(u_uv_rect.xy, u_uv_rect.zw, a_uv);
    v_color = a_color * u_color;
    gl_Position = u_view_proj * u_world * vec4(a_pos, 1.0);
}
