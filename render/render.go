package render

import (
	"encoding/binary"
	"math"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/debug"
	"aego/render/gpuprof"
	"aego/render/sprite"
	"aego/render/text"
	"aego/render/texture"
	"aego/render/view"
	"aego/vfs"
)

const (
	spriteStride  = sprite.InstanceStride
	uniformSize   = 64
	defaultSprite = 8192
	defaultDebug  = 8192
	defaultBudget = 4 << 20
)

type Options struct {
	MaxSprites    int
	MaxDebugVerts int
	UploadBudget  int
}

func (o Options) withDefaults() Options {
	if o.MaxSprites <= 0 {
		o.MaxSprites = defaultSprite
	}
	if o.MaxDebugVerts <= 0 {
		o.MaxDebugVerts = defaultDebug
	}
	if o.UploadBudget <= 0 {
		o.UploadBudget = defaultBudget
	}
	return o
}

type Stats struct {
	Submitted     uint32
	Visible       uint32
	Culled        uint32
	Dropped       uint32
	ZClamped      uint32
	Batches       uint32
	DrawCalls     uint32
	Instances     uint32
	TexSwitches   uint32
	DebugVerts    uint32
	InstanceBytes uint32
	CPURenderNS   uint64
	GPUNS         uint64
	GPUValid      bool
	Views         uint32
}

type Renderer struct {
	dev          gpu.Device
	caps         gpu.Capabilities
	log          *log.ChannelLogger
	files        vfs.FS
	opt          Options
	up           *texture.Uploader
	samplers     map[gpu.SamplerDesc]gpu.SamplerHandle
	batcher      *sprite.Batcher
	dbg          *debug.Draw
	font         *text.BitmapFont
	white        texture.Region
	spriteShader gpu.ShaderHandle
	debugShader  gpu.ShaderHandle
	spritePipes  [3]gpu.PipelineHandle
	debugPipe    gpu.PipelineHandle
	ub           gpu.BufferHandle
	ubBuf        []byte
	prof         *gpuprof.Profiler
	passes       []PassDesc
	order        []PassDesc
	frozen       bool
	frame        gpu.Frame
	fbW, fbH     int
	fbScale      float32
	vf           ViewFrame
	stats        Stats
	textBuf      []sprite.Draw
	passCtx      PassContext
	defSampler   gpu.SamplerHandle
	warned       bool
	uiFont       *text.Font
}

func New(dev gpu.Device, caps gpu.Capabilities, files vfs.FS, lg *log.Logger, opt Options) (*Renderer, error) {
	if lg == nil {
		lg = log.Nop()
	}
	opt = opt.withDefaults()
	r := &Renderer{
		dev:      dev,
		caps:     caps,
		log:      lg.With(log.Render),
		files:    files,
		opt:      opt,
		up:       texture.NewUploader(dev),
		samplers: make(map[gpu.SamplerDesc]gpu.SamplerHandle, 8),
		ubBuf:    make([]byte, uniformSize),
		textBuf:  make([]sprite.Draw, 0, 256),
	}
	if err := r.initPipelines(); err != nil {
		r.Close()
		return nil, err
	}
	b, err := sprite.NewBatcher(dev, opt.MaxSprites)
	if err != nil {
		r.Close()
		return nil, err
	}
	r.batcher = b
	d, err := debug.New(dev, opt.MaxDebugVerts)
	if err != nil {
		r.Close()
		return nil, err
	}
	r.dbg = d
	r.defSampler = r.Sampler(gpu.SamplerDesc{})
	whiteTex, err := r.up.Upload(texture.Solid(2, 2, mathx.Color{R: 1, G: 1, B: 1, A: 1}), texture.UploadOptions{Label: "render.white"})
	if err != nil {
		r.Close()
		return nil, err
	}
	r.white = texture.Whole(whiteTex, 2, 2).Sub(0, 0, 1, 1)
	if f, err := text.LoadBitmap(files, r.up, "engine://fonts/debug.png", "engine://fonts/debug.json"); err != nil {
		r.log.Warn("debug font is unavailable, text rendering disabled", log.F("err", err.Error()))
	} else {
		r.font = f
	}
	r.prof = gpuprof.New(dev, caps)
	r.passes = []PassDesc{
		{ID: "sprites", Pass: spritePass{}},
		{ID: "debug", After: []string{"sprites"}, Pass: debugPass{}},
	}
	r.vf.r = r
	return r, nil
}

