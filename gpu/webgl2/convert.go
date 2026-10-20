package webgl2

import (
	"strconv"
	"strings"

	"aego/gpu"
	"aego/gpu/internal/glstate"
)

type texInfo struct {
	internal uint32
	format   uint32
	xtype    uint32
	bpp      int
	depth    bool
	attach   uint32
	name     string
}

func textureInfo(f gpu.TextureFormat) (texInfo, bool) {
	switch f {
	case gpu.TexRGBA8:
		return texInfo{internal: glRGBA8, format: glRGBA, xtype: glUnsignedByte, bpp: 4, name: "RGBA8"}, true
	case gpu.TexSRGBA8:
		return texInfo{internal: glSRGB8Alpha8, format: glRGBA, xtype: glUnsignedByte, bpp: 4, name: "SRGB8_ALPHA8"}, true
	case gpu.TexR8:
		return texInfo{internal: glR8, format: glRed, xtype: glUnsignedByte, bpp: 1, name: "R8"}, true
	case gpu.TexRGBA16F:
		return texInfo{internal: glRGBA16F, format: glRGBA, xtype: glFloat, bpp: 16, name: "RGBA16F"}, true
	case gpu.TexDepth24Stencil8:
		return texInfo{internal: glDepth24Stencil8, format: glDepthStencil, xtype: glUnsignedInt248, bpp: 4, depth: true, attach: glDepthStencilAttachment, name: "DEPTH24_STENCIL8"}, true
	}
	return texInfo{}, false
}

func filterValue(f gpu.FilterMode) int {
	if f == gpu.FilterLinear {
		return glLinear
	}
	return glNearest
}

func minFilterValue(f gpu.FilterMode, mipmaps bool) int {
	switch {
	case mipmaps && f == gpu.FilterLinear:
		return glLinearMipmapLinear
	case mipmaps:
		return glNearestMipmapNearest
	}
	return filterValue(f)
}

func wrapValue(w gpu.WrapMode) int {
	if w == gpu.WrapRepeat {
		return glRepeat
	}
	return glClampToEdge
}

func blendFactor(f gpu.BlendFactor) uint32 {
	switch f {
	case gpu.BlendZero:
		return glZero
	case gpu.BlendOne:
		return glOne
	case gpu.BlendSrcAlpha:
		return glSrcAlpha
	case gpu.BlendOneMinusSrcAlpha:
		return glOneMinusSrcAlpha
	case gpu.BlendDstAlpha:
		return glDstAlpha
	case gpu.BlendOneMinusDstAlpha:
		return glOneMinusDstAlpha
	case gpu.BlendSrcColor:
		return glSrcColor
	case gpu.BlendOneMinusSrcColor:
		return glOneMinusSrcColor
	case gpu.BlendDstColor:
		return glDstColor
	case gpu.BlendOneMinusDstColor:
		return glOneMinusDstColor
	}
	return glOne
}

func toBlend(b gpu.BlendState) glstate.Blend {
	if !b.Enabled {
		return glstate.Blend{}
	}
	return glstate.Blend{Enabled: true, SrcRGB: blendFactor(b.SrcRGB), DstRGB: blendFactor(b.DstRGB), SrcA: blendFactor(b.SrcA), DstA: blendFactor(b.DstA)}
}

func compareFunc(c gpu.CompareFunc) uint32 {
	switch c {
	case gpu.CompareLessEqual:
		return glLequal
	case gpu.CompareGreater:
		return glGreater
	case gpu.CompareGreaterEqual:
		return glGequal
	case gpu.CompareEqual:
		return glEqual
	case gpu.CompareNotEqual:
		return glNotequal
	case gpu.CompareAlways:
		return glAlways
	case gpu.CompareNever:
		return glNever
	}
	return glLess
}

func toDepth(d gpu.DepthState) glstate.Depth {
	return glstate.Depth{Test: d.Test, Write: d.Write, Func: compareFunc(d.Compare)}
}

func toCull(c gpu.CullMode) glstate.Cull {
	switch c {
	case gpu.CullBack:
		return glstate.Cull{Enabled: true, Face: glBack}
	case gpu.CullFront:
		return glstate.Cull{Enabled: true, Face: glFront}
	}
	return glstate.Cull{}
}

func primitiveMode(p gpu.Primitive) uint32 {
	switch p {
	case gpu.PrimTriangleStrip:
		return glTriangleStrip
	case gpu.PrimLines:
		return glLines
	case gpu.PrimLineStrip:
		return glLineStrip
	case gpu.PrimPoints:
		return glPoints
	}
	return glTriangles
}

func vertexFormat(f gpu.VertexFormat) (size int32, xtype uint32, normalized, integer bool) {
	switch f {
	case gpu.VFFloat2:
		return 2, glFloat, false, false
	case gpu.VFFloat3:
		return 3, glFloat, false, false
	case gpu.VFFloat4:
		return 4, glFloat, false, false
	case gpu.VFUByte4Norm:
		return 4, glUnsignedByte, true, false
	case gpu.VFUInt:
		return 1, glUnsignedInt, false, true
	}
	return 1, glFloat, false, false
}

func stageType(s gpu.ShaderStage) uint32 {
	if s == gpu.StageFragment {
		return glFragmentShader
	}
	return glVertexShader
}

func isSamplerType(t uint32) bool {
	switch t {
	case glSampler2D, glSampler2DArray, glSampler2DShadow, glSampler3D, glSamplerCube, glIntSampler2D, glUnsignedIntSampler2D:
		return true
	}
	return false
}

func glErrorName(code uint32) string {
	switch code {
	case glInvalidEnum:
		return "INVALID_ENUM"
	case glInvalidValue:
		return "INVALID_VALUE"
	case glInvalidOperation:
		return "INVALID_OPERATION"
	case glInvalidFramebufferOperation:
		return "INVALID_FRAMEBUFFER_OPERATION"
	case glOutOfMemory:
		return "OUT_OF_MEMORY"
	case glContextLost:
		return "CONTEXT_LOST_WEBGL"
	}
	return "0x" + strconv.FormatUint(uint64(code), 16)
}

func fbStatusName(s uint32) string {
	switch s {
	case glFramebufferIncompleteAttachment:
		return "INCOMPLETE_ATTACHMENT"
	case glFramebufferIncompleteMissingAttachment:
		return "INCOMPLETE_MISSING_ATTACHMENT"
	case glFramebufferIncompleteDimensions:
		return "INCOMPLETE_DIMENSIONS"
	case glFramebufferUnsupported:
		return "UNSUPPORTED"
	case glFramebufferIncompleteMultisample:
		return "INCOMPLETE_MULTISAMPLE"
	}
	return "UNKNOWN"
}

func slotIndex(name, prefix string) (int, bool) {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok || rest == "" {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && n >= 0
}
