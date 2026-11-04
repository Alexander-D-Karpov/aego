package shader

import (
	"encoding/binary"
	"math"
)

const (
	ABI = 1

	SlotFrame    = 0
	SlotView     = 1
	SlotMaterial = 2
	SlotDraw     = 3
	SlotLighting = 4
	SlotSurface  = 5

	TexMain     = 0
	TexNormal   = 1
	TexUser     = 2
	TexShadow   = 8
	TexCookie   = 9
	TexEmission = 10

	LaneSlot              = 1
	LaneLocation          = 4
	LaneStride            = 64
	TrimSlot              = 2
	TrimLocation          = 8
	TrimStride            = 16
	AuxSlot               = 3
	AuxStride             = 80
	AffineLocation        = 9
	NormalUVLocation      = 10
	NormalLocalLocation   = 11
	EmissionUVLocation    = 12
	EmissionLocalLocation = 13

	MaxUserTextures   = 6
	MaxParams         = 32
	MaxNameLen        = 32
	MaxKeywords       = 8
	MaxVariants       = 32
	MaxLanes          = 4
	MaxIncludeDepth   = 8
	MaxIncludeFiles   = 128
	MaxExpandedBytes  = 1 << 20
	MaxInstanceParams = 8
	MaxArtifactBytes  = 64 << 20

	FrameSize = 16
	ViewSize  = 112
)

type Domain uint8

const (
	DomainSprite Domain = iota
	DomainMesh2D
	DomainPostFX
)

var domainNames = [...]string{"sprite", "mesh2d", "postfx"}

func (d Domain) String() string {
	if int(d) < len(domainNames) {
		return domainNames[d]
	}
	return "unknown"
}

func ParseDomain(s string) (Domain, bool) {
	for i, n := range domainNames {
		if n == s {
			return Domain(i), true
		}
	}
	return 0, false
}

type Blend uint8

const (
	BlendPremultiplied Blend = iota
	BlendOpaque
	BlendAdditive
)

var blendNames = [...]string{"premultiplied", "opaque", "additive"}

func (b Blend) String() string {
	if int(b) < len(blendNames) {
		return blendNames[b]
	}
	return "unknown"
}

func ParseBlend(s string) (Blend, bool) {
	for i, n := range blendNames {
		if n == s {
			return Blend(i), true
		}
	}
	return 0, false
}

type Kind uint8

const (
	KindFloat Kind = iota
	KindInt
	KindBool
	KindVec2
	KindVec3
	KindVec4
	KindTexture
)

var kindNames = [...]string{"float", "int", "bool", "vec2", "vec3", "vec4", "sampler2D"}

func (k Kind) GLSL() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "float"
}

func ParseKind(s string) (Kind, bool) {
	for i, n := range kindNames {
		if n == s {
			return Kind(i), true
		}
	}
	return 0, false
}

func (k Kind) Lanes() int {
	switch k {
	case KindVec2:
		return 2
	case KindVec3:
		return 3
	case KindVec4:
		return 4
	case KindTexture:
		return 0
	}
	return 1
}

func (k Kind) align() int {
	switch k {
	case KindVec2:
		return 8
	case KindVec3, KindVec4:
		return 16
	}
	return 4
}

func Std140(kinds []Kind, offsets []int) int {
	off := 0
	for i, k := range kinds {
		a := k.align()
		off = (off + a - 1) &^ (a - 1)
		offsets[i] = off
		off += 4 * k.Lanes()
	}
	return (off + 15) &^ 15
}

type Param struct {
	Name    string
	Kind    Kind
	Default [4]float32
	// IntDefault is the exact signed 32-bit default; Default is for floating-point values.
	IntDefault int32
	Former     []string
	Min        float32
	Max        float32
	HasRange   bool
	Color      bool
	Hidden     bool
	Instance   bool
	// Normal reserves the engine normal binding while preserving all six user samplers.
	Normal bool
	// Emission reserves the surface emission binding without reducing user textures.
	Emission bool
	Doc      string
	Display  string
	Unit     string
	Enum     []string
	Offset   int
	Lane     int
	Comp     int
	Slot     int
	File     int
	Line     int
}

type Loc struct {
	File int32
	Line int32
}

const FrameBlock = `layout(std140) uniform ub0 {
    vec4 aego_frame_data;
};
#define aego_time aego_frame_data.x
#define aego_delta aego_frame_data.y
#define aego_unscaled aego_frame_data.z
#define aego_frame_index aego_frame_data.w`

const ViewBlock = `layout(std140) uniform ub1 {
    mat4 aego_view_proj;
    vec4 aego_viewport;
    vec4 aego_view_params;
    vec4 aego_camera;
};
#define aego_pixel_scale aego_view_params.x
#define aego_pixels_per_unit aego_view_params.y
#define aego_zoom aego_view_params.z
#define aego_clip_to_pixel_y aego_view_params.w
#define aego_camera_pos aego_camera.xyz`

func put(b []byte, v float32) {
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
}

func PutFrame(dst []byte, time, delta, unscaled float32, frame uint64) {
	put(dst[0:], time)
	put(dst[4:], delta)
	put(dst[8:], unscaled)
	put(dst[12:], float32(frame))
}

func PutView(dst []byte, viewProj *[16]float32, viewport [4]float32, pixelScale, pixelsPerUnit, zoom float32, camera [3]float32) {
	for i, v := range viewProj {
		put(dst[4*i:], v)
	}
	for i, v := range viewport {
		put(dst[64+4*i:], v)
	}
	put(dst[80:], pixelScale)
	put(dst[84:], pixelsPerUnit)
	put(dst[88:], zoom)
	// Direct callers retain the offscreen (+Y) convention. The renderer writes
	// -1 for a normal top-left window viewport, +1 for its flipped offscreen view.
	put(dst[92:], 1)
	for i, v := range camera {
		put(dst[96+4*i:], v)
	}
	put(dst[108:], 0)
}
