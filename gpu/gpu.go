package gpu

import (
	"aego/core/handle"
	"aego/core/mathx"
)

const MaxVertexSlots = 4

type (
	bufferTag   struct{}
	textureTag  struct{}
	shaderTag   struct{}
	pipelineTag struct{}
	samplerTag  struct{}
)

type (
	BufferHandle   = handle.Handle[bufferTag]
	TextureHandle  = handle.Handle[textureTag]
	ShaderHandle   = handle.Handle[shaderTag]
	PipelineHandle = handle.Handle[pipelineTag]
	SamplerHandle  = handle.Handle[samplerTag]
)

type BufferUsage uint8

const (
	BufferVertex BufferUsage = iota
	BufferIndex
	BufferUniform
)

type IndexFormat uint8

const (
	Index16 IndexFormat = iota
	Index32
)

func (f IndexFormat) Size() int {
	if f == Index32 {
		return 4
	}
	return 2
}

type BufferDesc struct {
	Usage       BufferUsage
	Size        int
	Dynamic     bool
	IndexFormat IndexFormat
	Data        []byte
	Label       string
}

type TextureFormat uint8

const (
	TexRGBA8 TextureFormat = iota
	TexSRGBA8
	TexR8
	TexRGBA16F
	TexDepth24Stencil8
)

type TextureDimension uint8

const (
	Texture2D TextureDimension = iota
	Texture2DArray
)

type TextureDesc struct {
	Width, Height int
	Dimension     TextureDimension
	// Layers is one (or zero, for compatibility) for Texture2D. Array textures
	// require a positive layer count, including one-layer sampler2DArray resources.
	Layers int
	Format TextureFormat
	Mips   bool
	Data   []byte
	Label  string
}

// TextureSubresource selects one array layer and mip level. Its zero value is
// the original 2D, base-level attachment/update contract.
type TextureSubresource struct{ Layer, Mip int }

type TextureUpdate struct {
	Subresource         TextureSubresource
	X, Y, Width, Height int
	Data                []byte
}

type FilterMode uint8

const (
	FilterNearest FilterMode = iota
	FilterLinear
)

type WrapMode uint8

const (
	WrapClamp WrapMode = iota
	WrapRepeat
)

type SamplerDesc struct {
	MinFilter, MagFilter FilterMode
	WrapU, WrapV         WrapMode
	Mipmaps              bool
}

type ShaderStage uint8

const (
	StageVertex ShaderStage = iota
	StageFragment
)

func (s ShaderStage) String() string {
	if s == StageFragment {
		return "fragment"
	}
	return "vertex"
}

type ShaderSource struct {
	Stage  ShaderStage
	Source string
}

type ShaderDesc struct {
	Sources []ShaderSource
	Label   string
}

type VertexFormat uint8

const (
	VFFloat VertexFormat = iota
	VFFloat2
	VFFloat3
	VFFloat4
	VFUByte4Norm
	VFUInt
	VFUInt4
)

func (f VertexFormat) Components() int {
	switch f {
	case VFFloat2:
		return 2
	case VFFloat3:
		return 3
	case VFFloat4, VFUByte4Norm, VFUInt4:
		return 4
	}
	return 1
}

func (f VertexFormat) Size() int {
	switch f {
	case VFUByte4Norm:
		return 4
	case VFUInt:
		return 4
	}
	return 4 * f.Components()
}

type VertexAttr struct {
	Location uint8
	Slot     uint8
	Format   VertexFormat
	Offset   uint16
}

type VertexLayout struct {
	Attrs       []VertexAttr
	Strides     [MaxVertexSlots]uint16
	PerInstance [MaxVertexSlots]bool
}

type BlendFactor uint8

const (
	BlendZero BlendFactor = iota
	BlendOne
	BlendSrcAlpha
	BlendOneMinusSrcAlpha
	BlendDstAlpha
	BlendOneMinusDstAlpha
	BlendSrcColor
	BlendOneMinusSrcColor
	BlendDstColor
	BlendOneMinusDstColor
)

