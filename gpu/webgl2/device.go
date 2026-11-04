//go:build js

package webgl2

import (
	"math/bits"
	"reflect"
	"strings"
	"syscall/js"
	"time"

	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/internal/bufferdiff"
	"aego/gpu/internal/glstate"
	"aego/gpu/internal/texshadow"
)

const (
	scratchUnit  = glstate.MaxTextureUnits - 1
	minScratch   = 64 << 10
	timerExt     = "EXT_disjoint_timer_query_webgl2"
	floatExt     = "EXT_color_buffer_float"
	infoExt      = "WEBGL_debug_renderer_info"
	loseExt      = "WEBGL_lose_context"
	bridgeSample = 15
)

type Surface interface {
	WebGL2() js.Value
}

type Options struct {
	Debug bool
	Log   *log.Logger
}

type fns struct {
	GetUniformBlockIndex, GetActiveUniformBlockParameter, GetActiveUniforms                                js.Value
	ActiveTexture, AttachShader, BeginQuery, BindBuffer, BindBufferBase, BindFramebuffer, BindRenderbuffer js.Value
	BindSampler, BindTexture, BindVertexArray, BlendFuncSeparate, BufferData, BufferSubData                js.Value
	CheckFramebufferStatus, Clear, ClearColor, ClearDepth, CompileShader                                   js.Value
	CreateBuffer, CreateFramebuffer, CreateProgram, CreateQuery, CreateRenderbuffer                        js.Value
	CreateSampler, CreateShader, CreateTexture, CreateVertexArray, CullFace                                js.Value
	DeleteBuffer, DeleteFramebuffer, DeleteProgram, DeleteQuery, DeleteRenderbuffer                        js.Value
	DeleteSampler, DeleteShader, DeleteTexture, DeleteVertexArray, DepthFunc, DepthMask, DetachShader      js.Value
	Disable, DrawArrays, DrawArraysInstanced, DrawBuffers, DrawElements, DrawElementsInstanced, Enable     js.Value
	EnableVertexAttribArray, EndQuery, FramebufferRenderbuffer, FramebufferTexture2D, FrontFace            js.Value
	GenerateMipmap, GetActiveUniform, GetActiveUniformBlockName, GetError, GetExtension, GetParameter      js.Value
	GetProgramInfoLog, GetProgramParameter, GetQueryParameter, GetShaderInfoLog, GetShaderParameter        js.Value
	GetUniformLocation, IsContextLost, LinkProgram, PixelStorei, ReadBuffer, ReadPixels                    js.Value
	RenderbufferStorage, SamplerParameteri, Scissor, ShaderSource, TexImage2D, TexParameteri               js.Value
	TexSubImage2D, Uniform1i, UniformBlockBinding, UseProgram, VertexAttribDivisor                         js.Value
	VertexAttribIPointer, VertexAttribPointer, Viewport                                                    js.Value
	TexImage3D, TexSubImage3D, FramebufferTextureLayer, BlendEquationSeparate, ColorMask, IsEnabled        js.Value
}

func bindAll(ctx js.Value) (fns, error) {
	var f fns
	v := reflect.ValueOf(&f).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		n := t.Field(i).Name
		name := strings.ToLower(n[:1]) + n[1:]
		m := ctx.Get(name)
		if m.Type() != js.TypeFunction {
			return f, errs.New(errs.GPUInit, "the WebGL2 context has no "+name)
		}
		v.Field(i).Set(reflect.ValueOf(m.Call("bind", ctx)))
	}
	return f, nil
}

type objects struct {
	vals []js.Value
	free []uint32
}

func (o *objects) put(v js.Value) uint32 {
	if n := len(o.free); n > 0 {
		id := o.free[n-1]
		o.free = o.free[:n-1]
		o.vals[id] = v
		return id
	}
	if len(o.vals) == 0 {
		o.vals = append(o.vals, js.Null())
	}
	o.vals = append(o.vals, v)
	return uint32(len(o.vals) - 1)
}

func (o *objects) get(id uint32) js.Value {
	if id == 0 || int(id) >= len(o.vals) {
		return js.Null()
	}
	return o.vals[id]
}

func (o *objects) reuse(id uint32, v js.Value) uint32 {
	if id == 0 {
		return o.put(v)
	}
	o.vals[id] = v
	return id
}

func (o *objects) drop(id uint32) js.Value {
	v := o.get(id)
	if id != 0 && int(id) < len(o.vals) {
		o.vals[id] = js.Null()
		o.free = append(o.free, id)
	}
	return v
}

