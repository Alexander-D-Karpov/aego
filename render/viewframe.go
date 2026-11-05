package render

import (
	"fmt"
	"time"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/culling"
	"aego/render/debug"
	"aego/render/lighting"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/text"
	"aego/render/view"
)

const maxClipDepth = 16

type ViewFrame struct {
	postProcess       *postfx.Plan
	lighting          *lighting.Input
	lightingInvalid   bool
	droppedLights     []bool
	sets              []lighting.Set
	setIndex          map[lighting.Set]uint32
	litCount          int
	lightResources    *lightingView
	output            *lightingOutput
	outputOriginal    view.View
	outputEnvironment lighting.Environment
	hasAdditive       bool
	r                 *Renderer
	view              view.View
	pass              gpu.Pass
	bounds            mathx.AABB2
	frustum           mathx.Frustum
	cam               mathx.Vec3
	clips             [maxClipDepth]mathx.Rect
	clipOn            [maxClipDepth]bool
	depth             int
	submission        uint32
	ended             bool

	trackResolveBounds bool
	hasResolveBounds   bool
	resolveBounds      mathx.AABB2
	brightBounds       mathx.AABB2
	hasBrightBounds    bool
	brightThreshold    float32
	sceneGroups        sceneGroups
}

func (f *ViewFrame) reset(v view.View) {
	r := f.r
	w, h := r.framebuffer()
	v.Normalize(w, h, r.fbScale)
	f.view = v
	f.bounds = v.VisibleBounds()
	f.frustum = v.Frustum()
	f.cam = v.CameraPosition()
	f.depth = 0
	f.submission = 0
	f.ended = false
	f.pass = nil
	f.lighting = nil
	f.lightingInvalid = false
	f.droppedLights = f.droppedLights[:0]
	f.sets = f.sets[:0]
	clear(f.setIndex)
	f.litCount = 0
	f.lightResources = nil
	f.output = nil
	f.postProcess = nil
	f.hasAdditive = false
	f.trackResolveBounds, f.hasResolveBounds = false, false
	f.hasBrightBounds, f.brightThreshold = false, 0
	f.sceneGroups.count = 0
	r.batcher.Reset()
	r.batcher.SetView(v.ZMin, v.ZMax, v.PixelsPerUnit)
	r.sprites3d.Reset()
	r.meshes.Reset()
	r.dbg.Reset()
}

func (f *ViewFrame) End() {
	if f.ended {
		return
	}
	f.ended = true
	r := f.r
	r.batcher.Sort()
	r.sprites3d.Sort()
	r.meshes.Sort()
	r.stats.ZClamped += r.batcher.Clamped()
	if !r.prepareLighting(f) {
		return
	}
	var sceneStart time.Time
	if f.litCount > 0 {
		sceneStart = time.Now()
	}
	r.beginView(f)
	if f.pass == nil {
		return
	}
	r.passCtx = PassContext{Renderer: r, View: &f.view, Pass: f.pass, Frame: f}
	for i := range r.order {
		z := r.tracer.Begin(r.passZone[i])
		r.order[i].Pass.Execute(&r.passCtx)
		z.End()
	}
	if r.stats.Views == 1 && !r.inspection.active {
		r.prof.End(f.pass)
	}
	f.pass.End()
	f.pass = nil
	if !sceneStart.IsZero() {
		r.stats.LightingSceneNS += uint64(time.Since(sceneStart))
	}
	r.resolveLighting(f)
}

