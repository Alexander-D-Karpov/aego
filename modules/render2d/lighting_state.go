package render2d

import (
	"math"
	"slices"

	"aego/asset"
	"aego/asset/silhouette"
	"aego/contracts/shapes2d"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/engine/settings"
	"aego/render/lighting"
	"aego/render/material"
	"aego/render/postfx"
	"aego/render/texture"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
)

const lightingServiceID = "render2d.lighting"

type occluderKey struct {
	node ids.NodeID
	part uint32
}
type contourCache struct {
	owner                                     ids.NodeID
	sourceRevision                            uint64
	local, world                              []mathx.Vec2
	pose                                      mathx.Mat3
	bounds                                    mathx.AABB2
	closed                                    bool
	geometryRevision, transformRevision, seen uint64
	geometryError, transformError             string
}

// State is attached as a view service: prepared geometry and GPU-related
// revisions are never simulation keyframes or gameplay RNG consumers.
type lightingState struct {
	settings             *settings.Store
	reducedKey           settings.Key
	postProcessingKey    settings.Key
	scene                *scene.Scene
	postProcess          map[view.ID]*postProcessPlan
	fallbackEffects      *postfx.Registry
	cache                map[occluderKey]*contourCache
	spareQuads           []*contourCache
	shapes               shapes2d.Buffer
	input                lighting.Input
	next, epoch          uint64
	diagnostics, pending []problems.Diagnostic
	diagnosticRevision   uint64
	drop                 func()
}

func (m Module) Init(ctx *module.InitContext) error {
	if m.lighting != nil {
		m.lighting.problems = ctx.Problems
		m.lighting.settings = ctx.Settings
	}
	return nil
}

func (m Module) attachLighting(ctx *module.AttachContext) {
	if ctx.World == nil || ctx.Scene == nil {
		return
	}
	if old, ok := engine.ServiceOf[*lightingState](ctx.World, lightingServiceID); ok {
		if old.scene == ctx.Scene {
			return
		}
		old.close()
	}
	s := &lightingState{scene: ctx.Scene, cache: make(map[occluderKey]*contourCache)}
	if m.lighting != nil && m.lighting.settings != nil {
		s.settings = m.lighting.settings
		s.reducedKey = s.settings.Key("accessibility.reducedMotion")
		s.postProcessingKey = s.settings.Key(PostProcessingKey)
	}
	if m.lighting != nil && m.lighting.problems != nil {
		s.drop = m.lighting.problems.Add(problems.Source{
			Revision: func() uint64 { return s.diagnosticRevision },
			Append:   func(dst []problems.Diagnostic) []problems.Diagnostic { return append(dst, s.diagnostics...) },
		})
	}
	ctx.World.SetView(lightingServiceID, s)
}

func detachLighting(ctx *module.AttachContext) {
	if s, ok := engine.ServiceOf[*lightingState](ctx.World, lightingServiceID); ok && s.scene == ctx.Scene {
		s.close()
		ctx.World.RemoveService(lightingServiceID)
	}
}

func (s *lightingState) close() {
	if s.drop != nil {
		s.drop()
		s.drop = nil
	}
	clear(s.cache)
	s.spareQuads = nil
	s.postProcess = nil
	s.scene = nil
	s.input = lighting.Input{}
	s.shapes = shapes2d.Buffer{}
	s.diagnostics, s.pending = nil, nil
}

// LightingDiagnostics exposes the current aggregate without printing one
// warning every frame. Callers must treat the returned slice as read-only.
func LightingDiagnostics(w *engine.World) ([]problems.Diagnostic, uint64) {
	if s, ok := engine.ServiceOf[*lightingState](w, lightingServiceID); ok {
		return s.diagnostics, s.diagnosticRevision
	}
	return nil, 0
}

func (s *lightingState) diagnostic(node ids.NodeID, field, code, message string, severity problems.Severity) {
	for i := range s.pending {
		d := &s.pending[i]
		if d.Code == code && d.Subject.Node == node && d.Subject.Field == field {
			return
		}
	}
	if len(s.pending) >= lighting.MaxInputLights {
		return
	}
	s.pending = append(s.pending, problems.Diagnostic{Owner: ID, Code: code, Severity: severity, Message: message,
		Subject: problems.Subject{Kind: problems.SubjectField, Scene: s.scene, Node: node, Field: field}})
}

