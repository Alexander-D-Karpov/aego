package glstate

const (
	MaxUniformBuffers = 16
	MaxTextureUnits   = 16
	unknown           = ^uint32(0)
)

type Blend struct {
	Enabled        bool
	SrcRGB, DstRGB uint32
	SrcA, DstA     uint32
}

type Depth struct {
	Test  bool
	Write bool
	Func  uint32
}

type Cull struct {
	Enabled bool
	Face    uint32
}

type Rect struct {
	X, Y, W, H int32
}

type API interface {
	UseProgram(id uint32)
	BindVertexArray(id uint32)
	BindUniformBuffer(slot int, id uint32)
	BindTexture(unit int, id uint32)
	BindSampler(unit int, id uint32)
	BindFramebuffer(id uint32)
	SetBlend(b Blend)
	SetDepth(d Depth)
	SetCull(c Cull)
	Viewport(r Rect)
	Scissor(enabled bool, r Rect)
}

type Cache struct {
	api      API
	program  uint32
	vao      uint32
	fbo      uint32
	ubo      [MaxUniformBuffers]uint32
	tex      [MaxTextureUnits]uint32
	smp      [MaxTextureUnits]uint32
	blend    Blend
	blendOK  bool
	depth    Depth
	depthOK  bool
	cull     Cull
	cullOK   bool
	viewport Rect
	viewOK   bool
	scissor  Rect
	scissOn  bool
	scissOK  bool
}

func New(api API) *Cache {
	c := &Cache{api: api}
	c.Invalidate()
	return c
}

func (c *Cache) Invalidate() {
	c.program = unknown
	c.vao = unknown
	c.fbo = unknown
	for i := range c.ubo {
		c.ubo[i] = unknown
	}
	for i := range c.tex {
		c.tex[i] = unknown
		c.smp[i] = unknown
	}
	c.blendOK = false
	c.depthOK = false
	c.cullOK = false
	c.viewOK = false
	c.scissOK = false
}

func (c *Cache) UseProgram(id uint32) bool {
	if c.program == id {
		return false
	}
	c.program = id
	c.api.UseProgram(id)
	return true
}

func (c *Cache) Program() uint32 {
	return c.program
}

func (c *Cache) BindVertexArray(id uint32) bool {
	if c.vao == id {
		return false
	}
	c.vao = id
	c.api.BindVertexArray(id)
	return true
}

func (c *Cache) VertexArray() uint32 {
	return c.vao
}

func (c *Cache) BindFramebuffer(id uint32) bool {
	if c.fbo == id {
		return false
	}
	c.fbo = id
	c.api.BindFramebuffer(id)
	return true
}

func (c *Cache) BindUniformBuffer(slot int, id uint32) bool {
	if slot < 0 || slot >= MaxUniformBuffers || c.ubo[slot] == id {
		return false
	}
	c.ubo[slot] = id
	c.api.BindUniformBuffer(slot, id)
	return true
}

func (c *Cache) BindTexture(unit int, id uint32) bool {
	if unit < 0 || unit >= MaxTextureUnits || c.tex[unit] == id {
		return false
	}
	c.tex[unit] = id
	c.api.BindTexture(unit, id)
	return true
}

func (c *Cache) BindSampler(unit int, id uint32) bool {
	if unit < 0 || unit >= MaxTextureUnits || c.smp[unit] == id {
		return false
	}
	c.smp[unit] = id
	c.api.BindSampler(unit, id)
	return true
}

func (c *Cache) SetBlend(b Blend) bool {
	if !b.Enabled {
		b = Blend{}
	}
	if c.blendOK && c.blend == b {
		return false
	}
	c.blend, c.blendOK = b, true
	c.api.SetBlend(b)
	return true
}

func (c *Cache) SetDepth(d Depth) bool {
	if c.depthOK && c.depth == d {
		return false
	}
	c.depth, c.depthOK = d, true
	c.api.SetDepth(d)
	return true
}

func (c *Cache) SetCull(cl Cull) bool {
	if !cl.Enabled {
		cl = Cull{}
	}
	if c.cullOK && c.cull == cl {
		return false
	}
	c.cull, c.cullOK = cl, true
	c.api.SetCull(cl)
	return true
}

func (c *Cache) SetViewport(r Rect) bool {
	if c.viewOK && c.viewport == r {
		return false
	}
	c.viewport, c.viewOK = r, true
	c.api.Viewport(r)
	return true
}

func (c *Cache) SetScissor(enabled bool, r Rect) bool {
	if !enabled {
		r = Rect{}
	}
	if c.scissOK && c.scissOn == enabled && c.scissor == r {
		return false
	}
	c.scissOn, c.scissor, c.scissOK = enabled, r, true
	c.api.Scissor(enabled, r)
	return true
}

func (c *Cache) ForgetProgram(id uint32) {
	if c.program == id {
		c.program = unknown
	}
}

func (c *Cache) ForgetVertexArray(id uint32) {
	if c.vao == id {
		c.vao = unknown
	}
}

func (c *Cache) ForgetFramebuffer(id uint32) {
	if c.fbo == id {
		c.fbo = unknown
	}
}

func (c *Cache) ForgetUniformBuffer(id uint32) {
	for i, v := range c.ubo {
		if v == id {
			c.ubo[i] = unknown
		}
	}
}

func (c *Cache) ForgetTexture(id uint32) {
	for i, v := range c.tex {
		if v == id {
			c.tex[i] = unknown
		}
	}
}

func (c *Cache) ForgetSampler(id uint32) {
	for i, v := range c.smp {
		if v == id {
			c.smp[i] = unknown
		}
	}
}
