#version 330 core
uniform sampler2D tex0;
layout(std140) uniform ub0 { vec4 params; };
in vec2 uv;
out vec4 color;
vec3 encodeSRGB(vec3 x) {
    return mix(12.92*x,1.055*pow(max(x,vec3(0.0)),vec3(1.0/2.4))-0.055,step(vec3(0.0031308),x));
}
void main() {
    vec4 sampleColor=texture(tex0,uv);
    float coverage=clamp(sampleColor.a,0.0,1.0);
    vec3 straight=coverage>1e-8 ? max(sampleColor.rgb,vec3(0.0))/coverage : vec3(0.0);
    straight*=params.x;
    if (params.y>0.5) { straight=straight/(vec3(1.0)+straight); }
    straight=clamp(straight,vec3(0.0),vec3(1.0));
    // RGBA8 display outputs need explicit encoding. sRGB attachments and the
    // browser/native presentation path perform their own single conversion.
    if (params.w>0.5) { straight=encodeSRGB(straight); }
    color=vec4(straight*coverage,coverage);
}