type glBuffer struct {
	last    bufferdiff.LastWrite
	id      uint32
	usage   gpu.BufferUsage
	size    int
	index   gpu.IndexFormat
	dynamic bool
	shadow  []byte
}

type glTexture struct {
	opaqueRegions map[[4]int]bool
	rendered      bool
	id            uint32
	w, h          int
	info          texInfo
	mips          bool
	desc          gpu.TextureDesc
	target        uint32
	shadow        texshadow.Shadow
}

type glSampler struct {
	id   uint32
	desc gpu.SamplerDesc
}

type glShader struct {
	program uint32
	desc    gpu.ShaderDesc
}

type glPipeline struct {
	vao     uint32
	program uint32
	prim    gpu.Primitive
	mode    uint32
	attrs   []glstate.Attrib
	divisor []uint32
	used    [gpu.MaxVertexSlots]bool
	bind    glstate.VertexBinding
	blend   glstate.Blend
	depth   glstate.Depth
	cull    glstate.Cull
}

type resourceKind uint8

const (
	kindBuffer resourceKind = iota
	kindTexture
	kindSampler
	kindShader
	kindPipeline
	kindQuery
)

type pendingDestroy struct {
	kind       resourceKind
	index, gen uint32
}

type Device struct {
	warnedDepthSize  bool
	log              *log.ChannelLogger
	debug            bool
	caps             gpu.Capabilities
	ctx              js.Value
	canvas           js.Value
	f                fns
	obj              objects
	cache            *glstate.Cache
	buffers          *handle.Arena[glBuffer]
	textures         *handle.Arena[glTexture]
	samplers         *handle.Arena[glSampler]
	shaders          *handle.Arena[glShader]
	pipelines        *handle.Arena[glPipeline]
	queries          *handle.Arena[glQuery]
	fbos             map[fboKey]uint32
	frame            frame
	pass             pass
	inFrame          bool
	window           bool
	activeQuery      uint32
	pending          []pendingDestroy
	fbW, fbH         int
	bb               backbuffer
	out              output
	u8, f32          js.Value
	buf              js.Value
	bufN             int
	drawColor        js.Value
	drawNone         js.Value
	loseCtx          js.Value
	parallel         js.Value
	contextGen       uint64
	compiles         map[*compilation]struct{}
	lost             bool
	restores         int
	calls            uint64
	mark             uint64
	lastCalls        uint64
	st               gpu.BridgeStats
	onLost           js.Func
	onRestored       js.Func
	watching         bool
	targetErrors     [16]string
	targetErrorCount int
}

func newArena[T any](l *log.Logger, kind string, capacity int) *handle.Arena[T] {
	a := handle.NewArena[T](capacity)
	cl := l.With(log.GPU)
	a.OnGrow = func(from, to int) {
		cl.Warn("resource arena grows", log.F("kind", kind), log.F("from", from), log.F("to", to))
	}
	return a
}

func New(s Surface, opts Options) (*Device, error) {
	ctx := s.WebGL2()
	if !ctx.Truthy() {
		return nil, errs.New(errs.GPUInit, "the window has no WebGL2 context")
	}
	f, err := bindAll(ctx)
	if err != nil {
		return nil, err
	}
	logger := opts.Log
	if logger == nil {
		logger = log.Nop()
	}
	g := js.Global()
	d := &Device{
		log:       logger.With(log.GPU),
		debug:     opts.Debug,
		ctx:       ctx,
		canvas:    ctx.Get("canvas"),
		f:         f,
		buffers:   newArena[glBuffer](logger, "buffer", 256),
		textures:  newArena[glTexture](logger, "texture", 128),
		samplers:  newArena[glSampler](logger, "sampler", 16),
		shaders:   newArena[glShader](logger, "shader", 32),
		pipelines: newArena[glPipeline](logger, "pipeline", 32),
		queries:   newArena[glQuery](logger, "query", 16),
		fbos:      make(map[fboKey]uint32, 16),
		pending:   make([]pendingDestroy, 0, 64),
		u8:        g.Get("Uint8Array"),
		f32:       g.Get("Float32Array"),
		drawColor: js.ValueOf([]any{glColorAttachment0}),
		drawNone:  js.ValueOf([]any{glNone}),
	}
	d.frame.d = d
	d.pass.d = d
	d.cache = glstate.New(api{d: d})
	d.compiles = make(map[*compilation]struct{})
	d.queryCaps()
	d.initState()
	if err := d.initOutput(); err != nil {
		return nil, err
	}
	d.watchLoss()
	return d, nil
}

