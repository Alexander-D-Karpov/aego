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
	"aego/render/mesh"
	"aego/render/sprite"
	"aego/render/text"
	"aego/render/texture"
	"aego/render/view"
	"aego/vfs"
)

const (
	spriteStride    = sprite.InstanceStride
	sprite3DStride  = sprite.Instance3DStride
	uniformSize     = 64
	drawUniformSize = 96
	defaultSprite   = 8192
	defaultSprite3D = 4096
	defaultMeshes   = 4096
	defaultDebug    = 8192
	defaultBudget   = 4 << 20
	blendModes      = 3
)

type Options struct {
	MaxSprites    int
	MaxSprites3D  int
	MaxMeshes     int
	MaxDebugVerts int
	UploadBudget  int
}

func (o Options) withDefaults() Options {
	if o.MaxSprites <= 0 {
		o.MaxSprites = defaultSprite
	}
	if o.MaxSprites3D <= 0 {
		o.MaxSprites3D = defaultSprite3D
	}
	if o.MaxMeshes <= 0 {
		o.MaxMeshes = defaultMeshes
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
	Submitted      uint32
	Visible        uint32
	Culled         uint32
	Dropped        uint32
	ZClamped       uint32
	Batches        uint32
	DrawCalls      uint32
	Instances      uint32
	TexSwitches    uint32
	DebugVerts     uint32
	InstanceBytes  uint32
	MeshDraws      uint32
	MeshCulled     uint32
	Sprites3D      uint32
	Sprite3DCulled uint32
	CPURenderNS    uint64
	GPUNS          uint64
	GPUValid       bool
	Views          uint32
}

type Renderer struct {
	dev            gpu.Device
	caps           gpu.Capabilities
	log            *log.ChannelLogger
	files          vfs.FS
	opt            Options
	up             *texture.Uploader
	samplers       map[gpu.SamplerDesc]gpu.SamplerHandle
	batcher        *sprite.Batcher
	sprites3d      *sprite.Batcher3D
	meshes         *mesh.Batcher
	dbg            *debug.Draw
	font           *text.BitmapFont
	white          texture.Region
	spriteShader   gpu.ShaderHandle
	sprite3dShader gpu.ShaderHandle
	meshShader     gpu.ShaderHandle
	debugShader    gpu.ShaderHandle
	spritePipes    [blendModes]gpu.PipelineHandle
	sprite3dPipes  [blendModes]gpu.PipelineHandle
	meshPipes      [blendModes][2]gpu.PipelineHandle
	debugPipe      gpu.PipelineHandle
	debugDepthPipe gpu.PipelineHandle
	ub             gpu.BufferHandle
	ubDraw         gpu.BufferHandle
	ubBuf          []byte
	drawBuf        []byte
	prof           *gpuprof.Profiler
	passes         []PassDesc
	order          []PassDesc
	frozen         bool
	frame          gpu.Frame
	fbW, fbH       int
	fbScale        float32
	vf             ViewFrame
	stats          Stats
	textBuf        []sprite.Draw
	passCtx        PassContext
	defSampler     gpu.SamplerHandle
	warned         bool
	uiFont         *text.Font
	depthSeen      []gpu.TextureHandle
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
		drawBuf:  make([]byte, drawUniformSize),
		textBuf:  make([]sprite.Draw, 0, 256),
		meshes:   mesh.NewBatcher(opt.MaxMeshes),
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
	b3, err := sprite.NewBatcher3D(dev, opt.MaxSprites3D)
	if err != nil {
		r.Close()
		return nil, err
	}
	r.sprites3d = b3
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
		{ID: "mesh_opaque", Pass: meshOpaquePass{}},
		{ID: "mesh_blend", After: []string{"mesh_opaque"}, Pass: meshBlendPass{}},
		{ID: "sprites3d", After: []string{"mesh_blend"}, Pass: sprite3DPass{}},
		{ID: "sprites", After: []string{"sprites3d"}, Pass: spritePass{}},
		{ID: "debug", After: []string{"sprites"}, Pass: debugPass{}},
	}
	r.vf.r = r
	return r, nil
}

func (r *Renderer) loadShader(label, vert, frag string) (gpu.ShaderHandle, error) {
	vsrc, err := r.files.ReadFile("engine://shaders/" + vert)
	if err != nil {
		return gpu.ShaderHandle{}, err
	}
	fsrc, err := r.files.ReadFile("engine://shaders/" + frag)
	if err != nil {
		return gpu.ShaderHandle{}, err
	}
	return r.dev.CreateShader(gpu.ShaderDesc{
		Label: label,
		Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: string(vsrc)},
			{Stage: gpu.StageFragment, Source: string(fsrc)},
		},
	})
}

