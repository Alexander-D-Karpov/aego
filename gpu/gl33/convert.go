package gl33

import (
	"strconv"

	"aego/gpu/gl33/gl"

	"aego/gpu"
	"aego/gpu/internal/glstate"
)

type texInfo struct {
	internal int32
	format   uint32
	xtype    uint32
	bpp      int
	depth    bool
	attach   uint32
	name     string
}

var depthFormats = []texInfo{
	{internal: gl.DEPTH24_STENCIL8, format: gl.DEPTH_STENCIL, xtype: gl.UNSIGNED_INT_24_8, bpp: 4, depth: true, attach: gl.DEPTH_STENCIL_ATTACHMENT, name: "DEPTH24_STENCIL8"},
	{internal: gl.DEPTH_COMPONENT24, format: gl.DEPTH_COMPONENT, xtype: gl.UNSIGNED_INT, bpp: 4, depth: true, attach: gl.DEPTH_ATTACHMENT, name: "DEPTH_COMPONENT24"},
	{internal: gl.DEPTH_COMPONENT32F, format: gl.DEPTH_COMPONENT, xtype: gl.FLOAT, bpp: 4, depth: true, attach: gl.DEPTH_ATTACHMENT, name: "DEPTH_COMPONENT32F"},
}

func textureInfo(f gpu.TextureFormat) (texInfo, bool) {
	switch f {
	case gpu.TexRGBA8:
		return texInfo{internal: gl.RGBA8, format: gl.RGBA, xtype: gl.UNSIGNED_BYTE, bpp: 4, name: "RGBA8"}, true
	case gpu.TexSRGBA8:
		return texInfo{internal: gl.SRGB8_ALPHA8, format: gl.RGBA, xtype: gl.UNSIGNED_BYTE, bpp: 4, name: "SRGB8_ALPHA8"}, true
	case gpu.TexR8:
		return texInfo{internal: gl.R8, format: gl.RED, xtype: gl.UNSIGNED_BYTE, bpp: 1, name: "R8"}, true
	case gpu.TexRGBA16F:
		return texInfo{internal: gl.RGBA16F, format: gl.RGBA, xtype: gl.FLOAT, bpp: 16, name: "RGBA16F"}, true
	case gpu.TexDepth24Stencil8:
		return depthFormats[0], true
	}
	return texInfo{}, false
}

func filterValue(f gpu.FilterMode) int32 {
	if f == gpu.FilterLinear {
		return gl.LINEAR
	}
	return gl.NEAREST
}

func minFilterValue(f gpu.FilterMode, mipmaps bool) int32 {
	switch {
	case mipmaps && f == gpu.FilterLinear:
		return gl.LINEAR_MIPMAP_LINEAR
	case mipmaps:
		return gl.NEAREST_MIPMAP_NEAREST
	}
	return filterValue(f)
}

func wrapValue(w gpu.WrapMode) int32 {
	if w == gpu.WrapRepeat {
		return gl.REPEAT
	}
	return gl.CLAMP_TO_EDGE
}

func blendFactor(f gpu.BlendFactor) uint32 {
	switch f {
	case gpu.BlendZero:
		return gl.ZERO
	case gpu.BlendOne:
		return gl.ONE
	case gpu.BlendSrcAlpha:
		return gl.SRC_ALPHA
	case gpu.BlendOneMinusSrcAlpha:
		return gl.ONE_MINUS_SRC_ALPHA
	case gpu.BlendDstAlpha:
		return gl.DST_ALPHA
	case gpu.BlendOneMinusDstAlpha:
		return gl.ONE_MINUS_DST_ALPHA
	case gpu.BlendSrcColor:
		return gl.SRC_COLOR
	case gpu.BlendOneMinusSrcColor:
		return gl.ONE_MINUS_SRC_COLOR
	case gpu.BlendDstColor:
		return gl.DST_COLOR
	case gpu.BlendOneMinusDstColor:
		return gl.ONE_MINUS_DST_COLOR
	}
	return gl.ONE
}

func toBlend(b gpu.BlendState) glstate.Blend {
	if !b.Enabled {
		return glstate.Blend{}
	}
	return glstate.Blend{
		Enabled: true,
		SrcRGB:  blendFactor(b.SrcRGB),
		DstRGB:  blendFactor(b.DstRGB),
		SrcA:    blendFactor(b.SrcA),
		DstA:    blendFactor(b.DstA),
	}
}

