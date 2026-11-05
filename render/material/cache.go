package material

import (
	"fmt"
	"slices"

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
	trim    bool
	aux     bool
}

type reload struct {
	old, next   *Shader
	mats        []*Material
	commit      func()
	token       uint64
	device      uint64
	jobs        map[int]gpu.ShaderCompilation
	definitions map[*Material]*Material
	only        bool
	state       reloadState
	dependency  *reload
}

type reloadState uint8

const (
	reloadPending reloadState = iota
	reloadCommitted
	reloadRejected
	reloadCanceled
)

// Stats counts material-owned resources and traffic, without counting editor or
// driver allocations. Traffic and failures are cumulative for delta sampling.
type Stats struct {
	Materials, Programs, Pipelines, Pending                             int
	Uploads, UploadBytes, LaneBytes, CompileFailures, ReloadGenerations uint64
}

type Cache struct {
	spec           map[specKey]*Shader
	lightFirstUse  map[*Shader]uint64
	dev            gpu.Device
	laneBuffer     gpu.BufferHandle
	laneBytes      []byte
	domainFailed   map[domainFailure]bool
	es             bool
	tmpl           Template
	white          Tex
	progs          map[progKey]gpu.ShaderHandle
	pipes          map[pipeKey]gpu.PipelineHandle
	failed         map[progKey]bool
	used           map[*Shader]uint64
	mats           map[*Material]struct{}
	retired        []*Shader
	reloads        map[*Shader]*reload
	isolated       map[*Material]*reload
	materialChecks map[string]func(*Material, *Shader) error
	checks         map[string]func(old, next *Shader) error
	frame          uint64
	generation     uint64
	serial         uint64
	stats          Stats
	OnError        func(sh *Shader, variant int, err error)
	OnReload       func(*Shader)
}

func NewCache(dev gpu.Device, es bool, tmpl Template) *Cache {
	return &Cache{dev: dev, es: es, tmpl: tmpl, laneBytes: make([]byte, shader.LaneStride), domainFailed: map[domainFailure]bool{},
		progs: map[progKey]gpu.ShaderHandle{}, pipes: map[pipeKey]gpu.PipelineHandle{},
		failed: map[progKey]bool{}, used: map[*Shader]uint64{}, mats: map[*Material]struct{}{},
		reloads: map[*Shader]*reload{}, isolated: map[*Material]*reload{}, materialChecks: map[string]func(*Material, *Shader) error{}, checks: map[string]func(old, next *Shader) error{}}
}

func (c *Cache) SetWhite(t Tex) { c.white = t }
func (c *Cache) Track(m *Material) {
	if m != nil {
		if _, tracked := c.mats[m]; !tracked {
			c.mats[m] = struct{}{}
		}
	}
}

type restorer interface{ Restores() int }

func (c *Cache) deviceGeneration() uint64 {
	if d, ok := c.dev.(gpu.ContextState); ok {
		return d.ContextGeneration()
	}
	if d, ok := c.dev.(restorer); ok {
		return uint64(d.Restores())
	}
	return 0
}
func (c *Cache) lost() bool {
	if d, ok := c.dev.(gpu.ContextState); ok {
		return d.ContextLost()
	}
	return false
}

func (c *Cache) Begin() {
	c.frame++
	gen := c.deviceGeneration()
	if gen != c.generation {
		c.generation = gen
		clear(c.failed)
		for m := range c.mats {
			m.dirty = true
		}
		for _, r := range c.reloads {
			c.cancelJobs(r)
			if r.next != r.old {
				c.dropUnreferenced(r.next)
			}
			r.device = gen
		}
		for _, r := range c.isolated {
			c.cancelJobs(r)
			if r.next != r.old {
				c.dropUnreferenced(r.next)
			}
			r.device = gen
		}

	}
}

func (c *Cache) Ready(m *Material) bool {
	return m != nil && !c.lost() && !c.failed[progKey{m.shader, m.variant}]
}
func (c *Cache) fail(sh *Shader, v int, err error) {
	if c.lost() {
		return
	}
	c.failed[progKey{sh, v}] = true
	if sh.lightParent != nil {
		return
	}
	c.stats.CompileFailures++
	if c.OnError != nil {
		c.OnError(sh, v, err)
	}
}

