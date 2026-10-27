#version 330 core

layout(location = 0) in vec3 i_pos;
layout(location = 1) in vec3 i_right;
layout(location = 2) in vec3 i_down;
layout(location = 3) in vec4 i_uv;
layout(location = 4) in vec4 i_color;

layout(std140) uniform ub1 {
    mat4 u_view_proj;
};

out vec2 v_uv;
out vec4 v_color;

void main() {
    vec2 corner = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
    vec3 p = i_pos + i_right * corner.x + i_down * corner.y;
    v_uv = mix(i_uv.xy, i_uv.zw, corner);
    v_color = i_color;
    gl_Position = u_view_proj * vec4(p, 1.0);
}
