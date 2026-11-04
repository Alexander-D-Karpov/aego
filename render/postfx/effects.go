package postfx

import (
	"aego/core/mathx"
	"aego/render/shader"
)

const (
	BloomID      = "postfx.bloom"
	ColorGradeID = "postfx.color-grading"
	PixelateID   = "postfx.pixelate"
	ChromaticID  = "postfx.chromatic-aberration"
	CRTID        = "postfx.crt"
	CustomID     = "postfx.custom"
)

func scalar(n string, v, lo, hi float32) Field {
	return Field{Name: n, Kind: shader.KindFloat, Default: Parameter{Value: mathx.Vec4{X: v}}, Min: lo, Max: hi, Bounded: true}
}
func rgb(n string, v, lo, hi float32) Field {
	f := scalar(n, v, lo, hi)
	f.Kind = shader.KindVec3
	f.Default.Value.Y = v
	f.Default.Value.Z = v
	return f
}
func slot(f Field, i, lane int) Field { f.Packed = true; f.Slot = i; f.Lane = lane; return f }

// Every sample is expressed in local viewport UV. The renderer clamps to the
// active rectangle, not to the bucket-sized backing texture.
func effects() []Descriptor {
	return []Descriptor{
		{ID: BloomID, Label: "Bloom", Version: 1, Phase: SceneLinear, Alpha: OpaqueOnly, WritesAllPixels: true, PreservesOutsideAux: true, CenterAuxSample: true, AuxiliarySupport: bloomSupport, LumaThreshold: func(p [4][4]float32) float32 { return p[0][0] },
			Fields:  []Field{scalar("Threshold", 1, 0, 64), scalar("Intensity", .22, 0, 8), scalar("Radius", 6, 0, 48)},
			Neutral: func(p [4][4]float32) bool { return p[1][0] == 0 },
			HalfReadOffset: func(step int, p [4][4]float32) (float32, float32) {
				switch step {
				case 1:
					return p[2][0], 0
				case 2:
					return 0, p[2][0]
				default:
					return .5, .5
				}
			},
			HalfPasses: []string{brightPass, blurX, blurY},
			Fragment:   `vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]){return vec4(c.rgb+p[1].x*aego_aux(uv).rgb,c.a);}`},
		{ID: ColorGradeID, Label: "Color Grading", Version: 1, Phase: DisplayLinear, Alpha: PreserveCoverage, LUT: true,
			Fields: []Field{slot(scalar("Contrast", 1, 0, 4), 0, 0), slot(scalar("Saturation", 1, 0, 4), 0, 1), slot(scalar("LUTStrength", 1, 0, 1), 0, 2), slot(rgb("Lift", 0, -1, 1), 1, 0), slot(rgb("Gamma", 1, .01, 8), 2, 0), slot(rgb("Gain", 1, 0, 8), 3, 0)},
			Neutral: func(p [4][4]float32) bool {
				return p[0][0] == 1 && p[0][1] == 1 && p[1] == [4]float32{} && p[2] == [4]float32{1, 1, 1, 0} && p[3] == [4]float32{1, 1, 1, 0}
			}, Fragment: grading},
		{ID: PixelateID, Label: "Pixelate", Version: 1, Phase: DisplayLinear, Alpha: ResampleCoverage,
			Fields: []Field{{Name: "Size", Kind: shader.KindInt, Default: Parameter{Integer: 4}, Min: 1, Max: 256, Bounded: true}}, Neutral: func(p [4][4]float32) bool { return p[0][0] == 1 },
			Fragment: `vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]){vec2 origin=floor(uv*size/p[0].x)*p[0].x; vec2 center=origin+0.5*min(vec2(p[0].x),size-origin);return aego_sample(center/size);}`},
		{ID: ChromaticID, Label: "Chromatic Aberration", Version: 1, Phase: DisplayLinear, Alpha: OpaqueOnly, MotionBypass: true,
			Fields: []Field{scalar("OffsetPixels", 2, 0, 32)}, Neutral: func(p [4][4]float32) bool { return p[0][0] == 0 }, Fragment: `vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]){vec2 d=(uv*2.0-1.0)*p[0].x/size;return vec4(aego_sample(uv+d).r,c.g,aego_sample(uv-d).b,c.a);}`},
		{ID: CRTID, Label: "CRT", Version: 1, Phase: DisplayLinear, Alpha: OpaqueOnly,
			Fields: []Field{scalar("Curvature", .08, 0, .6), scalar("Scanlines", .18, 0, 1)}, Neutral: func(p [4][4]float32) bool { return p[0][0] == 0 && p[1][0] == 0 }, Fragment: `vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]){vec2 q=2.0*uv-1.0;q*=1.0+p[0].x*dot(q,q);vec2 t=.5*(q+1.0);if(any(lessThan(t,vec2(0)))||any(greaterThan(t,vec2(1))))return vec4(0,0,0,1);vec4 v=aego_sample(t);float row=mod(floor(uv.y*size.y),2.0);return vec4(v.rgb*(1.0-row*p[1].x),v.a);}`},
		{ID: CustomID, Label: "Custom Material", Version: 1, Phase: DisplayLinear, Alpha: OpaqueOnly, UsesMaterial: true, Fragment: `vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]){return c;}`},
	}
}

