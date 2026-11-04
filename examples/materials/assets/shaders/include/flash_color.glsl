// Edit this include to test transitive hot reload. Color is linear premultiplied.
vec3 demo_flash(vec3 source, vec3 target, float amount) {
    return mix(source, target, amount);
}
