#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
layout(location=0) in vec2 position;
void main() { gl_Position=vec4(position*2.0-1.0,0.0,1.0); }