const brightPass = `vec3 aego_bright(vec3 c,float threshold){float y=dot(c,vec3(.2126,.7152,.0722));return c*(max(y-threshold,0.0)/max(y,1e-6));}
vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]){vec2 d=.5/size;vec3 b=aego_bright(aego_sample(uv+vec2(d.x,d.y)).rgb,p[0].x)+aego_bright(aego_sample(uv+vec2(-d.x,d.y)).rgb,p[0].x)+aego_bright(aego_sample(uv+vec2(d.x,-d.y)).rgb,p[0].x)+aego_bright(aego_sample(uv-d).rgb,p[0].x);return vec4(.25*b,1);}`
const blurHead = `vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]){vec2 d=`
const blurTail = `*p[2].x/max(auxInfo.zw,vec2(1)); vec3 a=c.rgb*.375; a+=(aego_sample(uv-d*.5).rgb+aego_sample(uv+d*.5).rgb)*.25;a+=(aego_sample(uv-d).rgb+aego_sample(uv+d).rgb)*.0625;return vec4(a,1);}`
const blurX = blurHead + `vec2(1,0)` + blurTail
const blurY = blurHead + `vec2(0,1)` + blurTail
const grading = `vec3 aego_lut(vec3 v){int n=int(auxInfo.x);vec3 coord=clamp(v,vec3(0),vec3(1))*float(n-1);int k=int(floor(coord.b));vec2 size=vec2(n*n,n);vec2 local=coord.rg+vec2(.5);vec3 a=texture(tex1,(local+vec2(k*n,0))/size).rgb;vec3 b=texture(tex1,(local+vec2(min(n-1,k+1)*n,0))/size).rgb;return mix(a,b,fract(coord.b));}
vec4 aego_effect(vec4 c,vec2 uv,vec2 size,vec4 p[4]){if(c.a<=0.0)return vec4(0);vec3 v=c.rgb/c.a;v=pow(max((v+p[1].rgb)*p[3].rgb,vec3(0)),vec3(1)/p[2].rgb);v=(v-vec3(.5))*p[0].x+.5;v=mix(vec3(dot(v,vec3(.2126,.7152,.0722))),v,p[0].y);v=clamp(v,vec3(0),vec3(1));if(auxInfo.x>1.0&&p[0].z>0.0)v=mix(v,aego_lut(v),p[0].z);return vec4(v*c.a,c.a);}`

func bloomSupport(clear mathx.Color, params [4][4]float32) (float32, bool) {
	peak := max(clear.R, clear.G, clear.B)
	// Leave headroom for both RGBA8 and binary16 rounding during the scene clear.
	dark := clear.A == 1 && min(clear.R, clear.G, clear.B) >= 0 && peak+max(1.0/255, peak/512) < params[0][0]
	// Both blur directions, the extraction taps and bilinear filter footprints.
	return 2*params[2][0] + 8, dark
}
