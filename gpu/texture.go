package gpu

import (
	"errors"
	"math/bits"
)

func (f TextureFormat) StorageBytesPerPixel() int {
	switch f {
	case TexR8:
		return 1
	case TexRGBA8, TexSRGBA8, TexDepth24Stencil8:
		return 4
	case TexRGBA16F:
		return 8
	}
	return 0
}

// UploadBytesPerPixel describes the client-memory representation. RGBA16F uses
// four float32 values on upload/readback but stores four half-floats on the GPU.
func (f TextureFormat) UploadBytesPerPixel() int {
	if f == TexRGBA16F {
		return 16
	}
	return f.StorageBytesPerPixel()
}

func (d TextureDesc) LayerCount() int {
	if d.Dimension == Texture2D {
		return 1
	}
	return d.Layers
}

func (d TextureDesc) MipLevels() int {
	if d.Width <= 0 || d.Height <= 0 {
		return 0
	}
	if !d.Mips || d.Format == TexDepth24Stencil8 {
		return 1
	}
	return bits.Len(uint(max(d.Width, d.Height)))
}

func (d TextureDesc) SubresourceSize(s TextureSubresource) (int, int, bool) {
	if s.Layer < 0 || s.Layer >= d.LayerCount() || s.Mip < 0 || s.Mip >= d.MipLevels() {
		return 0, 0, false
	}
	return max(1, d.Width>>s.Mip), max(1, d.Height>>s.Mip), true
}

func (d TextureDesc) StorageBytes() uint64 {
	var n uint64
	for mip := 0; mip < d.MipLevels(); mip++ {
		w, h, ok := d.SubresourceSize(TextureSubresource{Mip: mip})
		if !ok {
			return 0
		}
		n += uint64(w) * uint64(h) * uint64(d.LayerCount()) * uint64(d.Format.StorageBytesPerPixel())
	}
	return n
}

func ValidateTextureDesc(d TextureDesc, caps Capabilities) error {
	if d.Width <= 0 || d.Height <= 0 || d.Width > caps.MaxTextureSize || d.Height > caps.MaxTextureSize {
		return errors.New("invalid texture dimensions")
	}
	switch d.Dimension {
	case Texture2D:
		if d.Layers < 0 || d.Layers > 1 {
			return errors.New("2D texture requires zero or one layer")
		}
	case Texture2DArray:
		if !caps.TextureArrays || d.Layers <= 0 || d.Layers > caps.MaxArrayTextureLayers {
			return errors.New("texture array exceeds device layer capabilities")
		}
	default:
		return errors.New("unknown texture dimension")
	}
	if d.Format.StorageBytesPerPixel() == 0 {
		return errors.New("unsupported texture format")
	}
	// Bound integer products before they are used in slice sizes or native calls.
	w, h, layers, bpp := uint64(d.Width), uint64(d.Height), uint64(d.LayerCount()), uint64(d.Format.UploadBytesPerPixel())
	maxInt := uint64(^uint(0) >> 1)
	if w > maxInt/h || w*h > maxInt/layers || w*h*layers > maxInt/bpp {
		return errors.New("texture data size overflows addressable memory")
	}
	if d.Data != nil && uint64(len(d.Data)) < w*h*layers*bpp {
		return errors.New("texture data is too small")
	}
	return nil
}

func ValidateTextureUpdate(d TextureDesc, u TextureUpdate) error {
	w, h, ok := d.SubresourceSize(u.Subresource)
	if !ok {
		return errors.New("texture update subresource is out of range")
	}
	if d.Format == TexDepth24Stencil8 {
		return errors.New("depth texture updates are unsupported")
	}
	if u.X < 0 || u.Y < 0 || u.Width <= 0 || u.Height <= 0 || u.Width > w || u.Height > h || u.X > w-u.Width || u.Y > h-u.Height {
		return errors.New("texture update region is out of range")
	}
	need := uint64(u.Width) * uint64(u.Height) * uint64(d.Format.UploadBytesPerPixel())
	if need > uint64(len(u.Data)) {
		return errors.New("texture update data is too small")
	}
	return nil
}
