package gl33

import (
	"strings"
	"unsafe"

	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/gl33/gl"
	"aego/gpu/internal/glstate"
)

const scratchUnit = glstate.MaxTextureUnits - 1

type Options struct {
	Debug bool
	Log   *log.Logger
}

type glBuffer struct {
	id    uint32
	usage gpu.BufferUsage
	size  int
	index gpu.IndexFormat
}

type glTexture struct {
	id     uint32
	w, h   int
	info   texInfo
	mips   bool
	desc   gpu.TextureDesc
	target uint32
}

type glSampler struct {
	id uint32
}

type glShader struct {
	program uint32
}

const _ = uint(glstate.MaxVertexSlots - gpu.MaxVertexSlots)
const _ = uint(gpu.MaxVertexSlots - glstate.MaxVertexSlots)

type glPipeline struct {
	vao     uint32
	program uint32
	prim    gpu.Primitive
	mode    uint32
	attrs   []glstate.Attrib
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
	api              realAPI
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
	activeQuery      uint32
	pending          []pendingDestroy
	fbW, fbH         int
	debugProc        gl.DebugProc
	depth            texInfo
	targetErrors     [16]string
	targetErrorCount int
}

func New(getProc func(string) unsafe.Pointer, opts Options) (*Device, error) {
	if err := gl.InitWithProcAddrFunc(getProc); err != nil {
		return nil, errs.Wrap(err, errs.GPUInit, "load OpenGL 3.3 core functions")
	}
	logger := opts.Log
	if logger == nil {
		logger = log.Nop()
	}
	d := &Device{
		log:       logger.With(log.GPU),
		debug:     opts.Debug,
		api:       realAPI{},
		cache:     glstate.New(realAPI{}),
		buffers:   newArena[glBuffer](logger, "buffer", 256),
		textures:  newArena[glTexture](logger, "texture", 128),
		samplers:  newArena[glSampler](logger, "sampler", 16),
		shaders:   newArena[glShader](logger, "shader", 32),
		pipelines: newArena[glPipeline](logger, "pipeline", 32),
		queries:   newArena[glQuery](logger, "query", 16),
		fbos:      make(map[fboKey]uint32, 16),
		pending:   make([]pendingDestroy, 0, 64),
	}
	d.frame.d = d
	d.pass.d = d
	d.queryCaps()
	d.depth = d.probeDepth()
	d.caps.RGBA16FRenderable = d.probeRGBA16F()
	if d.debug && d.caps.DebugOutput {
		d.installDebugOutput()
	}
	gl.Enable(gl.FRAMEBUFFER_SRGB)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	return d, nil
}

func newArena[T any](l *log.Logger, kind string, capacity int) *handle.Arena[T] {
	a := handle.NewArena[T](capacity)
	cl := l.With(log.GPU)
	a.OnGrow = func(from, to int) {
		cl.Warn("resource arena grows", log.F("kind", kind), log.F("from", from), log.F("to", to))
	}
	return a
}

func (d *Device) Caps() gpu.Capabilities {
	return d.caps
}

func (d *Device) queryCaps() {
	c := &d.caps
	c.Vendor = gl.GoStr(gl.GetString(gl.VENDOR))
	c.Renderer = gl.GoStr(gl.GetString(gl.RENDERER))
	c.Version = gl.GoStr(gl.GetString(gl.VERSION))
	c.GLSL = gl.GoStr(gl.GetString(gl.SHADING_LANGUAGE_VERSION))
	c.MaxTextureSize = getInt(gl.MAX_TEXTURE_SIZE)
	c.MaxVertexAttributes = getInt(gl.MAX_VERTEX_ATTRIBS)
	c.MaxUniformBlockSize = getInt(gl.MAX_UNIFORM_BLOCK_SIZE)
	c.MaxTextureUnits = min(getInt(gl.MAX_COMBINED_TEXTURE_IMAGE_UNITS), scratchUnit)
	c.MaxFragmentTextureUnits = getInt(gl.MAX_TEXTURE_IMAGE_UNITS)
	c.MaxArrayTextureLayers = getInt(gl.MAX_ARRAY_TEXTURE_LAYERS)
	c.MaxSamples = getInt(gl.MAX_SAMPLES)
	c.Instancing = true
	c.TimerQueries = true
	c.TextureArrays = true
	major, minor := getInt(gl.MAJOR_VERSION), getInt(gl.MINOR_VERSION)
	exts := extensions()
	c.DebugOutput = exts["GL_KHR_debug"] || major > 4 || (major == 4 && minor >= 3)
	c.SRGB = defaultFramebufferSRGB()
	c.ClipZeroToOne = false
	c.OriginBottomLeft = true
}