func (r *Renderer) initPipelines() error {
	vsrc, err := r.files.ReadFile("engine://shaders/sprite.vert")
	if err != nil {
		return err
	}
	fsrc, err := r.files.ReadFile("engine://shaders/sprite.frag")
	if err != nil {
		return err
	}
	r.spriteShader, err = r.dev.CreateShader(gpu.ShaderDesc{
		Label: "sprite",
		Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: string(vsrc)},
			{Stage: gpu.StageFragment, Source: string(fsrc)},
		},
	})
	if err != nil {
		return err
	}
	for m := sprite.BlendMode(0); m < 3; m++ {
		p, err := r.dev.CreatePipeline(gpu.PipelineDesc{
			Shader:    r.spriteShader,
			Layout:    sprite.Layout(),
			Blend:     sprite.BlendState(m),
			Primitive: gpu.PrimTriangleStrip,
			Label:     "sprite",
		})
		if err != nil {
			return err
		}
		r.spritePipes[m] = p
	}
	dvsrc, err := r.files.ReadFile("engine://shaders/debug.vert")
	if err != nil {
		return err
	}
	dfsrc, err := r.files.ReadFile("engine://shaders/debug.frag")
	if err != nil {
		return err
	}
	r.debugShader, err = r.dev.CreateShader(gpu.ShaderDesc{
		Label: "debug",
		Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: string(dvsrc)},
			{Stage: gpu.StageFragment, Source: string(dfsrc)},
		},
	})
	if err != nil {
		return err
	}
	r.debugPipe, err = r.dev.CreatePipeline(gpu.PipelineDesc{
		Shader:    r.debugShader,
		Layout:    debug.Layout(),
		Blend:     gpu.BlendPremultiplied(),
		Primitive: gpu.PrimLines,
		Label:     "debug",
	})
	if err != nil {
		return err
	}
	r.ub, err = r.dev.CreateBuffer(gpu.BufferDesc{
		Usage:   gpu.BufferUniform,
		Size:    uniformSize,
		Dynamic: true,
		Label:   "render.viewproj",
	})
	return err
}

func (r *Renderer) UIFont() *text.Font {
	if r.uiFont == nil && r.font != nil {
		r.uiFont = r.font.Font()
	}
	return r.uiFont
}

func (r *Renderer) setUniform(p gpu.Pass, m mathx.Mat4) {
	for i, v := range m {
		binary.LittleEndian.PutUint32(r.ubBuf[i*4:i*4+4], math.Float32bits(v))
	}
	r.dev.UpdateBuffer(r.ub, 0, r.ubBuf)
	p.SetUniformBuffer(0, r.ub)
}

func (r *Renderer) Textures() *texture.Uploader {
	return r.up
}

func (r *Renderer) White() texture.Region {
	if r.font != nil && !r.font.White.IsZero() {
		return r.font.White
	}
	return r.white
}

func (r *Renderer) DebugFont() *text.BitmapFont {
	return r.font
}

func (r *Renderer) Caps() gpu.Capabilities {
	return r.caps
}

func (r *Renderer) Sampler(d gpu.SamplerDesc) gpu.SamplerHandle {
	if h, ok := r.samplers[d]; ok {
		return h
	}
	h, err := r.dev.CreateSampler(d)
	if err != nil {
		r.log.Error("sampler creation failed", log.F("err", err.Error()))
		return gpu.SamplerHandle{}
	}
	r.samplers[d] = h
	return h
}

func (r *Renderer) AddPass(d PassDesc) error {
	if r.frozen {
		return errs.New(errs.EngineFrozen, "render passes are frozen after the first frame", log.F("pass", d.ID))
	}
	r.passes = append(r.passes, d)
	return nil
}

func (r *Renderer) BeginFrame(f gpu.Frame, pixelW, pixelH int, pixelScale float32) error {
	if !r.frozen {
		order, err := sortPasses(r.passes)
		if err != nil {
			return err
		}
		r.order = order
		r.frozen = true
	}
	r.frame = f
	r.fbW, r.fbH = pixelW, pixelH
	r.fbScale = pixelScale
	r.stats = Stats{}
	r.up.Drain(r.opt.UploadBudget)
	if f != nil {
		r.prof.Collect(f)
	}
	return nil
}

func (r *Renderer) framebuffer() (int, int) {
	return r.fbW, r.fbH
}

func (r *Renderer) View(v view.View) *ViewFrame {
	f := &r.vf
	f.reset(v)
	r.stats.Views++
	if r.frame == nil {
		return f
	}
	f.pass = r.frame.BeginPass(gpu.PassDesc{
		Target:     v.Target,
		ClearColor: v.Clear,
		Color:      clearAction(v.DoClear),
		Label:      v.Label,
	})
	f.pass.SetViewport(f.view.LetterboxViewport())
	if r.stats.Views == 1 {
		r.prof.Begin(f.pass)
	}
	return f
}

func clearAction(do bool) gpu.LoadAction {
	if do {
		return gpu.LoadClear
	}
	return gpu.LoadKeep
}

func (r *Renderer) EndFrame() Stats {
	if r.frame != nil {
		fs := r.frame.Stats()
		r.stats.DrawCalls = fs.DrawCalls
		r.stats.TexSwitches = fs.TexSwitches
	}
	if ns, ok := r.prof.LastNS(); ok {
		r.stats.GPUNS = ns
		r.stats.GPUValid = true
	}
	r.frame = nil
	return r.stats
}

func (r *Renderer) Close() {
	if r.prof != nil {
		r.prof.Close()
	}
	if r.dbg != nil {
		r.dbg.Close()
	}
	if r.batcher != nil {
		r.batcher.Close()
	}
	if !r.ub.IsZero() {
		r.dev.DestroyBuffer(r.ub)
	}
	for _, p := range r.spritePipes {
		if !p.IsZero() {
			r.dev.DestroyPipeline(p)
		}
	}
	if !r.debugPipe.IsZero() {
		r.dev.DestroyPipeline(r.debugPipe)
	}
	if !r.spriteShader.IsZero() {
		r.dev.DestroyShader(r.spriteShader)
	}
	if !r.debugShader.IsZero() {
		r.dev.DestroyShader(r.debugShader)
	}
}