func (s *lightingState) finish() {
	for key, entry := range s.cache {
		if entry.seen != s.epoch {
			delete(s.cache, key)
			// Animated alpha silhouettes have a different number of rectangular
			// parts on each frame. Keep only their small backing arrays; detached
			// node identities, errors, revisions and large polygons are never pooled.
			if len(s.spareQuads) < lighting.MaxOccluders && len(entry.local) == 4 && cap(entry.local) == 4 && cap(entry.world) <= 4 {
				*entry = contourCache{local: entry.local[:0], world: entry.world[:0]}
				s.spareQuads = append(s.spareQuads, entry)
			}
		}
	}
	if !slices.Equal(s.diagnostics, s.pending) {
		s.diagnostics, s.pending = s.pending, s.diagnostics
		s.diagnosticRevision++
	}
}

func finiteLighting(f float32) bool { return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0) }
func finiteLightingPose(m mathx.Mat3) bool {
	for _, v := range m {
		if !finiteLighting(v) {
			return false
		}
	}
	return true
}
func sameLightingPoints(a, b []mathx.Vec2) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float32bits(a[i].X) != math.Float32bits(b[i].X) || math.Float32bits(a[i].Y) != math.Float32bits(b[i].Y) {
			return false
		}
	}
	return true
}
func sameLightingPose(a, b mathx.Mat3) bool {
	for i := range a {
		if math.Float32bits(a[i]) != math.Float32bits(b[i]) {
			return false
		}
	}
	return true
}