func getInt(pname uint32) int {
	var v int32
	gl.GetIntegerv(pname, &v)
	return int(v)
}

func extensions() map[string]bool {
	n := getInt(gl.NUM_EXTENSIONS)
	out := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		out[gl.GoStr(gl.GetStringi(gl.EXTENSIONS, uint32(i)))] = true
	}
	return out
}

func defaultFramebufferSRGB() bool {
	var enc int32
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	gl.GetFramebufferAttachmentParameteriv(gl.FRAMEBUFFER, gl.BACK_LEFT, gl.FRAMEBUFFER_ATTACHMENT_COLOR_ENCODING, &enc)
	if gl.GetError() != gl.NO_ERROR {
		gl.GetFramebufferAttachmentParameteriv(gl.FRAMEBUFFER, gl.BACK, gl.FRAMEBUFFER_ATTACHMENT_COLOR_ENCODING, &enc)
		for gl.GetError() != gl.NO_ERROR {
		}
	}
	return enc == gl.SRGB
}

func (d *Device) installDebugOutput() {
	logger := d.log.Logger()
	d.debugProc = func(source, gltype, id, severity uint32, length int32, message string, userParam unsafe.Pointer) {
		lv := log.Debug
		switch severity {
		case gl.DEBUG_SEVERITY_HIGH:
			lv = log.Error
		case gl.DEBUG_SEVERITY_MEDIUM:
			lv = log.Warn
		case gl.DEBUG_SEVERITY_LOW:
			lv = log.Info
		}
		if !logger.Enabled(lv) {
			return
		}
		logger.Log(log.GPU, lv, strings.TrimSpace(message),
			log.F("id", id),
			log.F("type", debugTypeName(gltype)),
			log.F("source", debugSourceName(source)))
	}
	gl.Enable(gl.DEBUG_OUTPUT)
	gl.Enable(gl.DEBUG_OUTPUT_SYNCHRONOUS)
	gl.DebugMessageCallback(d.debugProc, nil)
	gl.DebugMessageControl(gl.DONT_CARE, gl.DONT_CARE, gl.DEBUG_SEVERITY_NOTIFICATION, 0, nil, false)
	d.log.Debug("KHR_debug output enabled")
}

func (d *Device) label(kind, id uint32, label string) {
	if !d.debug || !d.caps.DebugOutput || label == "" {
		return
	}
	cs, free := gl.Strs(label + "\x00")
	gl.ObjectLabel(kind, id, int32(len(label)), *cs)
	free()
}

func (d *Device) checkError(op, label string) error {
	if !d.debug {
		return nil
	}
	var codes []string
	for i := 0; i < 8; i++ {
		e := gl.GetError()
		if e == gl.NO_ERROR {
			break
		}
		codes = append(codes, glErrorName(e))
	}
	if len(codes) == 0 {
		return nil
	}
	return errs.New(errs.GPUResource, op+" failed",
		log.F("label", label), log.F("gl", strings.Join(codes, ",")))
}

func (d *Device) BeginFrame(pixelW, pixelH int) gpu.Frame {
	d.fbW, d.fbH = pixelW, pixelH
	d.frame.stats = gpu.FrameStats{}
	d.inFrame = true
	return &d.frame
}