func (c *Cache) descriptor(sh *Shader, v int) gpu.ShaderDesc {
	src := &sh.Variants[v].Sources
	return gpu.ShaderDesc{Label: sh.Name, Sources: []gpu.ShaderSource{
		{Stage: gpu.StageVertex, Source: src[SourceIndex(false, c.es)]},
		{Stage: gpu.StageFragment, Source: src[SourceIndex(true, c.es)]},
	}}
}

// PrepareSpriteProgram loads and validates the material's current shader variant
// into the same cache used by Bind. It creates no pipeline, UBO or draw call.
func (c *Cache) PrepareSpriteProgram(m *Material) {
	if m == nil || m.shader == nil || m.shader.Domain != shader.DomainSprite || c.lost() {
		return
	}
	c.program(m.shader, m.variant)
}

func (c *Cache) program(sh *Shader, v int) (gpu.ShaderHandle, bool) {
	k := progKey{sh, v}
	if h, ok := c.progs[k]; ok {
		return h, true
	}
	if c.failed[k] || c.lost() {
		return gpu.ShaderHandle{}, false
	}
	h, err := c.dev.CreateShader(c.descriptor(sh, v))
	if err != nil {
		c.fail(sh, v, err)
		return gpu.ShaderHandle{}, false
	}
	if err := c.validateLayout(sh, h); err != nil {
		c.dev.DestroyShader(h)
		c.fail(sh, v, err)
		return gpu.ShaderHandle{}, false
	}
	c.progs[k] = h
	return h, true
}

func (c *Cache) pipelineLayout(sh *Shader, v int, b shader.Blend, trim, aux bool) (gpu.PipelineHandle, bool) {
	k := pipeKey{sh, v, b, trim, aux}
	if h, ok := c.pipes[k]; ok {
		return h, true
	}
	caps := c.dev.Caps()
	if sh.Features&shader.FeatureLighting != 0 && caps.MaxUniformBlockSize > 0 && caps.MaxUniformBlockSize < shader.LightingSize {
		c.fail(sh, v, fmt.Errorf("SHD004: forward2d light block exceeds device uniform block size"))
		return gpu.PipelineHandle{}, false
	}
	units := caps.MaxFragmentTextureUnits
	if units == 0 {
		units = caps.MaxTextureUnits
	}
	lastLightingTexture := shader.TexCookie
	if sh.Features&shader.FeatureSurfaceChannels != 0 {
		lastLightingTexture = shader.TexEmission
	}
	if sh.Features&shader.FeatureLighting != 0 && (units > 0 && units <= lastLightingTexture || !caps.TextureArrays) {
		c.fail(sh, v, fmt.Errorf("SHD004: forward2d needs texture arrays and %d fragment texture units (including six material textures)", lastLightingTexture+1))
		return gpu.PipelineHandle{}, false
	}
	if aux && sh.Features&shader.FeatureAffine == 0 {
		c.fail(sh, v, fmt.Errorf("SHD009: this legacy prepared shader lacks the affine stream; reimport its source"))
		return gpu.PipelineHandle{}, false
	}
	if sh.Schema.Lanes > shader.MaxLanes || sh.Schema.Lanes < 0 || (caps.MaxUniformBlockSize > 0 && sh.Schema.Size > caps.MaxUniformBlockSize) {
		c.fail(sh, v, fmt.Errorf("SHD004: material layout exceeds device resources"))
		return gpu.PipelineHandle{}, false
	}
	d := c.tmpl(sh.Domain, sh.Schema.Lanes, b)
	if aux && sh.Domain == shader.DomainSprite {
		d.Layout.Attrs = append(d.Layout.Attrs, gpu.VertexAttr{Location: shader.AffineLocation, Slot: shader.AuxSlot, Format: gpu.VFFloat4})
		if sh.Features&shader.FeatureLighting != 0 {
			d.Layout.Attrs = append(d.Layout.Attrs,
				gpu.VertexAttr{Location: shader.NormalUVLocation, Slot: shader.AuxSlot, Format: gpu.VFFloat4, Offset: 16},
				gpu.VertexAttr{Location: shader.NormalLocalLocation, Slot: shader.AuxSlot, Format: gpu.VFFloat4, Offset: 32})
		}
		if sh.Features&shader.FeatureSurfaceChannels != 0 {
			d.Layout.Attrs = append(d.Layout.Attrs,
				gpu.VertexAttr{Location: shader.EmissionUVLocation, Slot: shader.AuxSlot, Format: gpu.VFFloat4, Offset: 48},
				gpu.VertexAttr{Location: shader.EmissionLocalLocation, Slot: shader.AuxSlot, Format: gpu.VFFloat4, Offset: 64})
		}
		d.Layout.Strides[shader.AuxSlot], d.Layout.PerInstance[shader.AuxSlot] = shader.AuxStride, true
	}
	if trim && sh.Domain == shader.DomainSprite {
		d.Layout.Attrs = append(d.Layout.Attrs, gpu.VertexAttr{Location: shader.TrimLocation, Slot: shader.TrimSlot, Format: gpu.VFFloat4})
		d.Layout.Strides[shader.TrimSlot] = shader.TrimStride
		d.Layout.PerInstance[shader.TrimSlot] = true
	}
	if sh.Domain == shader.DomainSprite && caps.MaxVertexAttributes > 0 && caps.MaxVertexAttributes <= shader.TrimLocation {
		c.fail(sh, v, fmt.Errorf("SHD004: sprite ABI trim attribute exceeds device capabilities"))
		return gpu.PipelineHandle{}, false
	}
	if err := gpu.ValidateVertexLayout(d.Layout, caps); err != nil {
		c.fail(sh, v, err)
		return gpu.PipelineHandle{}, false
	}
	prog, ok := c.program(sh, v)
	if !ok {
		return gpu.PipelineHandle{}, false
	}
	d.Shader, d.Label = prog, sh.Name
	h, err := c.dev.CreatePipeline(d)
	if err != nil {
		c.fail(sh, v, err)
		return gpu.PipelineHandle{}, false
	}
	c.pipes[k] = h
	return h, true
}