func (s *lightingState) prepare(r *frame.Runner, world *engine.World) {
	s.epoch++
	s.input.Reset()
	clear(s.pending)
	s.pending = s.pending[:0]
	sc := s.scene
	scene.Query1[Light2D](sc).Each(func(h scene.NodeHandle, c *Light2D) {
		if sc.Space(h) != scene.Space2D || sc.Under3D(h) || r.Hidden(h) {
			return
		}
		l := c.descriptor()
		l.ID = sc.ID(h)
		pose := sc.RenderWorld(h)
		if !finiteLightingPose(pose) {
			s.diagnostic(l.ID, "engine.Light2D", "LGT007", "Light presentation transform contains NaN or infinity.", problems.Error)
			return
		}
		l.Position = mathx.Vec2{X: pose[6], Y: pose[7]}
		length := math.Hypot(float64(pose[0]), float64(pose[1]))
		if length <= 1e-20 {
			s.diagnostic(l.ID, "engine.Light2D", "LGT007", "Light direction has a degenerate presentation transform; local +X is used.", problems.Warning)
		} else {
			l.Direction = mathx.Vec2{X: float32(float64(pose[0]) / length), Y: float32(float64(pose[1]) / length)}
		}
		if err := l.Validate(); err != nil {
			s.diagnostic(l.ID, "engine.Light2D", lightDiagnosticCode(err), err.Error(), problems.Error)
			return
		}
		if !c.Cookie.IsZero() {
			var ok bool
			l.Cookie, ok = c.Cookie.Region(r.Resolver())
			if !ok {
				s.diagnostic(l.ID, "engine.Light2D.cookie", "LGT006", "Light cookie texture is unresolved; its AssetID is preserved and a white mask is used.", problems.Warning)
			}
		}
		if len(s.input.Lights) >= lighting.MaxInputLights {
			s.diagnostic(ids.NodeID{}, "engine.Light2D", "LGT008", "Scene exceeds the bounded lighting input budget.", problems.Error)
			return
		}
		s.input.Lights = append(s.input.Lights, l)
	})
	scene.Query1[LightingEnvironment2D](sc).Each(func(h scene.NodeHandle, e *LightingEnvironment2D) {
		if e.Camera.IsZero() {
			return
		}
		camera, ok := e.Camera.Resolve(sc)
		if !ok || !scene.Has[Camera2D](sc, camera) {
			s.diagnostic(sc.ID(h), "engine.LightingEnvironment2D.camera", "LGT003", "Lighting environment refers to a missing Camera2D; its NodeID is preserved.", problems.Error)
		}
	})
	provider, _ := engine.ServiceOf[shapes2d.Provider](world, shapes2d.ServiceID)
	scene.Query1[LightOccluder2D](sc).Each(func(h scene.NodeHandle, c *LightOccluder2D) {
		if sc.Space(h) != scene.Space2D || sc.Under3D(h) || r.Hidden(h) || c.Layers == 0 {
			return
		}
		id := sc.ID(h)
		if c.Source > OccluderSpriteAlpha || c.CullMode > lighting.CullCounterClockwise {
			s.diagnostic(id, "engine.LightOccluder2D", "LGT004", "Unknown occluder source or culling mode.", problems.Error)
			return
		}
		if c.Source == OccluderPolygon {
			s.appendContour(occluderKey{id, 0}, id, 0, c.Polygon, c.Closed, c.CullMode, uint64(c.Layers), sc.RenderWorld(h))
			return
		}
		if c.Source == OccluderSpriteAlpha {
			sr, ok := scene.Get[SpriteRenderer](sc, h)
			if !ok {
				return
			}
			src, ok := sc.Resolver().(interface {
				SpriteShadow(ids.AssetID) (silhouette.Shape, bool)
			})
			if !ok {
				return
			}
			shape, ok := src.SpriteShadow(sr.Sprite.ID)
			if !ok {
				s.diagnostic(id, "engine.LightOccluder2D.source", "LGT004", "Missing or unsupported alpha shadow geometry for this sprite frame.", problems.Warning)
				return
			}
			size := Size(sr, mathx.V2(float32(shape.W), float32(shape.H)))
			pose := sc.RenderWorld(h)
			for i, b := range shape.Parts {
				left, right := float32(b.X)/float32(shape.W), float32(b.X+b.W)/float32(shape.W)
				top, bottom := float32(b.Y)/float32(shape.H), float32(b.Y+b.H)/float32(shape.H)
				if sr.FlipX {
					left, right = 1-right, 1-left
				}
				if sr.FlipY {
					top, bottom = 1-bottom, 1-top
				}
				left, right = (left-sr.Origin.X)*size.X, (right-sr.Origin.X)*size.X
				top, bottom = (top-sr.Origin.Y)*size.Y, (bottom-sr.Origin.Y)*size.Y
				points := [4]mathx.Vec2{mathx.V2(left, top), mathx.V2(right, top), mathx.V2(right, bottom), mathx.V2(left, bottom)}
				s.appendContour(occluderKey{id, uint32(i + 1)}, id, 0, points[:], true, c.CullMode, uint64(c.Layers), pose)
			}
			return
		}
		if provider == nil {
			s.diagnostic(id, "engine.LightOccluder2D.source", "LGT005", "ShapeProvider is unavailable in this world. Load a geometry provider or use a manual Polygon.", problems.Warning)
			return
		}
		owner := id
		if !c.ShapeNode.IsZero() {
			owner = c.ShapeNode.ID
		}
		s.shapes.Reset()
		if err := provider.Append(owner, c.Shape, &s.shapes); err != nil {
			s.diagnostic(id, "engine.LightOccluder2D.shape", "LGT005", err.Error(), problems.Warning)
			return
		}
		for _, shape := range s.shapes.Contours {
			ownerHandle := sc.Handle(shape.Owner)
			if !sc.IsAlive(ownerHandle) {
				s.diagnostic(id, "engine.LightOccluder2D.shape", "LGT005", "ShapeProvider returned a missing presentation owner.", problems.Warning)
				continue
			}
			s.appendContour(occluderKey{id, shape.ID.Part}, shape.Owner, shape.Revision, s.shapes.Vertices(shape), shape.Closed, c.CullMode, uint64(c.Layers), sc.RenderWorld(ownerHandle))
		}
	})
}

