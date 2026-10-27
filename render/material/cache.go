package material

import (
	"aego/gpu"
	"aego/render/shader"
)

type Template func(d shader.Domain, lanes int, b shader.Blend) gpu.PipelineDesc

type progKey struct {
	sh      *Shader
	variant int
}

type pipeKey struct {
	sh      *Shader
	variant int
	blend   shader.Blend
}

type Cache struct {
	dev     gpu.Device
	es      bool
	tmpl    Template
	white   Tex
	progs   map[progKey]gpu.ShaderHandle
	pipes   map[pipeKey]gpu.PipelineHandle
	failed  map[progKey]bool
	used    map[*Shader]uint64
	mats    map[*Material]struct{}
	retired []*Shader
	frame   uint64
	restore int
	OnError func(sh *Shader, variant int, err error)
}

func NewCache(dev gpu.Device, es bool, tmpl Template) *Cache {
	return &Cache{
		dev:    dev,
		es:     es,
		tmpl:   tmpl,
		progs:  map[progKey]gpu.ShaderHandle{},
		pipes:  map[pipeKey]gpu.PipelineHandle{},
		failed: map[progKey]bool{},
		used:   map[*Shader]uint64{},
		mats:   map[*Material]struct{}{},
	}
}

func (c *Cache) SetWhite(t Tex) {
	c.white = t
}

// restorer is a device that loses its context (WebGL2); dynamic buffers come back empty and their owners fill them again.
type restorer interface {
	Restores() int
}

func (c *Cache) Begin() {
	c.frame++
	if r, ok := c.dev.(restorer); ok && r.Restores() != c.restore {
		c.restore = r.Restores()
		for m := range c.mats {
			m.dirty = true
		}
	}
}

func (c *Cache) Ready(m *Material) bool {
	return !c.failed[progKey{m.shader, m.variant}]
}

func (c *Cache) fail(sh *Shader, v int, err error) {
	c.failed[progKey{sh, v}] = true
	if c.OnError != nil {
		c.OnError(sh, v, err)
	}
}

func (c *Cache) program(sh *Shader, v int) (gpu.ShaderHandle, bool) {
	k := progKey{sh, v}
	if h, ok := c.progs[k]; ok {
		return h, true
	}
	if c.failed[k] {
		return gpu.ShaderHandle{}, false
	}
	src := &sh.Variants[v].Sources
	h, err := c.dev.CreateShader(gpu.ShaderDesc{
		Label: sh.Name,
		Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: src[SourceIndex(false, c.es)]},
			{Stage: gpu.StageFragment, Source: src[SourceIndex(true, c.es)]},
		},
	})
	if err != nil {
		c.fail(sh, v, err)
		return gpu.ShaderHandle{}, false
	}
	c.progs[k] = h
	return h, true
}

func (c *Cache) pipeline(sh *Shader, v int, b shader.Blend) (gpu.PipelineHandle, bool) {
	k := pipeKey{sh, v, b}
	if h, ok := c.pipes[k]; ok {
		return h, true
	}
	prog, ok := c.program(sh, v)
	if !ok {
		return gpu.PipelineHandle{}, false
	}
	d := c.tmpl(sh.Domain, sh.Schema.Lanes, b)
	d.Shader, d.Label = prog, sh.Name
	h, err := c.dev.CreatePipeline(d)
	if err != nil {
		c.fail(sh, v, err)
		return gpu.PipelineHandle{}, false
	}
	c.pipes[k] = h
	return h, true
}

func (c *Cache) advance(m *Material) {
	next := m.next
	m.next = nil
	v := next.Variant(m.mask)
	if v < 0 {
		m.failed = true
		return
	}
	if _, ok := c.pipeline(next, v, m.blend); !ok {
		m.failed = true
		return
	}
	m.promote(next, v)
	m.failed = false
}

func (c *Cache) upload(m *Material) bool {
	if len(m.data) == 0 || !m.dirty {
		return true
	}
	if m.ubSize != len(m.data) {
		if !m.ub.IsZero() {
			c.dev.DestroyBuffer(m.ub)
		}
		h, err := c.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: len(m.data), Dynamic: true, Label: "material." + m.Name})
		if err != nil {
			m.ub, m.ubSize = gpu.BufferHandle{}, 0
			return false
		}
		m.ub, m.ubSize = h, len(m.data)
		c.mats[m] = struct{}{}
	}
	c.dev.UpdateBuffer(m.ub, 0, m.data)
	m.dirty = false
	return true
}

func (c *Cache) Bind(p gpu.Pass, m *Material) bool {
	if m.next != nil {
		c.advance(m)
	}
	sh := m.shader
	pipe, ok := c.pipeline(sh, m.variant, m.blend)
	if !ok || !c.upload(m) {
		return false
	}
	c.used[sh] = c.frame
	p.SetPipeline(pipe)
	if !m.ub.IsZero() {
		p.SetUniformBuffer(shader.SlotMaterial, m.ub)
	}
	for _, i := range sh.Schema.Textures {
		slot := sh.Schema.Params[i].Slot
		t := m.tex[slot-shader.TexUser]
		if t.Texture.IsZero() {
			t = c.white
		} else if t.Sampler.IsZero() {
			t.Sampler = c.white.Sampler
		}
		p.SetTexture(slot, t.Texture, t.Sampler)
	}
	return true
}

func (c *Cache) Release(m *Material) {
	if !m.ub.IsZero() {
		c.dev.DestroyBuffer(m.ub)
	}
	m.ub, m.ubSize, m.dirty = gpu.BufferHandle{}, 0, true
	delete(c.mats, m)
}

func (c *Cache) Retire(sh *Shader) {
	c.retired = append(c.retired, sh)
}

func (c *Cache) End() {
	keep := c.retired[:0]
	for _, sh := range c.retired {
		if c.used[sh] == c.frame {
			keep = append(keep, sh)
			continue
		}
		c.drop(sh)
	}
	c.retired = keep
}

func (c *Cache) drop(sh *Shader) {
	for k, h := range c.pipes {
		if k.sh == sh {
			c.dev.DestroyPipeline(h)
			delete(c.pipes, k)
		}
	}
	for k, h := range c.progs {
		if k.sh == sh {
			c.dev.DestroyShader(h)
			delete(c.progs, k)
		}
	}
	for k := range c.failed {
		if k.sh == sh {
			delete(c.failed, k)
		}
	}
	delete(c.used, sh)
}

func (c *Cache) Close() {
	for _, h := range c.pipes {
		c.dev.DestroyPipeline(h)
	}
	for _, h := range c.progs {
		c.dev.DestroyShader(h)
	}
	for m := range c.mats {
		c.dev.DestroyBuffer(m.ub)
		m.ub, m.ubSize = gpu.BufferHandle{}, 0
	}
	clear(c.pipes)
	clear(c.progs)
	clear(c.mats)
}