func (c *Cache) upload(m *Material) bool {
	if len(m.data) == 0 || !m.dirty {
		return true
	}
	if m.ubSize != len(m.data) {
		h, err := c.dev.CreateBuffer(gpu.BufferDesc{Usage: gpu.BufferUniform, Size: len(m.data), Dynamic: true, Label: "material." + m.Name})
		if err != nil {
			return false
		}
		if !m.ub.IsZero() {
			c.dev.DestroyBuffer(m.ub)
		}
		m.ub, m.ubSize = h, len(m.data)
	}
	c.dev.UpdateBuffer(m.ub, 0, m.data)
	c.stats.Uploads++
	c.stats.UploadBytes += uint64(len(m.data))
	m.dirty = false
	return true
}

func (c *Cache) Bind(p gpu.Pass, m *Material) bool { return c.bind(p, m, false, false) }

func (c *Cache) bind(p gpu.Pass, m *Material, trim, aux bool) bool {
	if m == nil {
		return false
	}
	return c.bindBlend(p, m, trim, aux, m.blend)
}
func (c *Cache) bindBlend(p gpu.Pass, m *Material, trim, aux bool, blend shader.Blend) bool {
	if c.lost() {
		return false
	}
	c.Track(m)
	sh := m.shader
	pipe, ok := c.pipelineLayout(sh, m.variant, blend, trim, aux)
	if !ok || !c.upload(m) {
		return false
	}
	if c.used[sh] != c.frame {
		c.used[sh] = c.frame
	}
	p.SetPipeline(pipe)
	c.BindPostParameters(p, m)

	return true
}

// QueueReload replaces a last-good shader's pending request; callbacks run at End after all views, when dependencies are ready, for a coherent next-frame schema.
func (c *Cache) QueueReload(old, next *Shader, mats []*Material, commit func()) uint64 {
	c.CancelReload(old)
	c.serial++
	if old == nil || next == nil {
		return c.serial
	}
	r := &reload{old: old, next: next, mats: slices.Clone(mats), commit: commit,
		token: c.serial, device: c.deviceGeneration(), jobs: map[int]gpu.ShaderCompilation{}}
	for _, m := range mats {
		c.Track(m)
		m.next = nil
	}
	c.reloads[old] = r
	return r.token
}

func (c *Cache) cancelJobs(r *reload) {
	for v, job := range r.jobs {
		job.Cancel()
		delete(r.jobs, v)
	}
}
func (c *Cache) CancelReload(old *Shader) {
	if r, ok := c.reloads[old]; ok {
		if r.state == reloadPending {
			r.state = reloadCanceled
		}
		c.cancelJobs(r)
		if r.next != r.old {
			c.dropUnreferenced(r.next)
		}
		delete(c.reloads, old)
	}
}

