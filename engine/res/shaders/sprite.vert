#version 330 core

layout(location = 0) in vec4 i_pos_size;
layout(location = 1) in vec4 i_sincos_origin;
layout(location = 2) in vec4 i_uv;
layout(location = 3) in vec4 i_color;
layout(location = 9) in vec4 i_affine;

layout(std140) uniform ub1 {
    mat4 u_view_proj;
};

out vec2 v_uv;
out vec4 v_color;

void main() {
    vec2 corner = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
    vec2 local = (corner - i_sincos_origin.zw) * i_pos_size.zw;
    float s = i_sincos_origin.x;
    float c = i_sincos_origin.y;
    vec2 rotated = vec2(local.x * c - local.y * s, local.x * s + local.y * c);
    rotated = mat2(1.0+i_affine.x,i_affine.y,i_affine.z,i_affine.w) * rotated;
    v_uv = mix(i_uv.xy, i_uv.zw, corner);
    v_color = i_color;
    gl_Position = u_view_proj * vec4(i_pos_size.xy + rotated, 0.0, 1.0);
}
