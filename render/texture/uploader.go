package texture

import (
	"sync"

	"aego/gpu"
)

type UploadOptions struct {
	Mips  bool
	Label string
}

type job struct {
	data  *Data
	opt   UploadOptions
	tex   gpu.TextureHandle
	err   error
	ready bool
}

type Pending struct {
	j  *job
	up *Uploader
}

func (p Pending) Ready() (gpu.TextureHandle, bool) {
	if p.j == nil {
		return gpu.TextureHandle{}, false
	}
	p.up.mu.Lock()
	defer p.up.mu.Unlock()
	return p.j.tex, p.j.ready
}

func (p Pending) Err() error {
	if p.j == nil {
		return nil
	}
	p.up.mu.Lock()
	defer p.up.mu.Unlock()
	return p.j.err
}

type Uploader struct {
	dev     gpu.Device
	mu      sync.Mutex
	queue   []*job
	pending int
}

func NewUploader(dev gpu.Device) *Uploader {
	return &Uploader{dev: dev, queue: make([]*job, 0, 16)}
}

func (u *Uploader) Upload(d *Data, opt UploadOptions) (gpu.TextureHandle, error) {
	if err := d.Validate(); err != nil {
		return gpu.TextureHandle{}, err
	}
	if d.Format != gpu.TexR8 {
		d.Premultiply()
	}
	return u.dev.CreateTexture(gpu.TextureDesc{
		Width:  d.Width,
		Height: d.Height,
		Format: d.Format,
		Mips:   opt.Mips,
		Data:   d.Pixels,
		Label:  opt.Label,
	})
}

func (u *Uploader) Enqueue(d *Data, opt UploadOptions) Pending {
	j := &job{data: d, opt: opt}
	u.mu.Lock()
	u.queue = append(u.queue, j)
	u.pending++
	u.mu.Unlock()
	return Pending{j: j, up: u}
}

func (u *Uploader) Pending() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.pending
}

func (u *Uploader) Drain(maxBytes int) int {
	if maxBytes <= 0 {
		maxBytes = 1 << 30
	}
	spent := 0
	for spent < maxBytes {
		u.mu.Lock()
		if len(u.queue) == 0 {
			u.mu.Unlock()
			return spent
		}
		j := u.queue[0]
		u.queue = u.queue[1:]
		u.mu.Unlock()

		tex, err := u.Upload(j.data, j.opt)
		spent += len(j.data.Pixels)

		u.mu.Lock()
		j.tex, j.err, j.ready = tex, err, true
		j.data = nil
		u.pending--
		u.mu.Unlock()
	}
	return spent
}