func num(v js.Value) int {
	if v.Type() == js.TypeNumber {
		return v.Int()
	}
	return 0
}

func (d *Device) call(f js.Value, args ...any) js.Value {
	d.calls++
	if d.calls&bridgeSample != 0 {
		return f.Invoke(args...)
	}
	t := time.Now()
	v := f.Invoke(args...)
	d.st.BridgeNs += uint64(time.Since(t)) * (bridgeSample + 1)
	return v
}

func (d *Device) queryCall(f js.Value, args ...any) js.Value {
	d.st.Queries++
	return d.call(f, args...)
}

func (d *Device) BridgeStats() gpu.BridgeStats {
	s := d.st
	s.Calls = d.calls - d.st.Queries
	return s
}

func (d *Device) param(p int) js.Value {
	return d.call(d.f.GetParameter, p)
}

func (d *Device) scratch(n int) js.Value {
	if n > d.bufN {
		c := 1 << bits.Len(uint(max(n, minScratch)-1))
		d.buf = d.u8.New(c)
		d.bufN = c
	}
	return d.buf
}

func (d *Device) bytes(b []byte) js.Value {
	v := d.scratch(len(b))
	js.CopyBytesToJS(v, b)
	return v
}

func (d *Device) view(i texInfo, data []byte) js.Value {
	u := d.bytes(data)
	if i.xtype == glFloat {
		return d.f32.New(u.Get("buffer"), 0, len(data)/4)
	}
	return u
}

func (d *Device) queryCaps() {
	c := &d.caps
	c.Vendor = d.param(glVendor).String()
	c.Renderer = d.param(glRenderer).String()
	if d.call(d.f.GetExtension, infoExt).Truthy() {
		if v := d.param(glUnmaskedVendor); v.Type() == js.TypeString {
			c.Vendor = v.String()
		}
		if v := d.param(glUnmaskedRenderer); v.Type() == js.TypeString {
			c.Renderer = v.String()
		}
	}
	c.Version = d.param(glVersion).String()
	c.GLSL = d.param(glShadingLanguageVersion).String()
	c.MaxTextureSize = num(d.param(glMaxTextureSize))
	c.MaxVertexAttributes = num(d.param(glMaxVertexAttribs))
	c.MaxUniformBlockSize = num(d.param(glMaxUniformBlockSize))
	c.MaxTextureUnits = min(num(d.param(glMaxCombinedTextureImageUnits)), scratchUnit)
	c.MaxFragmentTextureUnits = num(d.param(glMaxTextureImageUnits))
	c.MaxArrayTextureLayers = num(d.param(glMaxArrayTextureLayers))
	c.MaxSamples = num(d.param(glMaxSamples))
	c.Instancing = true
	c.TextureArrays = true
	c.SRGB = true
	c.ESSL = true
	c.OriginBottomLeft = true
	c.TimerQueries = d.call(d.f.GetExtension, timerExt).Truthy()
	c.RGBA16FRenderable = false
	if d.call(d.f.GetExtension, floatExt).Truthy() {
		c.RGBA16FRenderable = d.probeRGBA16F()
	}
	d.loseCtx = d.call(d.f.GetExtension, loseExt)
	d.parallel = d.call(d.f.GetExtension, "KHR_parallel_shader_compile")
}

func (d *Device) initState() {
	d.call(d.f.PixelStorei, glUnpackAlignment, 1)
	d.call(d.f.PixelStorei, glPackAlignment, 1)
}

func (d *Device) checkError(op, label string) error {
	if !d.debug {
		return nil
	}
	var codes []string
	for i := 0; i < 8; i++ {
		e := uint32(num(d.call(d.f.GetError)))
		if e == glNoError {
			break
		}
		codes = append(codes, glErrorName(e))
	}
	if len(codes) == 0 {
		return nil
	}
	return errs.New(errs.GPUResource, op+" failed", log.F("label", label), log.F("gl", strings.Join(codes, ",")))
}

func (d *Device) Caps() gpu.Capabilities {
	return d.caps
}

func (d *Device) Calls() uint64 {
	return d.lastCalls
}

func (d *Device) Lost() bool {
	return d.lost
}

func (d *Device) Restores() int {
	return d.restores
}

