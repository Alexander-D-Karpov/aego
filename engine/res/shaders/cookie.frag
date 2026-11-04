#version 330 core
uniform sampler2D tex0;
in vec2 uv;
flat in vec4 bounds;
out vec4 color;
void main() {
    vec2 halfTexel=0.5/vec2(textureSize(tex0,0));
    float mask=texture(tex0,clamp(uv,min(bounds.xy,bounds.zw)+halfTexel,max(bounds.xy,bounds.zw)-halfTexel)).r;
    color=vec4(mask,0.0,0.0,1.0);
}
