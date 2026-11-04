package gpu_test

import (
	"testing"

	"aego/gpu"
	"aego/gpu/null"
)

func TestTextureStorageAndSubresources(t *testing.T) {
	d := gpu.TextureDesc{Width: 8, Height: 4, Dimension: gpu.Texture2DArray, Layers: 3, Mips: true, Format: gpu.TexRGBA16F}
	if got := d.StorageBytes(); got != (32+8+2+1)*3*8 {
		t.Fatalf("storage bytes: %d", got)
	}
	if d.Format.UploadBytesPerPixel() != 16 || d.Format.StorageBytesPerPixel() != 8 {
		t.Fatal("half-float storage and float32 transfer sizes must differ")
	}
	if err := gpu.ValidateTextureDesc(d, null.New().Caps()); err != nil {
		t.Fatal(err)
	}
	for mip, want := range [][2]int{{8, 4}, {4, 2}, {2, 1}, {1, 1}} {
		w, h, ok := d.SubresourceSize(gpu.TextureSubresource{Layer: 2, Mip: mip})
		if !ok || w != want[0] || h != want[1] {
			t.Fatalf("mip %d: %d x %d, %v", mip, w, h, ok)
		}
	}
	for _, sub := range []gpu.TextureSubresource{{Layer: -1}, {Layer: 3}, {Mip: -1}, {Mip: 4}} {
		if _, _, ok := d.SubresourceSize(sub); ok {
			t.Fatalf("accepted invalid subresource %+v", sub)
		}
	}
	legacy := gpu.TextureDesc{Width: 3, Height: 2, Format: gpu.TexRGBA8}
	if legacy.LayerCount() != 1 || legacy.MipLevels() != 1 || legacy.StorageBytes() != 24 {
		t.Fatal("legacy 2D defaults changed")
	}
	depth := gpu.TextureDesc{Width: 8, Height: 8, Mips: true, Format: gpu.TexDepth24Stencil8}
	if depth.MipLevels() != 1 || depth.StorageBytes() != 256 {
		t.Fatal("depth mip chain unexpectedly enabled")
	}
}

func TestTextureValidationRejectsInvalidBudgets(t *testing.T) {
	caps := null.New().Caps()
	base := gpu.TextureDesc{Width: 4, Height: 4, Format: gpu.TexRGBA8}
	for _, change := range []func(*gpu.TextureDesc){
		func(d *gpu.TextureDesc) { d.Width = 0 },
		func(d *gpu.TextureDesc) { d.Height = caps.MaxTextureSize + 1 },
		func(d *gpu.TextureDesc) { d.Dimension = 99 },
		func(d *gpu.TextureDesc) { d.Layers = 2 },
		func(d *gpu.TextureDesc) { d.Dimension = gpu.Texture2DArray; d.Layers = 0 },
		func(d *gpu.TextureDesc) { d.Dimension = gpu.Texture2DArray; d.Layers = caps.MaxArrayTextureLayers + 1 },
		func(d *gpu.TextureDesc) { d.Format = 99 },
		func(d *gpu.TextureDesc) { d.Data = make([]byte, 63) },
		func(d *gpu.TextureDesc) { d.Format = gpu.TexRGBA16F; d.Data = make([]byte, 4*4*8) },
	} {
		d := base
		change(&d)
		if gpu.ValidateTextureDesc(d, caps) == nil {
			t.Fatalf("accepted invalid descriptor %+v", d)
		}
	}
	d := base
	d.Dimension = gpu.Texture2DArray
	d.Layers = 1
	caps.TextureArrays = false
	if gpu.ValidateTextureDesc(d, caps) == nil {
		t.Fatal("accepted array on incapable device")
	}
	caps.TextureArrays = true
	caps.MaxTextureSize = int(^uint(0) >> 1)
	d.Width = caps.MaxTextureSize
	d.Height = 4
	if gpu.ValidateTextureDesc(d, caps) == nil {
		t.Fatal("addressable-size overflow accepted")
	}
}

func TestTextureRegionValidationAndNullParity(t *testing.T) {
	d := null.New()
	defer d.Close()
	desc := gpu.TextureDesc{Width: 4, Height: 4, Format: gpu.TexRGBA8, Dimension: gpu.Texture2DArray, Layers: 2, Mips: true}
	h, err := d.CreateTexture(desc)
	if err != nil {
		t.Fatal(err)
	}
	u := gpu.TextureUpdate{Subresource: gpu.TextureSubresource{Layer: 1, Mip: 1}, X: 1, Y: 1, Width: 1, Height: 1, Data: []byte{1, 2, 3, 4}}
	if err := d.UpdateTextureRegion(h, u); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*gpu.TextureUpdate){
		func(u *gpu.TextureUpdate) { u.Subresource.Layer = 2 },
		func(u *gpu.TextureUpdate) { u.Subresource.Mip = 3 },
		func(u *gpu.TextureUpdate) { u.X = 2 },
		func(u *gpu.TextureUpdate) { u.Y = -1 },
		func(u *gpu.TextureUpdate) { u.Width = 0 },
		func(u *gpu.TextureUpdate) { u.Width = int(^uint(0) >> 1) },
		func(u *gpu.TextureUpdate) { u.Data = u.Data[:3] },
	} {
		bad := u
		change(&bad)
		if d.UpdateTextureRegion(h, bad) == nil {
			t.Fatalf("invalid region accepted %+v", bad)
		}
	}
	if err := d.ValidateRenderTarget(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Layer: 1, Mip: 2}}); err != nil {
		t.Fatal(err)
	}
	if d.ValidateRenderTarget(gpu.PassDesc{Target: h, TargetSubresource: gpu.TextureSubresource{Layer: 2}}) == nil {
		t.Fatal("out-of-range framebuffer layer accepted")
	}
	if d.ReadTexturePixels(h, gpu.TextureSubresource{Layer: 1, Mip: 1}, 0, 0, 2, 2, make([]byte, 16)) != nil {
		t.Fatal("valid mip readback rejected")
	}
	if d.ReadTexturePixels(h, gpu.TextureSubresource{Layer: 1, Mip: 1}, 0, 0, 4, 4, make([]byte, 64)) == nil {
		t.Fatal("readback ignores mip dimensions")
	}
	d.DestroyTexture(h)
	if d.UpdateTextureRegion(h, u) == nil {
		t.Fatal("stale handle accepted")
	}
}

func TestFloatTextureReadbackRequiresFloatDestination(t *testing.T) {
	d := null.New()
	defer d.Close()
	h, err := d.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA16F})
	if err != nil {
		t.Fatal(err)
	}
	if d.ReadPixels(h, 0, 0, 1, 1, make([]byte, 4)) == nil {
		t.Fatal("float attachment accepted for byte readback")
	}
	if err := d.ReadTexturePixelsFloat(h, gpu.TextureSubresource{}, 0, 0, 1, 1, make([]float32, 4)); err != nil {
		t.Fatal(err)
	}
}

func TestTextureInfoRejectsStaleHandleAndOmitsData(t *testing.T) {
	d := null.New()
	defer d.Close()
	desc := gpu.TextureDesc{Width: 2, Height: 2, Format: gpu.TexSRGBA8, Data: make([]byte, 16)}
	h, err := d.CreateTexture(desc)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := d.TextureInfo(h)
	if !ok || got.Width != 2 || got.Height != 2 || got.Format != gpu.TexSRGBA8 || got.Data != nil {
		t.Fatalf("description %+v %v", got, ok)
	}
	d.DestroyTexture(h)
	if _, ok := d.TextureInfo(h); ok {
		t.Fatal("stale texture descriptor exposed")
	}
}
