#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
layout(std140) uniform ub0 { vec4 params; };
out vec2 uv;
void main() {
    vec2 corner=vec2(float(gl_VertexID&1),float((gl_VertexID>>1)&1));
    uv=corner;
    gl_Position=vec4((corner*2.0-1.0)*vec2(1.0,params.z),0.0,1.0);
}
