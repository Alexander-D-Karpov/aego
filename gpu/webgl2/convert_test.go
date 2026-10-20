package webgl2

import (
	"testing"

	"aego/gpu"
	"aego/gpu/internal/glstate"
)

func TestTextureFormats(t *testing.T) {
	for _, f := range []gpu.TextureFormat{gpu.TexRGBA8, gpu.TexSRGBA8, gpu.TexR8, gpu.TexRGBA16F, gpu.TexDepth24Stencil8} {
		info, ok := textureInfo(f)
		if !ok || info.bpp == 0 || info.internal == 0 || info.format == 0 {
			t.Fatalf("format %d: %+v", f, info)
		}
	}
	d, _ := textureInfo(gpu.TexDepth24Stencil8)
	if !d.depth || d.attach != glDepthStencilAttachment || d.xtype != glUnsignedInt248 {
		t.Fatalf("depth = %+v", d)
	}
	if _, ok := textureInfo(gpu.TextureFormat(200)); ok {
		t.Fatal("unknown format accepted")
	}
}

func TestBlend(t *testing.T) {
	b := toBlend(gpu.BlendPremultiplied())
	if !b.Enabled || b.SrcRGB != glOne || b.DstRGB != glOneMinusSrcAlpha {
		t.Fatalf("premultiplied = %+v", b)
	}
	if toBlend(gpu.BlendState{SrcRGB: gpu.BlendOne}) != (glstate.Blend{}) {
		t.Fatal("disabled blend keeps factors")
	}
}

func TestSlots(t *testing.T) {
	if n, ok := slotIndex("ub1", "ub"); !ok || n != 1 {
		t.Fatal("ub1")
	}
	if _, ok := slotIndex("u_color", "ub"); ok {
		t.Fatal("u_color is not a block slot")
	}
	if !isSamplerType(glSampler2D) || isSamplerType(glFloat) {
		t.Fatal("sampler types")
	}
}

func TestSamplerValues(t *testing.T) {
	if filterValue(gpu.FilterLinear) != glLinear || filterValue(gpu.FilterNearest) != glNearest {
		t.Fatal("filters")
	}
	if minFilterValue(gpu.FilterLinear, true) != glLinearMipmapLinear || minFilterValue(gpu.FilterNearest, true) != glNearestMipmapNearest || minFilterValue(gpu.FilterLinear, false) != glLinear {
		t.Fatal("min filters")
	}
	if wrapValue(gpu.WrapRepeat) != glRepeat || wrapValue(gpu.WrapClamp) != glClampToEdge {
		t.Fatal("wrap")
	}
}

func TestPipelineState(t *testing.T) {
	if d := toDepth(gpu.DepthState{Test: true, Write: true, Compare: gpu.CompareLessEqual}); !d.Test || !d.Write || d.Func != glLequal {
		t.Fatalf("depth = %+v", d)
	}
	if compareFunc(gpu.CompareNever) != glNever || compareFunc(gpu.CompareAlways) != glAlways {
		t.Fatal("compare")
	}
	if c := toCull(gpu.CullBack); !c.Enabled || c.Face != glBack {
		t.Fatalf("cull back = %+v", c)
	}
	if toCull(gpu.CullFront).Face != glFront || toCull(gpu.CullNone).Enabled {
		t.Fatal("cull")
	}
	if primitiveMode(gpu.PrimTriangles) != glTriangles || primitiveMode(gpu.PrimLineStrip) != glLineStrip || primitiveMode(gpu.PrimPoints) != glPoints {
		t.Fatal("primitives")
	}
	if stageType(gpu.StageFragment) != glFragmentShader || stageType(gpu.StageVertex) != glVertexShader {
		t.Fatal("stages")
	}
}

func TestVertexFormats(t *testing.T) {
	if n, ty, norm, in := vertexFormat(gpu.VFUByte4Norm); n != 4 || ty != glUnsignedByte || !norm || in {
		t.Fatal("ubyte4 norm")
	}
	if n, ty, _, in := vertexFormat(gpu.VFUInt); n != 1 || ty != glUnsignedInt || !in {
		t.Fatal("uint")
	}
	if n, ty, _, _ := vertexFormat(gpu.VFFloat3); n != 3 || ty != glFloat {
		t.Fatal("float3")
	}
}

func TestDiagnosticNames(t *testing.T) {
	if glErrorName(glInvalidOperation) != "INVALID_OPERATION" || glErrorName(glContextLost) != "CONTEXT_LOST_WEBGL" || glErrorName(0x1234) != "0x1234" {
		t.Fatal("gl error names")
	}
	if fbStatusName(glFramebufferIncompleteAttachment) != "INCOMPLETE_ATTACHMENT" || fbStatusName(1) != "UNKNOWN" {
		t.Fatal("framebuffer status names")
	}
}
