// All returned colours are linear and already premultiplied. Additional opacity
// scales the returned vec4 once; never multiply its RGB by its alpha again.
vec2 aego_texel() {
    return 1.0 / vec2(textureSize(aego_main, 0));
}

// Atlas coordinates are clamped to the texel centres of this region. Samples
// outside it are transparent, so neighbouring atlas tiles cannot enter outlines.
vec4 aego_sample_at(vec2 uv) {
    vec2 lo = min(aego_region.xy, aego_region.zw);
    vec2 hi = max(aego_region.xy, aego_region.zw);
    if (any(lessThan(uv, lo)) || any(greaterThan(uv, hi))) {
        return vec4(0.0);
    }
    vec2 halfTexel = min(aego_texel() * 0.5, (hi - lo) * 0.5);
    return texture(aego_main, clamp(uv, lo + halfTexel, hi - halfTexel)) * aego_color;
}

vec4 aego_sample() {
    return aego_sample_at(aego_uv);
}

// Local UV addresses the original, untrimmed sprite and follows its flips.
// Material textures such as noise use aego_local directly with their own sampler.
// Transparent cropped margins are outside local_rect, not stretched atlas data.
vec4 aego_sample_local(vec2 uv) {
    vec2 extent = aego_local_rect.zw - aego_local_rect.xy;
    if (extent.x == 0.0 || extent.y == 0.0) {
        return vec4(0.0);
    }
    vec2 regionLocal = (uv - aego_local_rect.xy) / extent;
    if (any(lessThan(regionLocal, vec2(0.0))) || any(greaterThan(regionLocal, vec2(1.0)))) {
        return vec4(0.0);
    }
    return aego_sample_at(mix(aego_region.xy, aego_region.zw, regionLocal));
}