func (c *Cache) candidateProgram(r *reload, v int) (ready, good bool) {
	key := progKey{r.next, v}
	if _, ok := c.progs[key]; ok {
		return true, true
	}
	if c.failed[key] {
		return true, false
	}
	async, supported := c.dev.(gpu.AsyncShaderCompiler)
	if !supported {
		_, ok := c.program(r.next, v)
		return true, ok
	}
	job := r.jobs[v]
	if job == nil {
		var err error
		job, err = async.BeginShaderCompile(c.descriptor(r.next, v))
		if err != nil {
			c.fail(r.next, v, err)
			return true, false
		}
		r.jobs[v] = job
	}
	h, done, err := job.Poll()
	if !done {
		return false, true
	}
	delete(r.jobs, v)
	if err != nil {
		c.fail(r.next, v, err)
		return true, false
	}
	if err := c.validateLayout(r.next, h); err != nil {
		c.dev.DestroyShader(h)
		c.fail(r.next, v, err)
		return true, false
	}
	c.progs[key] = h
	return true, true
}

func (c *Cache) finishReload(r *reload) bool {
	if c.lost() {
		return false
	}
	if r.device != c.deviceGeneration() {
		c.cancelJobs(r)
		if r.next != r.old {
			c.dropUnreferenced(r.next)
		}
		r.device = c.deviceGeneration()
		return false
	}
	// Materials created after import joined the same old shader, so include them.
	for m := range c.mats {
		if !r.only && m.shader == r.old && !slices.Contains(r.mats, m) {
			r.mats = append(r.mats, m)
		}
	}
	type replacement struct{ old, next *Material }
	var prepared []replacement
	cleanup := func() {
		for _, p := range prepared {
			if !p.next.ub.IsZero() {
				c.dev.DestroyBuffer(p.next.ub)
			}
		}
	}
	fail := func(err error) bool {
		r.state = reloadRejected
		cleanup()
		for _, m := range r.mats {
			m.failed = true
		}
		if err != nil {
			c.fail(r.next, 0, err)
		}
		c.cancelJobs(r)
		if r.next != r.old {
			c.dropUnreferenced(r.next)
		}
		return true
	}
	for _, m := range r.mats {
		if _, live := c.mats[m]; !live || m.shader != r.old {
			continue
		}
		source := m
		if def := r.definitions[m]; def != nil {
			source = def
		}
		n, err := source.prepare(r.next)
		if err != nil {
			return fail(err)
		}
		ready, good := c.candidateProgram(r, n.variant)
		if !good {
			return fail(nil)
		}
		if !ready {
			cleanup()
			return false
		}
		// Prepare the material's used layouts before publishing: affine/normal and trim streams are pipeline state, not just program state, and layout failure must retain last-good too.
		usedLayout := false
		for key := range c.pipes {
			if key.sh != r.old || key.variant != m.variant || key.blend != m.blend {
				continue
			}
			aux := key.aux || r.next.Domain == shader.DomainSprite && r.next.Features&shader.FeatureLighting != 0
			if _, ok := c.pipelineLayout(r.next, n.variant, n.blend, key.trim, aux); !ok {
				return fail(nil)
			}
			usedLayout = true
		}
		if !usedLayout {
			aux := r.next.Domain == shader.DomainSprite && r.next.Features&shader.FeatureLighting != 0
			if _, ok := c.pipelineLayout(r.next, n.variant, n.blend, false, aux); !ok {
				return fail(nil)
			}
		}
		prepared = append(prepared, replacement{m, n})
		if !c.upload(n) {
			return fail(fmt.Errorf("MAT005: cannot allocate candidate material buffer"))
		}
	}
	if len(r.mats) == 0 {
		if len(r.next.Variants) == 0 {
			return fail(fmt.Errorf("SHD006: shader has no variants"))
		}
		ready, good := c.candidateProgram(r, 0)
		if !good {
			return fail(nil)
		}
		if !ready {
			return false
		}
	}
	if r.only {
		for _, m := range r.mats {
			for _, check := range c.materialChecks {
				if err := check(m, r.next); err != nil {
					return fail(err)
				}
			}
		}
	} else {
		for _, check := range c.checks {
			if err := check(r.old, r.next); err != nil {
				return fail(err)
			}
		}
	}
	// No callback or draw may observe a half-committed group.
	if r.device != c.deviceGeneration() || c.lost() {
		cleanup()
		return false
	}
	for _, p := range prepared {
		oldUB := p.old.ub
		p.old.commit(p.next)
		p.old.ub, p.old.ubSize, p.old.dirty = p.next.ub, p.next.ubSize, false
		if !oldUB.IsZero() {
			c.dev.DestroyBuffer(oldUB)
		}
	}
	r.state = reloadCommitted
	if r.commit != nil {
		r.commit()
	}
	c.stats.ReloadGenerations++
	if c.OnReload != nil {
		c.OnReload(r.next)
	}
	if r.old != r.next {
		c.Retire(r.old)
	}
	return true
}