func (d *Device) EndFrame() {
	if d.pass.active {
		d.log.Warn("render pass was not ended before EndFrame")
		d.pass.active = false
	}
	d.inFrame = false
	for _, p := range d.pending {
		d.destroyNow(p.kind, p.index, p.gen)
	}
	d.pending = d.pending[:0]
}

func (d *Device) Close() {
	d.pending = d.pending[:0]
	d.inFrame = false
	d.cache.BindVertexArray(0)
	d.cache.UseProgram(0)
	d.cache.BindFramebuffer(0)
	d.deleteFramebuffers()
	d.pipelines.Each(func(_ handle.Handle[glPipeline], p *glPipeline) {
		id := p.vao
		gl.DeleteVertexArrays(1, &id)
	})
	d.shaders.Each(func(_ handle.Handle[glShader], s *glShader) {
		gl.DeleteProgram(s.program)
	})
	d.samplers.Each(func(_ handle.Handle[glSampler], s *glSampler) {
		id := s.id
		gl.DeleteSamplers(1, &id)
	})
	d.textures.Each(func(_ handle.Handle[glTexture], t *glTexture) {
		id := t.id
		gl.DeleteTextures(1, &id)
	})
	d.buffers.Each(func(_ handle.Handle[glBuffer], b *glBuffer) {
		id := b.id
		gl.DeleteBuffers(1, &id)
	})
	d.queries.Each(func(_ handle.Handle[glQuery], q *glQuery) {
		id := q.id
		gl.DeleteQueries(1, &id)
	})
	d.pipelines.Clear()
	d.shaders.Clear()
	d.samplers.Clear()
	d.textures.Clear()
	d.buffers.Clear()
	d.queries.Clear()
	d.activeQuery = 0
	d.cache.Invalidate()
}

func (d *Device) formatInfo(f gpu.TextureFormat) (texInfo, bool) {
	if f == gpu.TexDepth24Stencil8 && d.depth.internal != 0 {
		return d.depth, true
	}
	return textureInfo(f)
}

func (d *Device) probeDepth() texInfo {
	for i, cand := range depthFormats {
		if !d.depthWorks(cand) {
			continue
		}
		if i > 0 {
			d.log.Warn("depth-stencil render targets are incomplete on this driver, using a depth-only format",
				log.F("format", cand.name), log.F("renderer", d.caps.Renderer))
		}
		return cand
	}
	d.log.Error("no depth format is renderable, 3D views will fail to create render targets", log.F("renderer", d.caps.Renderer))
	return depthFormats[0]
}

func (d *Device) depthWorks(info texInfo) bool {
	for gl.GetError() != gl.NO_ERROR {
	}
	var tex [2]uint32
	gl.GenTextures(2, &tex[0])
	d.bindScratch(tex[0])
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, 4, 4, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	d.bindScratch(tex[1])
	gl.TexImage2D(gl.TEXTURE_2D, 0, info.internal, 4, 4, 0, info.format, info.xtype, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	var fbo uint32
	gl.GenFramebuffers(1, &fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex[0], 0)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, info.attach, gl.TEXTURE_2D, tex[1], 0)
	ok := gl.CheckFramebufferStatus(gl.FRAMEBUFFER) == gl.FRAMEBUFFER_COMPLETE && gl.GetError() == gl.NO_ERROR
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	gl.DeleteFramebuffers(1, &fbo)
	d.cache.ForgetTexture(tex[0])
	d.cache.ForgetTexture(tex[1])
	gl.DeleteTextures(2, &tex[0])
	d.cache.Invalidate()
	return ok
}

func (d *Device) ResourceStats() gpu.ResourceStats {
	st := gpu.ResourceStats{Buffers: d.buffers.Len(), Textures: d.textures.Len(), Samplers: d.samplers.Len(), Shaders: d.shaders.Len(), Pipelines: d.pipelines.Len()}
	d.textures.Each(func(_ handle.Handle[glTexture], t *glTexture) { st.TextureStorageBytes += t.desc.StorageBytes() })
	st.Framebuffers = len(d.fbos)
	return st
}