// Lighting supplies presentation descriptors for this view. It must precede
// drawable submission and remains borrowed until End returns.
func (f *ViewFrame) Lighting(input *lighting.Input) error {
	f.lighting = input
	f.trackResolveBounds = f.submission == 0 && input != nil && !input.Disabled && f.r.caps.ESSL && !f.r.customPasses && f.view.Target.IsZero() && !f.view.Is3D() && f.view.DoClear && f.view.Clear.A == 1 && !f.view.DeferResolve
	f.lightingInvalid = false
	f.droppedLights = f.droppedLights[:0]
	if input == nil {
		return nil
	}
	var err error
	if len(input.Lights) > lighting.MaxInputLights || len(input.Occluders) > lighting.MaxOccluders || len(input.Points) > lighting.MaxPoints {
		err = fmt.Errorf("LGT004: lighting descriptor budget exceeded: %d lights, %d occluders, %d points", len(input.Lights), len(input.Occluders), len(input.Points))
	} else {
		for i := range input.Lights {
			if e := input.Lights[i].Validate(); e != nil {
				err = fmt.Errorf("light %s: %w", input.Lights[i].ID, e)
				break
			}
		}
	}
	if err != nil {
		f.lightingInvalid = true
		f.r.lightingFailure(err)
		return err
	}
	if cap(f.droppedLights) < len(input.Lights) {
		f.droppedLights = make([]bool, len(input.Lights), max(lighting.MaxShadows, len(input.Lights)))
	} else {
		f.droppedLights = f.droppedLights[:len(input.Lights)]
		clear(f.droppedLights)
	}
	return nil
}

func (f *ViewFrame) lightSet(bounds mathx.AABB2, layers uint64) uint32 {
	start := time.Now()
	defer func() { f.r.stats.LightingSelectNS += uint64(time.Since(start)) }()
	var set lighting.Set
	dropped := 0
	if f.lighting != nil && !f.lighting.Disabled && !f.lightingInvalid {
		set, dropped = lighting.Select(f.lighting.Lights, bounds, layers)
		// Sort influence-selected members by stable identity without scratch allocation, avoiding distinct GPU bindings for different receiver rankings without changing draw order.
		for i := 1; i < set.Count; i++ {
			index := set.Indices[i]
			id := f.lighting.Lights[index].ID
			j := i
			for j > 0 {
				previous := set.Indices[j-1]
				pid := f.lighting.Lights[previous].ID
				if pid.Hi < id.Hi || pid.Hi == id.Hi && (pid.Lo < id.Lo || pid.Lo == id.Lo && previous < index) {
					break
				}
				set.Indices[j] = previous
				j--
			}
			set.Indices[j] = index
		}
	}
	f.r.stats.DroppedLightAssignments += uint32(dropped)
	if dropped > 0 {
		for i := range f.lighting.Lights {
			if f.droppedLights[i] || !lighting.Affects(&f.lighting.Lights[i], bounds, layers) {
				continue
			}
			selected := false
			for j := 0; j < set.Count; j++ {
				if set.Indices[j] == i {
					selected = true
					break
				}
			}
			if !selected {
				f.droppedLights[i] = true
				f.r.stats.DroppedLights++
			}
		}
	}
	f.litCount++
	if f.setIndex == nil {
		f.setIndex = make(map[lighting.Set]uint32)
	}
	if id, ok := f.setIndex[set]; ok {
		return id
	}
	f.sets = append(f.sets, set)
	id := uint32(len(f.sets))
	f.setIndex[set] = id
	f.r.stats.LightSets++
	f.r.stats.SelectedLights += uint32(set.Count)
	return id
}
func (f *ViewFrame) View() *view.View {
	return &f.view
}

func (f *ViewFrame) Debug() *debug.Draw {
	return f.r.dbg
}

func (f *ViewFrame) PushClip(r mathx.Rect) {
	if f.depth >= maxClipDepth {
		return
	}
	scale := f.view.PixelScale
	px := mathx.Rect{X: r.X * scale, Y: r.Y * scale, W: r.W * scale, H: r.H * scale}
	if f.depth > 0 && f.clipOn[f.depth-1] {
		px = intersect(px, f.clips[f.depth-1])
	}
	f.clips[f.depth] = px
	f.clipOn[f.depth] = true
	f.depth++
}

func (f *ViewFrame) PopClip() {
	if f.depth > 0 {
		f.depth--
	}
}

func intersect(a, b mathx.Rect) mathx.Rect {
	x0 := max(a.X, b.X)
	y0 := max(a.Y, b.Y)
	x1 := min(a.X+a.W, b.X+b.W)
	y1 := min(a.Y+a.H, b.Y+b.H)
	return mathx.Rect{X: x0, Y: y0, W: max(0, x1-x0), H: max(0, y1-y0)}
}

func (f *ViewFrame) clip() (mathx.Rect, bool) {
	if f.depth == 0 {
		return mathx.Rect{}, false
	}
	return f.clips[f.depth-1], f.clipOn[f.depth-1]
}

