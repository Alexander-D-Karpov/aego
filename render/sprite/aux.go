package sprite

import (
	"unsafe"

	"aego/gpu"
	"aego/render/shader"
)

const (
	AuxStride             = shader.AuxStride
	AuxSlot               = shader.AuxSlot
	AffineLocation        = shader.AffineLocation
	NormalUVLocation      = shader.NormalUVLocation
	NormalLocalLocation   = shader.NormalLocalLocation
	EmissionUVLocation    = shader.EmissionUVLocation
	EmissionLocalLocation = shader.EmissionLocalLocation
)

// Aux is an optional per-instance stream, independent of material overrides.
// Affine stores (m00-1,m10,m01,m11); the disabled float attribute's (0,0,0,1)
// therefore decodes as identity. NormalUV and NormalLocal map the normal's
// independently packed rectangle to its original source canvas.
type Aux struct {
	Affine        [4]float32
	NormalUV      [4]float32
	NormalLocal   [4]float32
	EmissionUV    [4]float32
	EmissionLocal [4]float32
}

const _ = uint(AuxStride - unsafe.Sizeof(Aux{}))
const _ = uint(unsafe.Sizeof(Aux{}) - AuxStride)

func identityAux() Aux {
	return Aux{Affine: [4]float32{0, 0, 0, 1}, NormalLocal: [4]float32{0, 0, 1, 1}, EmissionLocal: [4]float32{0, 0, 1, 1}}
}

func makeAux(d *Draw) Aux {
	a := identityAux()
	a.Affine = d.Basis()
	a.Affine[0]--
	if d.Lighting && !d.Normal.IsZero() {
		r := d.Normal
		a.NormalUV = [4]float32{r.UVMin.X, r.UVMin.Y, r.UVMax.X, r.UVMax.Y}
		a.NormalLocal = r.LocalRect()
	}
	if d.Lighting && d.Emission.Valid() {
		r := d.Emission.Region
		a.EmissionUV = [4]float32{r.UVMin.X, r.UVMin.Y, r.UVMax.X, r.UVMax.Y}
		a.EmissionLocal = r.LocalRect()
	}
	return a
}

func (b *Batcher) collectAux(from, to int) int {
	if len(b.aux) == 0 {
		return -1
	}
	found := false
	for i := from; i < to; i++ {
		if b.sortedAux[i] >= 0 {
			found = true
			break
		}
	}
	if !found {
		return -1
	}
	first := len(b.auxOut)
	for i := from; i < to; i++ {
		a := identityAux()
		if j := b.sortedAux[i]; j >= 0 {
			a = b.aux[j]
		}
		b.auxOut = append(b.auxOut, a)
	}
	return first
}

// UploadAux leaves the ordinary sprite path untouched until a draw needs an
// affine basis or lighting. Capacity growth is reused by subsequent frames.
func (b *Batcher) UploadAux() bool {
	n := len(b.auxOut)
	if n == 0 {
		return true
	}
	if n > b.auxCap {
		c := max(n, 256, b.auxCap*2)
		h, err := b.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferVertex, Size: c * AuxStride, Dynamic: true, Label: "sprite.aux"})
		if err != nil {
			return false
		}
		if !b.auxVB.IsZero() {
			b.dev.DestroyBuffer(b.auxVB)
		}
		b.auxVB, b.auxCap = h, c
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&b.auxOut[0])), n*AuxStride)
	b.dev.UpdateBuffer(b.auxVB, 0, raw)
	return true
}

func (b *Batcher) AuxBuffer() gpu.BufferHandle { return b.auxVB }
func (b *Batcher) AuxBytes() int               { return len(b.auxOut) * AuxStride }

// AuxLayout adds the affine input used by the engine's plain sprite shader.
// Material shaders add their feature-specific channel inputs in material.Cache.
// A disabled stream uses the backend's generic float attribute defaults.
func AuxLayout(l gpu.VertexLayout) gpu.VertexLayout {
	l.Attrs = append(l.Attrs, gpu.VertexAttr{Location: AffineLocation, Slot: AuxSlot, Format: gpu.VFFloat4, Offset: 0})
	l.Strides[AuxSlot] = AuxStride
	l.PerInstance[AuxSlot] = true
	return l
}
