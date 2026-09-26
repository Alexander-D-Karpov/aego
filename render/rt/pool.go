package rt

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
	"aego/render/texture"
)

const BucketSize = 64

func Bucket(v int) int {
	if v < 1 {
		v = 1
	}
	return (v + BucketSize - 1) / BucketSize * BucketSize
}

type Target struct {
	Texture gpu.TextureHandle
	W, H    int
	BucketW int
	BucketH int
	pool    *Pool
	inUse   bool
}

func (t *Target) Region() texture.Region {
	return texture.Whole(t.Texture, t.BucketW, t.BucketH).Sub(0, 0, t.W, t.H)
}

type Pool struct {
	dev    gpu.Device
	format gpu.TextureFormat
	label  string
	free   []*Target
	live   int
}

func New(dev gpu.Device, format gpu.TextureFormat, label string) *Pool {
	return &Pool{dev: dev, format: format, label: label}
}

func (p *Pool) Acquire(w, h int) (*Target, error) {
	if w <= 0 || h <= 0 {
		return nil, errs.New(errs.RenderTarget, "render target size must be positive",
			log.F("w", w), log.F("h", h))
	}
	bw, bh := Bucket(w), Bucket(h)
	for i, t := range p.free {
		if t.BucketW == bw && t.BucketH == bh {
			p.free = append(p.free[:i], p.free[i+1:]...)
			t.W, t.H = w, h
			t.inUse = true
			return t, nil
		}
	}
	tex, err := p.dev.CreateTexture(gpu.TextureDesc{Width: bw, Height: bh, Format: p.format, Label: p.label})
	if err != nil {
		return nil, err
	}
	p.live++
	return &Target{Texture: tex, W: w, H: h, BucketW: bw, BucketH: bh, pool: p, inUse: true}, nil
}

func (p *Pool) Resize(t *Target, w, h int) (*Target, error) {
	if t != nil && t.pool == p && t.inUse && Bucket(w) == t.BucketW && Bucket(h) == t.BucketH {
		t.W, t.H = w, h
		return t, nil
	}
	p.Release(t)
	return p.Acquire(w, h)
}

func (p *Pool) Release(t *Target) {
	if t == nil || t.pool != p || !t.inUse {
		return
	}
	t.inUse = false
	p.free = append(p.free, t)
}

func (p *Pool) Trim(keep int) {
	if keep < 0 {
		keep = 0
	}
	for len(p.free) > keep {
		t := p.free[0]
		p.free = p.free[1:]
		p.dev.DestroyTexture(t.Texture)
		p.live--
	}
}

func (p *Pool) Live() int {
	return p.live
}

func (p *Pool) Free() int {
	return len(p.free)
}

func (p *Pool) Close() {
	p.Trim(0)
}
