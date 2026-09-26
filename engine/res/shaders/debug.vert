#version 330 core

layout(location = 0) in vec2 a_pos;
layout(location = 1) in vec4 a_color;

layout(std140) uniform ub0 {
    mat4 u_view_proj;
};

out vec4 v_color;

void main() {
    v_color = a_color;
    gl_Position = u_view_proj * vec4(a_pos, 0.0, 1.0);
}