type BlendState struct {
	Enabled        bool
	SrcRGB, DstRGB BlendFactor
	SrcA, DstA     BlendFactor
}

func BlendAlpha() BlendState {
	return BlendState{
		Enabled: true,
		SrcRGB:  BlendSrcAlpha, DstRGB: BlendOneMinusSrcAlpha,
		SrcA: BlendOne, DstA: BlendOneMinusSrcAlpha,
	}
}

func BlendPremultiplied() BlendState {
	return BlendState{
		Enabled: true,
		SrcRGB:  BlendOne, DstRGB: BlendOneMinusSrcAlpha,
		SrcA: BlendOne, DstA: BlendOneMinusSrcAlpha,
	}
}

// BlendAdditive adds premultiplied RGB without changing destination coverage; normalized targets clamp RGB, while floating targets retain brightness above one.
func BlendAdditive() BlendState {
	return BlendState{
		Enabled: true,
		SrcRGB:  BlendOne, DstRGB: BlendOne,
		SrcA: BlendZero, DstA: BlendOne,
	}
}

// BlendAdditiveLegacy retains the pre-lighting alpha-summing contract for
// callers that explicitly require it. Builtin Additive uses BlendAdditive.
func BlendAdditiveLegacy() BlendState {
	return BlendState{
		Enabled: true,
		SrcRGB:  BlendOne, DstRGB: BlendOne,
		SrcA: BlendOne, DstA: BlendOne,
	}
}

type CompareFunc uint8

const (
	CompareLess CompareFunc = iota
	CompareLessEqual
	CompareGreater
	CompareGreaterEqual
	CompareEqual
	CompareNotEqual
	CompareAlways
	CompareNever
)

type DepthState struct {
	Test, Write bool
	Compare     CompareFunc
}

type CullMode uint8

const (
	CullNone CullMode = iota
	CullBack
	CullFront
)

type Primitive uint8

const (
	PrimTriangles Primitive = iota
	PrimTriangleStrip
	PrimLines
	PrimLineStrip
	PrimPoints
)

func TriangleCount(p Primitive, vertices int) int {
	switch p {
	case PrimTriangles:
		return vertices / 3
	case PrimTriangleStrip:
		if vertices >= 3 {
			return vertices - 2
		}
	}
	return 0
}

type PipelineDesc struct {
	Shader    ShaderHandle
	Layout    VertexLayout
	Blend     BlendState
	Depth     DepthState
	Cull      CullMode
	Primitive Primitive
	Label     string
}

type LoadAction uint8

const (
	LoadKeep LoadAction = iota
	LoadClear
	LoadDontCare
)

type PassDesc struct {
	// ClearRect limits attachment clears in target pixel coordinates.
	// An empty rectangle keeps the low-level full-target clear default.
	ClearRect         mathx.Rect
	Target            TextureHandle
	DepthTarget       TextureHandle
	TargetSubresource TextureSubresource
	DepthSubresource  TextureSubresource
	ClearColor        mathx.Color
	ClearDepth        float32
	Color             LoadAction
	Depth             LoadAction
	FlipWinding       bool
	Label             string
}

type Capabilities struct {
	MaxTextureSize          int
	MaxVertexAttributes     int
	MaxUniformBlockSize     int
	MaxTextureUnits         int
	MaxFragmentTextureUnits int
	MaxArrayTextureLayers   int
	MaxSamples              int
	Instancing              bool
	SRGB                    bool
	TimerQueries            bool
	TextureArrays           bool
	// RGBA16FRenderable is verified by an actual framebuffer/draw/readback probe
	// for this context generation; extension presence alone does not set it.
	RGBA16FRenderable bool
	DebugOutput       bool
	ClipZeroToOne     bool
	OriginBottomLeft  bool
	ESSL              bool
	Vendor            string
	Renderer          string
	Version           string
	GLSL              string
}

type FrameStats struct {
	DrawCalls        uint32
	Vertices         uint32
	Triangles        uint32
	TexSwitches      uint32
	PipelineSwitches uint32
}

type queryTag struct{}

type QueryHandle = handle.Handle[queryTag]
