#version 330 core

layout(location = 0) in vec2 a_pos;
layout(location = 1) in vec2 a_uv;

layout(std140) uniform ub0 {
    mat4 u_mvp;
    vec4 u_tint;
};

out vec2 v_uv;

void main() {
    v_uv = a_uv;
    gl_Position = u_mvp * vec4(a_pos, 0.0, 1.0);
}