func (c *Cache) Release(m *Material) {
	if m == nil {
		return
	}
	if !m.ub.IsZero() {
		c.dev.DestroyBuffer(m.ub)
	}
	m.ub, m.ubSize, m.dirty = gpu.BufferHandle{}, 0, true
	delete(c.mats, m)
	c.CancelMaterial(m)
	for old, r := range c.reloads {
		r.mats = slices.DeleteFunc(r.mats, func(v *Material) bool { return v == m })
		delete(r.definitions, m)
		if r.only && len(r.mats) == 0 {
			c.CancelReload(old)
		}
	}
}

func (c *Cache) Retire(sh *Shader) {
	if sh != nil && !slices.Contains(c.retired, sh) {
		c.retired = append(c.retired, sh)
	}
}

func (c *Cache) End() {
	// Compatibility with callers using Material.Reload: collect the request into
	// the same transactional machinery used by asset.Loader.
	for m := range c.mats {
		if m.next != nil {
			next := m.next
			m.next = nil
			if r := c.reloads[m.shader]; r == nil || r.next != next {
				c.QueueReload(m.shader, next, nil, nil)
			}
		}
	}
	for old, r := range c.reloads {
		if c.finishReload(r) && c.reloads[old] == r {
			delete(c.reloads, old)
		}
	}
	for m, r := range c.isolated {
		// A material switching to a shader candidate shares its outcome, so a failed group cannot become visible through another asset.
		// The pointer retains the terminal state after removal from reloads, including cancellation by a newer source revision.
		if dep := r.dependency; dep != nil && dep.state != reloadCommitted {
			if dep.state == reloadPending {
				continue
			}
			m.failed = true
			c.CancelMaterial(m)
			continue
		}
		if m.shader != r.old {
			c.cancelJobs(r)
			if r.next == r.old {
				r.next = m.shader
			}
			r.old = m.shader
			r.device = c.deviceGeneration()
		}
		if c.finishReload(r) && c.isolated[m] == r {
			delete(c.isolated, m)
		}
	}

	keep := c.retired[:0]
	for _, sh := range c.retired {
		live := c.used[sh] == c.frame
		for m := range c.mats {
			if m.shader == sh {
				live = true
				break
			}
		}
		for _, r := range c.reloads {
			if r.next == sh {
				live = true
				break
			}
		}
		for _, r := range c.isolated {
			if r.next == sh {
				live = true
				break
			}
		}
		if live {
			keep = append(keep, sh)
		} else {
			c.drop(sh)
		}
	}
	c.retired = keep
}

func (c *Cache) drop(sh *Shader) {
	delete(c.lightFirstUse, sh)
	for key, derived := range c.spec {
		if key.sh == sh {
			delete(c.spec, key)
			if derived != nil {
				c.drop(derived)
			}
		}
	}
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
	for key := range c.domainFailed {
		if key.shader == sh {
			delete(c.domainFailed, key)
		}
	}
}

func (c *Cache) Stats() Stats {
	s := c.stats
	s.Materials, s.Programs, s.Pipelines, s.Pending = len(c.mats), len(c.progs), len(c.pipes), len(c.reloads)+len(c.isolated)
	return s
}

func (c *Cache) Close() {
	if !c.laneBuffer.IsZero() {
		c.dev.DestroyBuffer(c.laneBuffer)
		c.laneBuffer = gpu.BufferHandle{}
	}
	for old := range c.reloads {
		c.CancelReload(old)
	}
	for m := range c.isolated {
		c.CancelMaterial(m)
	}
	for _, h := range c.pipes {
		c.dev.DestroyPipeline(h)
	}
	for _, h := range c.progs {
		c.dev.DestroyShader(h)
	}
	for m := range c.mats {
		if !m.ub.IsZero() {
			c.dev.DestroyBuffer(m.ub)
		}
		m.ub, m.ubSize, m.dirty = gpu.BufferHandle{}, 0, true
	}
	clear(c.pipes)
	clear(c.progs)
	clear(c.spec)
	clear(c.lightFirstUse)
	clear(c.mats)
	clear(c.failed)
	clear(c.used)
	c.retired = nil
}

