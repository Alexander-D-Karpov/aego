// Forward 2D: each receiver evaluates sources with its own surface normal.
// Normal images are linear, +Y image-up; source-local UV is image-down.
#if AEGO_FRAGMENT && defined(AEGO_LIGHTING)
vec3 aego_surface_normal(float strength) {
#ifdef AEGO_MESH_SURFACE
    // Derivatives must be evaluated before per-fragment early returns: GLSL
    // leaves them undefined in non-uniform control flow around trim edges.
    vec2 dx = dFdx(aego_surface_local_position);
    vec2 dy = dFdy(aego_surface_local_position);
    vec2 ux = dFdx(aego_local);
    vec2 uy = dFdy(aego_local);
#endif
    vec2 span = aego_normal_local_rect.zw - aego_normal_local_rect.xy;
    vec2 uvspan = aego_normal_region.zw - aego_normal_region.xy;
    if (abs(span.x * span.y) < 1e-8 || abs(uvspan.x * uvspan.y) < 1e-12) {
        return vec3(0.0, 0.0, 1.0);
    }
    vec2 local = (aego_local - aego_normal_local_rect.xy) / span;
    if (any(lessThan(local,vec2(0.0))) || any(greaterThan(local,vec2(1.0)))) {
        return vec3(0.0, 0.0, 1.0);
    }
    vec2 halfTexel = 0.5 / vec2(textureSize(aego_normal_map,0));
    vec2 lo = min(aego_normal_region.xy,aego_normal_region.zw) + halfTexel;
    vec2 hi = max(aego_normal_region.xy,aego_normal_region.zw) - halfTexel;
    vec2 uv = clamp(mix(aego_normal_region.xy,aego_normal_region.zw,local),lo,hi);
    vec3 n = texture(aego_normal_map,uv).xyz * 2.0 - 1.0;
    n.xy *= max(strength,0.0);
    if (dot(n,n) < 1e-10) { n = vec3(0.0,0.0,1.0); }
    n = normalize(n);
#ifdef AEGO_MESH_SURFACE
    // Reconstruct the basis from actual geometry and UV. Degenerate UV has an
    // explicit flat fallback, never a NaN or an arbitrary highlight direction.
    float detUV = ux.x * uy.y - ux.y * uy.x;
    if (abs(detUV) < 1e-10) { return vec3(0.0,0.0,1.0); }
    vec2 tx = (dx * uy.y - dy * ux.y) / detUV;
    vec2 ty = (dy * ux.x - dx * uy.x) / detUV;
    if (dot(tx,tx)<1e-10 || dot(ty,ty)<1e-10) { return vec3(0.0,0.0,1.0); }
    float orientation = tx.x*ty.y-tx.y*ty.x;
    if (abs(orientation)<1e-10) { return vec3(0.0,0.0,1.0); }
    tx = normalize(tx);
    ty = vec2(-tx.y,tx.x)*sign(orientation);
    mat2 basis = mat2(aego_surface_basis.xy,aego_surface_basis.zw)*mat2(tx,ty);
#else
    mat2 basis = mat2(aego_surface_basis.xy,aego_surface_basis.zw);
    // UV sampling is already reflected. Reflect vector orientation exactly
    // once, independently from a reflected parent/node affine matrix.
    n.xy *= sign(aego_local_rect.zw - aego_local_rect.xy);
#endif
    if (abs(determinant(basis)) < 1e-8) { return vec3(0.0,0.0,1.0); }
    vec2 xy = transpose(inverse(basis)) * vec2(n.x,-n.y);
    return normalize(vec3(xy,n.z));
}

float aego_shadow_visibility(int i, vec2 position) {
    float layer = aego_light_shape[i].z;
    if (layer < 0.0) { return 1.0; }
    vec2 uv = (position-aego_light_pose[i].xy)/(2.0*aego_light_color[i].w)+0.5;
    if (uv.y < aego_light_cookie[i].y || any(lessThan(uv,vec2(0.0))) || any(greaterThan(uv,vec2(1.0)))) { return 1.0; }
    float radius = aego_light_shape[i].w;
    if (radius <= 0.0) { return texture(aego_shadow_masks,vec3(uv,layer)).r; }
    // Limited mask filtering; all taps use the same array layer.
    float visibility = 0.0;
    for (int y=-1;y<=1;y++) {
        for (int x=-1;x<=1;x++) {
            visibility += texture(aego_shadow_masks,vec3(clamp(uv+vec2(float(x),float(y))*radius,vec2(0.0),vec2(1.0)),layer)).r;
        }
    }
    return visibility/9.0;
}