func (s *lightingState) appendContour(key occluderKey, owner ids.NodeID, sourceRevision uint64, points []mathx.Vec2, closed bool, cull lighting.CullMode, layers uint64, pose mathx.Mat3) {
	if len(points) > shapes2d.MaxContourPoints || len(s.input.Occluders) >= lighting.MaxOccluders || len(points) > lighting.MaxPoints-len(s.input.Points) {
		s.diagnostic(key.node, "engine.LightOccluder2D", "LGT008", "Occluder geometry exceeds the bounded contour or point budget.", problems.Error)
		return
	}
	g := s.cache[key]
	if g == nil {
		if len(s.cache) >= lighting.MaxOccluders {
			s.diagnostic(key.node, "engine.LightOccluder2D", "LGT008", "Occluder cache exceeds its bounded entry budget.", problems.Error)
			return
		}
		if n := len(s.spareQuads); len(points) == 4 && n > 0 {
			g = s.spareQuads[n-1]
			s.spareQuads[n-1] = nil
			s.spareQuads = s.spareQuads[:n-1]
		} else {
			g = &contourCache{}
		}
		s.cache[key] = g
	}
	g.seen = s.epoch
	geometryChanged := g.geometryRevision == 0 || g.owner != owner || g.sourceRevision != sourceRevision || g.closed != closed || !sameLightingPoints(g.local, points)
	if geometryChanged {
		g.owner, g.sourceRevision, g.closed = owner, sourceRevision, closed
		g.local = append(g.local[:0], points...)
		s.next++
		g.geometryRevision = s.next
		g.geometryError = ""
		if err := shapes2d.ValidateContour(points, closed); err != nil {
			g.geometryError = err.Error()
		}
	}
	if g.geometryError != "" {
		s.diagnostic(key.node, "engine.LightOccluder2D.polygon", "LGT004", g.geometryError, problems.Error)
		return
	}
	poseChanged := g.transformRevision == 0 || !sameLightingPose(g.pose, pose)
	if poseChanged {
		s.next++
		g.transformRevision = s.next
		g.pose = pose
	}
	if geometryChanged || poseChanged {
		g.world = g.world[:0]
		g.transformError = ""
		if !finiteLightingPose(pose) {
			g.transformError = "Occluder presentation transform contains NaN or infinity."
		} else if float64(pose[0])*float64(pose[4])-float64(pose[1])*float64(pose[3]) == 0 {
			g.transformError = "Occluder presentation transform is degenerate; no shadow geometry is emitted."
		} else {
			for _, p := range points {
				v := pose.Apply(p)
				if !finiteLighting(v.X) || !finiteLighting(v.Y) {
					g.transformError = "Transformed occluder coordinates exceed the finite world range."
					break
				}
				if len(g.world) == 0 {
					g.bounds = mathx.AABB2{Min: v, Max: v}
				} else {
					g.bounds.Min.X, g.bounds.Min.Y = min(g.bounds.Min.X, v.X), min(g.bounds.Min.Y, v.Y)
					g.bounds.Max.X, g.bounds.Max.Y = max(g.bounds.Max.X, v.X), max(g.bounds.Max.Y, v.Y)
				}
				g.world = append(g.world, v)
			}
		}
	}
	if g.transformError != "" {
		s.diagnostic(key.node, "engine.LightOccluder2D", "LGT007", g.transformError, problems.Error)
		return
	}
	s.input.Occluders = append(s.input.Occluders, lighting.Occluder{Bounds: g.bounds, ID: key.node, Part: key.part, GeometryRevision: g.geometryRevision, TransformRevision: g.transformRevision,
		First: len(s.input.Points), Count: len(g.world), Closed: closed, Cull: cull, Layers: layers})
	s.input.Points = append(s.input.Points, g.world...)
}