func (r *Renderer) initPipelines() error {
	var err error
	if r.spriteShader, err = r.loadShader("sprite", "sprite.vert", "sprite.frag"); err != nil {
		return err
	}
	if r.sprite3dShader, err = r.loadShader("sprite3d", "sprite3d.vert", "sprite.frag"); err != nil {
		return err
	}
	if r.meshShader, err = r.loadShader("mesh", "mesh.vert", "mesh.frag"); err != nil {
		return err
	}
	if r.debugShader, err = r.loadShader("debug", "debug.vert", "debug.frag"); err != nil {
		return err
	}
	for m := sprite.BlendMode(0); m < blendModes; m++ {
		if r.spritePipes[m], err = r.dev.CreatePipeline(gpu.PipelineDesc{
			Shader:    r.spriteShader,
			Layout:    sprite.Layout(),
			Blend:     sprite.BlendState(m),
			Primitive: gpu.PrimTriangleStrip,
			Label:     "sprite",
		}); err != nil {
			return err
		}
		if r.sprite3dPipes[m], err = r.dev.CreatePipeline(gpu.PipelineDesc{
			Shader:    r.sprite3dShader,
			Layout:    sprite.Layout3D(),
			Blend:     sprite.BlendState(m),
			Depth:     gpu.DepthState{Test: true, Compare: gpu.CompareLessEqual},
			Primitive: gpu.PrimTriangleStrip,
			Label:     "sprite3d",
		}); err != nil {
			return err
		}
		for ds := 0; ds < 2; ds++ {
			cull := gpu.CullBack
			if ds == 1 {
				cull = gpu.CullNone
			}
			if r.meshPipes[m][ds], err = r.dev.CreatePipeline(gpu.PipelineDesc{
				Shader:    r.meshShader,
				Layout:    mesh.Layout(),
				Blend:     sprite.BlendState(m),
				Depth:     gpu.DepthState{Test: true, Write: m == sprite.BlendOpaque, Compare: gpu.CompareLess},
				Cull:      cull,
				Primitive: gpu.PrimTriangles,
				Label:     "mesh",
			}); err != nil {
				return err
			}
		}
	}
	if r.debugPipe, err = r.dev.CreatePipeline(gpu.PipelineDesc{
		Shader:    r.debugShader,
		Layout:    debug.Layout(),
		Blend:     gpu.BlendPremultiplied(),
		Primitive: gpu.PrimLines,
		Label:     "debug",
	}); err != nil {
		return err
	}
	if r.debugDepthPipe, err = r.dev.CreatePipeline(gpu.PipelineDesc{
		Shader:    r.debugShader,
		Layout:    debug.Layout(),
		Blend:     gpu.BlendPremultiplied(),
		Depth:     gpu.DepthState{Test: true, Compare: gpu.CompareLessEqual},
		Primitive: gpu.PrimLines,
		Label:     "debug.depth",
	}); err != nil {
		return err
	}
	if r.ub, err = r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: uniformSize, Dynamic: true, Label: "render.viewproj"}); err != nil {
		return err
	}
	r.ubDraw, err = r.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: drawUniformSize, Dynamic: true, Label: "render.draw"})
	return err
}

func (r *Renderer) UIFont() *text.Font {
	if r.uiFont == nil && r.font != nil {
		r.uiFont = r.font.Font()
	}
	return r.uiFont
}

func putF32(b []byte, v float32) {
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
}

func (r *Renderer) setUniform(p gpu.Pass, m mathx.Mat4) {
	for i, v := range m {
		putF32(r.ubBuf[i*4:], v)
	}
	r.dev.UpdateBuffer(r.ub, 0, r.ubBuf)
	p.SetUniformBuffer(0, r.ub)
}

func (r *Renderer) Textures() *texture.Uploader {
	return r.up
}

func (r *Renderer) Device() gpu.Device {
	return r.dev
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
	r.depthSeen = r.depthSeen[:0]
	r.up.Drain(r.opt.UploadBudget)
	if f != nil {
		r.prof.Collect(f)
	}
	return nil
}

func (r *Renderer) firstDepthUse(t gpu.TextureHandle) bool {
	for _, s := range r.depthSeen {
		if s == t {
			return false
		}
	}
	r.depthSeen = append(r.depthSeen, t)
	return true
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
	fv := &f.view
	depth := gpu.LoadKeep
	if fv.UsesDepth() {
		target := fv.Target
		if !fv.DepthTarget.IsZero() {
			target = fv.DepthTarget
		}
		if r.firstDepthUse(target) || fv.DoClear {
			depth = gpu.LoadClear
		}
	}
	f.pass = r.frame.BeginPass(gpu.PassDesc{
		Target:      fv.Target,
		DepthTarget: fv.DepthTarget,
		ClearColor:  fv.Clear,
		ClearDepth:  1,
		Color:       clearAction(fv.DoClear),
		Depth:       depth,
		FlipWinding: fv.Flipped(r.caps),
		Label:       fv.Label,
	})
	f.pass.SetViewport(fv.LetterboxViewport())
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

func (r *Renderer) destroyPipe(p gpu.PipelineHandle) {
	if !p.IsZero() {
		r.dev.DestroyPipeline(p)
	}
}

func (r *Renderer) destroyShader(s gpu.ShaderHandle) {
	if !s.IsZero() {
		r.dev.DestroyShader(s)
	}
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
	if r.sprites3d != nil {
		r.sprites3d.Close()
	}
	if !r.ub.IsZero() {
		r.dev.DestroyBuffer(r.ub)
	}
	if !r.ubDraw.IsZero() {
		r.dev.DestroyBuffer(r.ubDraw)
	}
	for m := range r.spritePipes {
		r.destroyPipe(r.spritePipes[m])
		r.destroyPipe(r.sprite3dPipes[m])
		r.destroyPipe(r.meshPipes[m][0])
		r.destroyPipe(r.meshPipes[m][1])
	}
	r.destroyPipe(r.debugPipe)
	r.destroyPipe(r.debugDepthPipe)
	r.destroyShader(r.spriteShader)
	r.destroyShader(r.sprite3dShader)
	r.destroyShader(r.meshShader)
	r.destroyShader(r.debugShader)
}
