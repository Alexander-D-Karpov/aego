#version 330 core

in vec2 v_uv;
in vec4 v_color;

uniform sampler2D tex0;

out vec4 o_color;

void main() {
    o_color = texture(tex0, v_uv) * v_color;
}
