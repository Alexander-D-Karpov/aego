#version 330 core
layout(std140) uniform ub0 { vec4 region; vec4 localRect; vec4 fit; };
out vec2 uv;
flat out vec4 bounds;
void main() {
    vec2 corner=vec2(float(gl_VertexID&1),float((gl_VertexID>>1)&1));
    vec2 local=mix(localRect.xy,localRect.zw,corner);
    gl_Position=vec4((local-0.5)*2.0*fit.xy,0.0,1.0);
    uv=mix(region.xy,region.zw,corner);
    bounds=region;
}