func compareFunc(c gpu.CompareFunc) uint32 {
	switch c {
	case gpu.CompareLess:
		return gl.LESS
	case gpu.CompareLessEqual:
		return gl.LEQUAL
	case gpu.CompareGreater:
		return gl.GREATER
	case gpu.CompareGreaterEqual:
		return gl.GEQUAL
	case gpu.CompareEqual:
		return gl.EQUAL
	case gpu.CompareNotEqual:
		return gl.NOTEQUAL
	case gpu.CompareAlways:
		return gl.ALWAYS
	case gpu.CompareNever:
		return gl.NEVER
	}
	return gl.LESS
}

func toDepth(d gpu.DepthState) glstate.Depth {
	return glstate.Depth{Test: d.Test, Write: d.Write, Func: compareFunc(d.Compare)}
}

func toCull(c gpu.CullMode) glstate.Cull {
	switch c {
	case gpu.CullBack:
		return glstate.Cull{Enabled: true, Face: gl.BACK}
	case gpu.CullFront:
		return glstate.Cull{Enabled: true, Face: gl.FRONT}
	}
	return glstate.Cull{}
}

func primitiveMode(p gpu.Primitive) uint32 {
	switch p {
	case gpu.PrimTriangleStrip:
		return gl.TRIANGLE_STRIP
	case gpu.PrimLines:
		return gl.LINES
	case gpu.PrimLineStrip:
		return gl.LINE_STRIP
	case gpu.PrimPoints:
		return gl.POINTS
	}
	return gl.TRIANGLES
}

func vertexFormat(f gpu.VertexFormat) (size int32, xtype uint32, normalized, integer bool) {
	switch f {
	case gpu.VFFloat:
		return 1, gl.FLOAT, false, false
	case gpu.VFFloat2:
		return 2, gl.FLOAT, false, false
	case gpu.VFFloat3:
		return 3, gl.FLOAT, false, false
	case gpu.VFFloat4:
		return 4, gl.FLOAT, false, false
	case gpu.VFUByte4Norm:
		return 4, gl.UNSIGNED_BYTE, true, false
	case gpu.VFUInt4:
		return 4, gl.UNSIGNED_INT, false, true
	case gpu.VFUInt:
		return 1, gl.UNSIGNED_INT, false, true
	}
	return 1, gl.FLOAT, false, false
}

func stageType(s gpu.ShaderStage) uint32 {
	if s == gpu.StageFragment {
		return gl.FRAGMENT_SHADER
	}
	return gl.VERTEX_SHADER
}

func isSamplerType(t uint32) bool {
	switch t {
	case gl.SAMPLER_2D, gl.SAMPLER_2D_ARRAY, gl.SAMPLER_2D_SHADOW, gl.SAMPLER_3D,
		gl.SAMPLER_CUBE, gl.INT_SAMPLER_2D, gl.UNSIGNED_INT_SAMPLER_2D, gl.SAMPLER_2D_RECT:
		return true
	}
	return false
}

func glErrorName(code uint32) string {
	switch code {
	case gl.INVALID_ENUM:
		return "INVALID_ENUM"
	case gl.INVALID_VALUE:
		return "INVALID_VALUE"
	case gl.INVALID_OPERATION:
		return "INVALID_OPERATION"
	case gl.INVALID_FRAMEBUFFER_OPERATION:
		return "INVALID_FRAMEBUFFER_OPERATION"
	case gl.OUT_OF_MEMORY:
		return "OUT_OF_MEMORY"
	}
	return "0x" + strconv.FormatUint(uint64(code), 16)
}

func debugSourceName(s uint32) string {
	switch s {
	case gl.DEBUG_SOURCE_API:
		return "api"
	case gl.DEBUG_SOURCE_WINDOW_SYSTEM:
		return "window"
	case gl.DEBUG_SOURCE_SHADER_COMPILER:
		return "shader"
	case gl.DEBUG_SOURCE_THIRD_PARTY:
		return "third_party"
	case gl.DEBUG_SOURCE_APPLICATION:
		return "application"
	}
	return "other"
}

func debugTypeName(t uint32) string {
	switch t {
	case gl.DEBUG_TYPE_ERROR:
		return "error"
	case gl.DEBUG_TYPE_DEPRECATED_BEHAVIOR:
		return "deprecated"
	case gl.DEBUG_TYPE_UNDEFINED_BEHAVIOR:
		return "undefined"
	case gl.DEBUG_TYPE_PORTABILITY:
		return "portability"
	case gl.DEBUG_TYPE_PERFORMANCE:
		return "performance"
	case gl.DEBUG_TYPE_MARKER:
		return "marker"
	}
	return "other"
}