func (f *ViewFrame) Sprite(d sprite.Draw) {
	f.submit(&d)
}

func (f *ViewFrame) Sprites(ds []sprite.Draw) {
	for i := range ds {
		f.submit(&ds[i])
	}
}

func (f *ViewFrame) submit(d *sprite.Draw) {
	r := f.r
	r.stats.Submitted++
	if d.Region.IsZero() {
		return
	}
	bounded := d.Material == nil || !d.Material.Shader().Vertex
	var bounds mathx.AABB2
	if bounded {
		bounds = culling.Bounds(d, f.view.PixelsPerUnit)
		if !f.bounds.Intersects(bounds) {
			r.stats.Culled++
			return
		}
	}
	clip, on := f.clip()
	if d.Blend == sprite.BlendAdditive || d.Material != nil && d.Material.Blend() == shader.BlendAdditive {
		f.hasAdditive = true
	}
	if d.Material != nil && d.Material.Shader().Features&shader.FeatureLighting != 0 {
		copyDraw := *d
		d = &copyDraw
		d.Lighting = true
		if normal, explicit, _ := d.Material.Normal(); explicit {
			d.Normal = normal
		}
		if emission, explicit, _ := d.Material.Emission(); explicit {
			d.Emission = emission
		}
		if !bounded {
			bounds = culling.Bounds(d, f.view.PixelsPerUnit)
		}
		d.LightSet = f.lightSet(bounds, d.VisibilityLayers)
	}
	if !r.batcher.PushOrdered(d, r.defSampler, clip, on, f.submission) {
		r.stats.Dropped++
		if !r.warned {
			r.warned = true
			r.log.Warn("sprite budget exceeded, extra sprites are dropped")
		}
		return
	}
	if f.trackResolveBounds {
		if !bounded || !f.stableSpriteBounds(d) {
			f.trackResolveBounds = false
		} else {
			if f.brightThreshold > 0 {
				f.sceneGroups.add(bounds)
			}
			if f.brightThreshold > 0 && !f.belowBloomThreshold(d) {
				if f.hasBrightBounds {
					f.brightBounds = f.brightBounds.Union(bounds)
				} else {
					f.brightBounds, f.hasBrightBounds = bounds, true
				}
			}
			if f.hasResolveBounds {
				f.resolveBounds = f.resolveBounds.Union(bounds)
			} else {
				f.resolveBounds, f.hasResolveBounds = bounds, true
			}
		}
	}
	r.stats.Visible++
	f.submission++
}

func (f *ViewFrame) Text(font *text.BitmapFont, s string, pos mathx.Vec2, scale float32, c mathx.Color, layer, order int16) {
	if font == nil || s == "" {
		return
	}
	r := f.r
	r.textBuf = font.Layout(s, pos, scale, c, layer, order, r.textBuf[:0])
	f.Sprites(r.textBuf)
}

func (f *ViewFrame) TextFont(font *text.Font, s string, pos mathx.Vec2, scale float32, c mathx.Color, layer, order int16) {
	if font == nil || s == "" {
		return
	}
	r := f.r
	r.textBuf = font.Layout(s, pos, scale, c, layer, order, r.textBuf[:0])
	f.Sprites(r.textBuf)
}

func (f *ViewFrame) stableSpriteBounds(d *sprite.Draw) bool {
	size, origin := d.WorldSize(f.view.PixelsPerUnit), d.Origin
	if d.Region.Trimmed() {
		_, size, origin = d.Geometry(f.view.PixelsPerUnit)
	}
	if !(origin.X >= 0 && origin.X <= 1 && origin.Y >= 0 && origin.Y <= 1) {
		return false
	}
	basis := float32(2)
	if d.HasLinear {
		basis = max(mathx.Abs(d.Linear[0])+mathx.Abs(d.Linear[2]), mathx.Abs(d.Linear[1])+mathx.Abs(d.Linear[3]))
	}
	operands := max(mathx.Abs(d.Position.X), mathx.Abs(d.Position.Y)) + basis*max(mathx.Abs(size.X), mathx.Abs(size.Y))
	return operands*max(float32(1), f.view.Camera.Zoom) <= 1<<18
}