// A typed call fixes only per-draw control flow. All light parameters, shadow
// and cookie samples, attenuation arithmetic and accumulation order stay live.
void aego_apply_light(int i, int lightType, bool shadowed, vec3 normal, bool receiveShadows, inout vec3 result) {
    vec4 pose = aego_light_pose[i];
    vec4 color = aego_light_color[i];
    vec4 cone = aego_light_cone[i];
    vec4 shape = aego_light_shape[i];
    vec3 direction;
    float attenuation = 1.0;
    if (lightType==2) {
        direction = vec3(-cone.xy*cos(shape.y),sin(shape.y));
    } else {
        vec2 delta = pose.xy-aego_surface_world.xy;
        float distanceXY = length(delta);
        if (distanceXY >= color.w) { return; }
        attenuation = pow(max(0.0,1.0-distanceXY/color.w),shape.x);
        vec3 toward = vec3(delta,pose.z);
        float lengthSquared = dot(toward,toward);
        direction = lengthSquared > 1e-12 ? toward*inversesqrt(lengthSquared) : vec3(0.0,0.0,1.0);
        if (lightType==1) {
            float cosine = distanceXY > 1e-6 ? dot(-delta/distanceXY,cone.xy) : 1.0;
            attenuation *= cone.z > cone.w ? smoothstep(cone.w,cone.z,cosine) : step(cone.w,cosine);
        }
        if (lightType==3) {
            float cookieLayer = aego_light_cookie[i].x;
            if (cookieLayer < 0.0) { return; }
            vec2 local = vec2(dot(-delta,cone.xy),dot(-delta,vec2(-cone.y,cone.x)))/(2.0*color.w)+0.5;
            attenuation *= texture(aego_cookies,vec3(clamp(local,vec2(0.0),vec2(1.0)),cookieLayer)).r;
        }
        // The direct term is exactly zero behind a receiver or outside its
        // spot/cookie. Skip the PCF reads for those fragments.
        if (attenuation == 0.0 || dot(normal,direction) <= 0.0) { return; }
        if (receiveShadows && shadowed) { attenuation *= aego_shadow_visibility(i,aego_surface_world.xy); }
    }
    result += color.rgb * (attenuation * max(0.0,dot(normal,direction)));
}

vec3 aego_lighting(vec3 normal, bool receiveShadows) {
    vec3 result = aego_ambient_count.rgb;
#ifdef AEGO_TYPED_LIGHT_CALLS
    AEGO_TYPED_LIGHT_CALLS
#else
    for (int i=0;i<8;i++) {
        if (i>=int(aego_ambient_count.w)) { break; }
        aego_apply_light(i, int(aego_light_pose[i].w), true, normal, receiveShadows, result);
    }
#endif
    return result;
}

// Emission RGB is straight color; an emission mask is an independent scalar.
// The source canvas, trim and atlas rectangle belong to this channel, not to
// albedo. Coverage is applied exactly once by aego_lit, from the albedo alpha.
vec3 aego_emission(vec3 color, float intensity) {
    vec3 value = color * max(intensity,0.0);
#ifdef AEGO_SURFACE_CHANNELS
    int mode = int(aego_surface_channel_modes.x);
    if (mode == 0) { return value; }
    vec2 span = aego_emission_local_rect.zw-aego_emission_local_rect.xy;
    vec2 uvspan = aego_emission_region.zw-aego_emission_region.xy;
    if (abs(span.x*span.y)<1e-8 || abs(uvspan.x*uvspan.y)<1e-12) { return vec3(0.0); }
    vec2 local = (aego_local-aego_emission_local_rect.xy)/span;
    if (any(lessThan(local,vec2(0.0))) || any(greaterThan(local,vec2(1.0)))) { return vec3(0.0); }
    vec2 halfTexel = 0.5/vec2(textureSize(aego_emission_map,0));
    vec2 lo = min(aego_emission_region.xy,aego_emission_region.zw)+halfTexel;
    vec2 hi = max(aego_emission_region.xy,aego_emission_region.zw)-halfTexel;
    vec2 uv = clamp(mix(aego_emission_region.xy,aego_emission_region.zw,local),lo,hi);
    vec3 sampleColor = texture(aego_emission_map,uv).rgb;
    value *= mode == 2 ? vec3(sampleColor.r) : sampleColor;
#endif
    return value;
}

vec4 aego_lit(vec4 premultipliedAlbedo, float normalStrength, vec3 emission, bool receiveShadows) {
    vec3 normal = aego_surface_normal(normalStrength);
    int debugMode = int(aego_light_cookie[0].w);
    if (debugMode==3) { return premultipliedAlbedo; }
    if (debugMode==1) { return vec4((normal*0.5+0.5)*premultipliedAlbedo.a,premultipliedAlbedo.a); }
    if (debugMode==2) {
        float visibility = 1.0;
        for (int i=0;i<8;i++) {
            if (i>=int(aego_ambient_count.w)) { break; }
            visibility = min(visibility,aego_shadow_visibility(i,aego_surface_world.xy));
        }
        return vec4(vec3(visibility)*premultipliedAlbedo.a,premultipliedAlbedo.a);
    }
    vec3 rgb = premultipliedAlbedo.rgb*aego_lighting(normal,receiveShadows);
    rgb += emission*premultipliedAlbedo.a;
    return vec4(rgb,premultipliedAlbedo.a);
}
#endif
