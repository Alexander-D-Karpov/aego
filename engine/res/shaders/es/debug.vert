#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
layout(location = 0) in vec3 a_pos;
layout(location = 1) in vec4 a_color;

layout(std140) uniform ub0 {
    mat4 u_view_proj;
};

out vec4 v_color;

void main() {
    v_color = a_color;
    gl_Position = u_view_proj * vec4(a_pos, 1.0);
}