func (s *lightingState) prepareView(r *frame.Runner, v view.View, dst *lighting.Input) {
	dst.Reset()
	enabled, debug := r.LightingView()
	dst.Disabled, dst.Debug = !enabled, debug
	_, width, height := r.Target()
	v.Normalize(width, height, r.PixelScale())
	bounds := v.VisibleBounds()
	for i := range s.input.Lights {
		l := &s.input.Lights[i]
		if l.Layers&uint64(v.Layers) == 0 || l.Intensity <= 0 || l.Type != lighting.Directional && !bounds.Intersects(lighting.LightBounds(l)) {
			continue
		}
		dst.Lights = append(dst.Lights, *l)
	}
	// An offscreen caster can shadow a visible receiver. These independent
	// per-camera buffers include all prepared casters; the shadow pass clips
	// each light's geometry to its influence, not to sprite visibility.
	dst.Occluders = append(dst.Occluders, s.input.Occluders...)
	dst.Points = append(dst.Points, s.input.Points...)
	if selected := r.LightingEnvironmentPreview(); !selected.IsZero() {
		h := s.scene.Handle(selected)
		e, ok := scene.Get[LightingEnvironment2D](s.scene, h)
		if !ok || !s.scene.EffectiveEnabled(h) || s.scene.Space(h) != scene.Space2D || s.scene.Under3D(h) {
			s.diagnostic(selected, "engine.LightingEnvironment2D", "LGT003", "The selected Scene View environment is missing, disabled or outside 2D. Compatible defaults are used; reset the preview selection or restore the node.", problems.Warning)
			return
		}
		x := e.descriptor()
		if err := x.Validate(); err != nil {
			s.diagnostic(selected, "engine.LightingEnvironment2D", "LGT003", err.Error(), problems.Error)
			return
		}
		dst.Environment, dst.HasEnvironment = x, true
		return
	}
	count := 0
	var envNode ids.NodeID
	scene.Query1[LightingEnvironment2D](s.scene).Each(func(h scene.NodeHandle, e *LightingEnvironment2D) {
		if s.scene.Space(h) != scene.Space2D || s.scene.Under3D(h) {
			return
		}
		if !e.Camera.IsZero() && e.Camera.ID != v.ID.Node {
			return
		}
		count++
		envNode = s.scene.ID(h)
		x := e.descriptor()
		if err := x.Validate(); err != nil {
			s.diagnostic(envNode, "engine.LightingEnvironment2D", "LGT003", err.Error(), problems.Error)
			return
		}
		dst.Environment, dst.HasEnvironment = x, true
	})
	if count > 1 {
		dst.Environment, dst.HasEnvironment = lighting.DefaultEnvironment(), false
		s.diagnostic(envNode, "engine.LightingEnvironment2D", "LGT003", "Several environments apply to this camera output; compatible defaults are used until the conflict is resolved.", problems.Error)
	}
}

func (s *lightingState) normal(m *material.Material, source ids.AssetID, res asset.Resolver, node ids.NodeID, field string) texture.Region {
	region, explicit, resolved := m.Normal()
	if explicit {
		if !resolved {
			s.diagnostic(node, field, "LGT006", "Explicit material normal texture is unresolved; its AssetID is preserved and a flat normal is used.", problems.Warning)
		}
		return region
	}
	if pairs, ok := res.(asset.NormalSource); ok && !source.IsZero() {
		id, normal, found := pairs.NormalFor(source)
		if found {
			return normal
		}
		if !id.IsZero() {
			s.diagnostic(node, field, "LGT006", "Sprite frame's normal texture is unresolved; its AssetID is preserved and a flat normal is used.", problems.Warning)
		}
	}
	return texture.Region{}
}

func (s *lightingState) emission(m *material.Material, source ids.AssetID, res asset.Resolver, node ids.NodeID, field string) texture.Emission {
	channel, explicit, resolved := m.Emission()
	if explicit {
		if !resolved {
			s.diagnostic(node, field, "LGT006", "Explicit material emission texture is unresolved; its AssetID is preserved and uniform emission is used.", problems.Warning)
		}
		return channel
	}
	if pairs, ok := res.(asset.EmissionSource); ok && !source.IsZero() {
		id, emission, found := pairs.EmissionFor(source)
		if found {
			return emission
		}
		if !id.IsZero() {
			s.diagnostic(node, field, "LGT006", "Sprite frame's emission texture is unresolved; its AssetID is preserved and uniform emission is used.", problems.Warning)
		}
	}
	return texture.Emission{}
}
