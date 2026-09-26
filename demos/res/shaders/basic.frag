#version 330 core

layout(std140) uniform ub0 {
    mat4 u_mvp;
    vec4 u_tint;
};

uniform sampler2D tex0;

in vec2 v_uv;
out vec4 o_color;

void main() {
    o_color = texture(tex0, v_uv) * u_tint;
}