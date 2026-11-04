package shader

// PostFX block 0 is the output-pass ABI, not the sprite frame block.
const PostProcessUniforms = `
layout(std140) uniform ub0 {
    vec4 outputParams;
    vec4 sourceRect;
    vec4 phases;
    vec4 effectParams[4];
    vec4 auxInfo;
};
`

const PostProcessVertex = `
out vec2 uv;
void main() {
    uv = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
    gl_Position = vec4((uv * 2.0 - 1.0) * vec2(1.0, outputParams.z), 0.0, 1.0);
}
`

const postProcessSampling = `
uniform sampler2D tex0;
uniform sampler2D tex1;
in vec2 uv;
out vec4 color;

vec4 toDisplay(vec4 c) {
    float a = clamp(c.a, 0.0, 1.0);
    vec3 rgb = a > 1e-8 ? max(c.rgb, vec3(0)) / a : vec3(0);
    rgb *= outputParams.x;
    if (outputParams.y > 0.5) rgb /= vec3(1) + rgb;
    return vec4(clamp(rgb, vec3(0), vec3(1)) * a, a);
}

vec4 aego_sample(vec2 localUV) {
    vec2 extent = vec2(textureSize(tex0, 0));
    vec2 pixel = sourceRect.xy + clamp(localUV * sourceRect.zw,vec2(.5),max(vec2(.5),sourceRect.zw-.5));
    vec2 sampleUV = pixel/extent;
    vec4 c = texture(tex0, sampleUV);
#ifdef AEGO_AUX_PASS
    return c;
#else
    return phases.x > 0.5 ? toDisplay(c) : c;
#endif
}
`

// PostProcessFragment is also used to build author materials. Keep their
// sampler unchanged. The bounded variant is selected only for builtin passes
// whose auxiliary kernel supplies the black RGB guard.
const PostProcessFragment = postProcessSampling + `vec4 aego_aux(vec2 u){return texture(tex1,clamp(u*auxInfo.xy,vec2(.5),auxInfo.xy-.5)/vec2(textureSize(tex1,0)));}

`

const PostProcessBoundedAux = postProcessSampling + `vec4 aego_aux(vec2 u){
    vec2 pixel = clamp(u*auxInfo.xy,vec2(.5),auxInfo.xy-.5);
    pixel = clamp(pixel,phases.zw,auxInfo.zw);
    return texture(tex1,pixel/vec2(textureSize(tex1,0)));
}

`

const PostProcessMain = `
void main() {
    vec4 c = aego_effect(aego_sample(uv), uv, sourceRect.zw, effectParams);
#ifndef AEGO_AUX_PASS
    if (phases.y > 0.5) c = toDisplay(c);
    if (outputParams.w > 0.5) {
        vec3 rgb = c.a > 1e-8 ? max(c.rgb, vec3(0)) / c.a : vec3(0);
        c.rgb = mix(12.92*rgb, 1.055*pow(rgb, vec3(1.0/2.4))-0.055, step(vec3(0.0031308),rgb)) * c.a;
    }
#endif
    color = c;
}
`
