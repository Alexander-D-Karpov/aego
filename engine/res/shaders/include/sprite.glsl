vec4 aego_sample() {
    return texture(aego_main, aego_uv) * aego_color;
}

vec4 aego_sample_at(vec2 uv) {
    return texture(aego_main, uv) * aego_color;
}

vec2 aego_texel() {
    return 1.0 / vec2(textureSize(aego_main, 0));
}
