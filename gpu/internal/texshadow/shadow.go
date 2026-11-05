// Package texshadow keeps texture upload data needed after a context restore.
// Rendered pixels are deliberately not mirrored; their owner redraws them.
package texshadow

import "aego/gpu"

type level struct {
	data   []byte
	dirty  []uint64
	active bool
}

type Shadow struct {
	desc   gpu.TextureDesc
	levels []level
}

func New(desc gpu.TextureDesc) Shadow {
	s := Shadow{desc: desc, levels: make([]level, desc.MipLevels())}
	s.desc.Data = nil
	if desc.Data != nil {
		n := desc.Width * desc.Height * desc.LayerCount() * desc.Format.UploadBytesPerPixel()
		s.levels[0].data = append([]byte(nil), desc.Data[:n]...)
	}
	return s
}

func (s *Shadow) Base() []byte { return s.levels[0].data }

// Update expects a region already checked by gpu.ValidateTextureUpdate.
// A base-level update regenerates the mip chain, replacing earlier manual edits.
func (s *Shadow) Update(u gpu.TextureUpdate) {
	w, h, _ := s.desc.SubresourceSize(u.Subresource)
	bpp := s.desc.Format.UploadBytesPerPixel()
	l := &s.levels[u.Subresource.Mip]
	pixels := w * h * s.desc.LayerCount()
	if l.data == nil {
		l.data = make([]byte, pixels*bpp)
	}
	if u.Subresource.Mip != 0 {
		if l.dirty == nil {
			l.dirty = make([]uint64, (pixels+63)/64)
		} else if !l.active {
			clear(l.dirty)
		}
		l.active = true
	} else {
		for i := 1; i < len(s.levels); i++ {
			s.levels[i].active = false
		}
	}
	for y := 0; y < u.Height; y++ {
		p := (u.Subresource.Layer*h+u.Y+y)*w + u.X
		copy(l.data[p*bpp:(p+u.Width)*bpp], u.Data[y*u.Width*bpp:(y+1)*u.Width*bpp])
		if u.Subresource.Mip != 0 {
			for x := p; x < p+u.Width; x++ {
				l.dirty[x/64] |= 1 << (x % 64)
			}
		}
	}
}

// ManualMipRuns replays only explicitly written texels after the backend generates mipmaps from Base; whole zero-filled partial mip buffers would corrupt generated texels on context restore.
func (s *Shadow) ManualMipRuns(fn func(gpu.TextureUpdate)) {
	bpp := s.desc.Format.UploadBytesPerPixel()
	for mip := 1; mip < len(s.levels); mip++ {
		l := &s.levels[mip]
		if !l.active {
			continue
		}
		w, h, _ := s.desc.SubresourceSize(gpu.TextureSubresource{Mip: mip})
		for layer := 0; layer < s.desc.LayerCount(); layer++ {
			for y := 0; y < h; y++ {
				row := (layer*h + y) * w
				for x := 0; x < w; {
					p := row + x
					if l.dirty[p/64]&(1<<(p%64)) == 0 {
						x++
						continue
					}
					start := x
					for x++; x < w; x++ {
						p = row + x
						if l.dirty[p/64]&(1<<(p%64)) == 0 {
							break
						}
					}
					fn(gpu.TextureUpdate{Subresource: gpu.TextureSubresource{Layer: layer, Mip: mip}, X: start, Y: y, Width: x - start, Height: 1, Data: l.data[(row+start)*bpp : (row+x)*bpp]})
				}
			}
		}
	}
}