func (d *Device) SimulateLoss(lose bool) bool {
	if !d.loseCtx.Truthy() {
		return false
	}
	if lose {
		d.loseCtx.Call("loseContext")
	} else {
		d.loseCtx.Call("restoreContext")
	}
	return true
}

func (d *Device) markLost() {
	if d.lost {
		return
	}
	d.lost = true
	d.contextGen++
	for job := range d.compiles {
		job.Cancel()
	}
	d.log.Warn("WebGL context lost, rendering stops until the browser restores it")
}

func (d *Device) watchLoss() {
	if !d.canvas.Truthy() {
		return
	}
	d.onLost = js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) > 0 {
			a[0].Call("preventDefault")
		}
		d.markLost()
		return nil
	})
	d.onRestored = js.FuncOf(func(js.Value, []js.Value) any {
		d.restore()
		return nil
	})
	d.canvas.Call("addEventListener", "webglcontextlost", d.onLost)
	d.canvas.Call("addEventListener", "webglcontextrestored", d.onRestored)
	d.watching = true
}

func (d *Device) bindScratch(id uint32) {
	d.call(d.f.ActiveTexture, glTexture0+scratchUnit)
	d.cache.BindTexture(scratchUnit, id)
}

func (d *Device) BeginFrame(pixelW, pixelH int) gpu.Frame {
	d.fbW, d.fbH = pixelW, pixelH
	d.frame.stats = gpu.FrameStats{}
	d.inFrame = true
	d.window = false
	d.mark = d.calls
	return &d.frame
}

func (d *Device) EndFrame() {
	if d.pass.active {
		d.log.Warn("render pass was not ended before EndFrame")
		d.pass.active = false
	}
	if d.window && !d.lost {
		d.present()
	}
	d.inFrame = false
	for _, p := range d.pending {
		d.destroyNow(p.kind, p.index, p.gen)
	}
	d.pending = d.pending[:0]
	d.lastCalls = d.calls - d.mark
}

func (d *Device) Close() {
	for job := range d.compiles {
		job.Cancel()
	}
	d.pending = d.pending[:0]
	d.inFrame = false
	d.cache.BindVertexArray(0)
	d.cache.UseProgram(0)
	d.cache.BindFramebuffer(0)
	d.deleteFramebuffers()
	d.freeBackbuffer()
	d.pipelines.Each(func(_ handle.Handle[glPipeline], p *glPipeline) {
		d.call(d.f.DeleteVertexArray, d.obj.drop(p.vao))
	})
	d.shaders.Each(func(_ handle.Handle[glShader], s *glShader) {
		d.call(d.f.DeleteProgram, d.obj.drop(s.program))
	})
	d.samplers.Each(func(_ handle.Handle[glSampler], s *glSampler) {
		d.call(d.f.DeleteSampler, d.obj.drop(s.id))
	})
	d.textures.Each(func(_ handle.Handle[glTexture], t *glTexture) {
		d.call(d.f.DeleteTexture, d.obj.drop(t.id))
	})
	d.buffers.Each(func(_ handle.Handle[glBuffer], b *glBuffer) {
		d.call(d.f.DeleteBuffer, d.obj.drop(b.id))
	})
	d.queries.Each(func(_ handle.Handle[glQuery], q *glQuery) {
		d.call(d.f.DeleteQuery, d.obj.drop(q.id))
	})
	d.freeOutput()
	d.pipelines.Clear()
	d.shaders.Clear()
	d.samplers.Clear()
	d.textures.Clear()
	d.buffers.Clear()
	d.queries.Clear()
	d.activeQuery = 0
	d.cache.Invalidate()
	if d.watching {
		d.canvas.Call("removeEventListener", "webglcontextlost", d.onLost)
		d.canvas.Call("removeEventListener", "webglcontextrestored", d.onRestored)
		d.onLost.Release()
		d.onRestored.Release()
		d.watching = false
	}
}

func (d *Device) ContextGeneration() uint64 { return d.contextGen }
func (d *Device) ContextLost() bool         { return d.lost }

func (d *Device) ResourceStats() gpu.ResourceStats {
	st := gpu.ResourceStats{Buffers: d.buffers.Len(), Textures: d.textures.Len(), Samplers: d.samplers.Len(), Shaders: d.shaders.Len(), Pipelines: d.pipelines.Len(), PendingCompiles: len(d.compiles)}
	d.textures.Each(func(_ handle.Handle[glTexture], t *glTexture) { st.TextureStorageBytes += t.desc.StorageBytes() })
	st.Framebuffers = len(d.fbos)
	return st
}
