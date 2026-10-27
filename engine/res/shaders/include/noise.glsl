float aego_hash(vec2 p) {
    p = fract(p * vec2(123.34, 456.21));
    p += dot(p, p + 45.32);
    return fract(p.x * p.y);
}

float aego_noise(vec2 p) {
    vec2 i = floor(p);
    vec2 f = fract(p);
    vec2 u = f * f * (3.0 - 2.0 * f);
    return mix(mix(aego_hash(i), aego_hash(i + vec2(1.0, 0.0)), u.x),
               mix(aego_hash(i + vec2(0.0, 1.0)), aego_hash(i + vec2(1.0, 1.0)), u.x), u.y);
}
