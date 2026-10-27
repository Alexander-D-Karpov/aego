vec3 aego_srgb_to_linear(vec3 c) {
    return mix(c / 12.92, pow((c + 0.055) / 1.055, vec3(2.4)), step(vec3(0.04045), c));
}

vec3 aego_linear_to_srgb(vec3 c) {
    return mix(c * 12.92, 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055, step(vec3(0.0031308), c));
}

vec4 aego_premultiply(vec4 c) {
    return vec4(c.rgb * c.a, c.a);
}

vec4 aego_unpremultiply(vec4 c) {
    return c.a > 0.0 ? vec4(c.rgb / c.a, c.a) : vec4(0.0);
}

float aego_luma(vec3 c) {
    return dot(c, vec3(0.2126, 0.7152, 0.0722));
}