// SetReloadCheck lets a module validate live node overrides without importing
// scenes into the renderer. Checks run outside the hot path. A nil check removes it.
func (c *Cache) SetReloadCheck(key string, check func(old, next *Shader) error) {
	if check == nil {
		delete(c.checks, key)
	} else {
		c.checks[key] = check
	}
}

// QueueReloadDefinitions includes material source edits in the same shader
// transaction. Definition pointers are CPU-only; live material identities stay stable.
func (c *Cache) QueueReloadDefinitions(old, next *Shader, definitions map[*Material]*Material, commit func()) uint64 {
	mats := make([]*Material, 0, len(definitions))
	for m := range definitions {
		mats = append(mats, m)
	}
	token := c.QueueReload(old, next, mats, commit)
	if r := c.reloads[old]; r != nil {
		r.definitions = make(map[*Material]*Material, len(definitions))
		for m, def := range definitions {
			r.definitions[m] = def
		}
	}
	return token
}

func (c *Cache) QueueMaterial(m, candidate *Material, commit func()) uint64 {
	if m == nil || candidate == nil {
		return 0
	}
	c.CancelMaterial(m)
	c.serial++
	c.Track(m)
	r := &reload{old: m.shader, next: candidate.shader, mats: []*Material{m}, commit: commit,
		token: c.serial, device: c.deviceGeneration(), jobs: map[int]gpu.ShaderCompilation{}, definitions: map[*Material]*Material{m: candidate}, only: true}
	for _, group := range c.reloads {
		if group.next == candidate.shader {
			r.dependency = group
			break
		}
	}
	c.isolated[m] = r
	return c.serial
}

func (c *Cache) CancelMaterial(m *Material) {
	if r := c.isolated[m]; r != nil {
		if r.state == reloadPending {
			r.state = reloadCanceled
		}
		c.cancelJobs(r)
		if r.next != r.old {
			c.dropUnreferenced(r.next)
		}
		delete(c.isolated, m)
	}
}

func (c *Cache) SetMaterialReloadCheck(key string, check func(*Material, *Shader) error) {
	if check == nil {
		delete(c.materialChecks, key)
	} else {
		c.materialChecks[key] = check
	}
}

func (c *Cache) dropUnreferenced(sh *Shader) {
	for m := range c.mats {
		if m.shader == sh {
			return
		}
	}
	c.drop(sh)
}

// PreparePost preflights the actual runtime program and its UBO before clearing the destination.
func (c *Cache) PreparePost(m *Material, final bool) (gpu.PipelineHandle, bool) {
	if m == nil || m.shader == nil || m.shader.Domain != shader.DomainPostFX || c.lost() {
		return gpu.PipelineHandle{}, false
	}
	c.Track(m)
	blend := shader.BlendOpaque
	if final {
		blend = shader.BlendPremultiplied
	}
	pipe, ok := c.pipelineLayout(m.shader, m.variant, blend, false, false)
	if !ok || !c.upload(m) {
		return gpu.PipelineHandle{}, false
	}
	c.used[m.shader] = c.frame
	return pipe, true
}

// BindPostParameters binds shared material resources; the pass retains ownership of tex0 and block 0.
func (c *Cache) BindPostParameters(p gpu.Pass, m *Material) {
	sh := m.shader
	if !m.ub.IsZero() {
		p.SetUniformBuffer(shader.SlotMaterial, m.ub)
	}
	for _, i := range sh.Schema.Textures {
		slot := sh.Schema.Params[i].Slot
		// The renderer resolves material normal -> frame pair -> flat and binds
		// this engine slot together with its per-surface mapping.
		if slot == shader.TexNormal || slot == shader.TexEmission {
			continue
		}
		t := m.tex[textureIndex(slot)]
		if t.Texture.IsZero() {
			t = c.white
		} else if t.Sampler.IsZero() {
			t.Sampler = c.white.Sampler
		}
		p.SetTexture(slot, t.Texture, t.Sampler)
	}
}
