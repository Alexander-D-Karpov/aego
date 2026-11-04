package physics

import (
	"errors"
	"fmt"
	"math"

	"aego/contracts/shapes2d"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/modules/physics/components"
	"aego/scene"
)

const shapeCurveSegments = 32

type shapeGeometry struct {
	source    components.Collider
	signature []mathx.Vec2
	points    []mathx.Vec2
	revision  uint64
	closed    bool
	err       error
}

// shapeProvider reads authoring components, not a simulation. Its lifetime is
// the attached world, including Edit and preview worlds without a solver.
type shapeProvider struct {
	scene *scene.Scene
	cache map[shapes2d.ID]*shapeGeometry
	next  uint64
	drop  func()
}

func newShapeProvider(s *scene.Scene) *shapeProvider {
	p := &shapeProvider{scene: s, cache: make(map[shapes2d.ID]*shapeGeometry)}
	p.drop = s.Events().Subscribe(func(e scene.Event) {
		switch e.Kind {
		case scene.EvNodeDestroyed:
			for kind := components.Kind(0); kind < components.KindCount; kind++ {
				delete(p.cache, shapeID(e.ID, kind))
			}
		case scene.EvComponentRemoved:
			for kind := components.Kind(0); kind < components.KindCount; kind++ {
				if e.Type == components.ColliderType(kind) {
					delete(p.cache, shapeID(e.ID, kind))
				}
			}
		}
	})
	return p
}

func shapeID(node ids.NodeID, kind components.Kind) shapes2d.ID {
	// Fixed component slots, independent of which other components are present.
	return shapes2d.ID{Node: node, Part: uint32(kind) + 1}
}

func (p *shapeProvider) Close() {
	if p.drop != nil {
		p.drop()
		p.drop = nil
	}
	clear(p.cache)
	p.scene = nil
}

func attachShapes(ctx *module.AttachContext) error {
	if ctx.World == nil || ctx.Scene == nil {
		return nil
	}
	if value, exists := ctx.World.ServiceAny(shapes2d.ServiceID); exists {
		p, ok := value.(*shapeProvider)
		if !ok {
			return errors.New("a shape geometry provider is already registered for this world")
		}
		if p.scene == ctx.Scene {
			return nil
		}
		p.Close()
	}
	ctx.World.SetView(shapes2d.ServiceID, shapes2d.Provider(newShapeProvider(ctx.Scene)))
	return nil
}

func detachShapes(ctx *module.AttachContext) {
	if ctx.World == nil {
		return
	}
	if p, ok := engine.ServiceOf[*shapeProvider](ctx.World, shapes2d.ServiceID); ok && p.scene == ctx.Scene {
		ctx.World.RemoveService(shapes2d.ServiceID)
		p.Close()
	}
}

func (p *shapeProvider) Append(node ids.NodeID, key string, dst *shapes2d.Buffer) (err error) {
	if p.scene == nil {
		return shapes2d.ErrClosed
	}
	if dst == nil {
		return shapes2d.ErrBuffer
	}
	s := p.scene
	h := s.Handle(node)
	if !s.IsAlive(h) {
		return shapes2d.ErrMissingNode
	}
	// Bring enabled/space flags up to date, but return no transformed points.
	_ = s.World(h)
	if s.Space(h) != scene.Space2D || s.Under3D(h) {
		return shapes2d.ErrSpace
	}
	if !s.EffectiveEnabled(h) {
		return nil
	}
	first, points := len(dst.Contours), len(dst.Points)
	defer func() {
		if err != nil {
			clear(dst.Contours[first:])
			dst.Contours, dst.Points = dst.Contours[:first], dst.Points[:points]
		}
	}()
	found := false
	for kind := components.Kind(0); kind < components.KindCount; kind++ {
		if key != "" && key != components.Names[kind] {
			continue
		}
		var c components.Collider
		if !components.Read(s, h, kind, &c) {
			continue
		}
		found = true
		if len(c.Points) > shapes2d.MaxContourPoints {
			return shapes2d.ErrBudget
		}
		id := shapeID(node, kind)
		g := p.cache[id]
		if g == nil {
			if len(p.cache) >= shapes2d.MaxContours {
				return shapes2d.ErrBudget
			}
			g = &shapeGeometry{}
			p.cache[id] = g
		}
		if g.revision == 0 || !g.matches(&c) {
			p.next++
			g.revision = p.next
			g.source = c
			g.source.Points = nil
			g.signature = append(g.signature[:0], c.Points...)
			g.points, g.closed, g.err = localContour(&c, g.points[:0])
			if g.err != nil {
				g.err = fmt.Errorf("%s: %w", components.Names[kind], g.err)
			}
		}
		if g.err != nil {
			return g.err
		}
		if err := dst.Append(shapes2d.Contour{ID: id, Owner: node, Key: components.Names[kind], Revision: g.revision, Closed: g.closed}, g.points); err != nil {
			return err
		}
	}
	if !found && key != "" {
		return shapes2d.ErrMissingShape
	}
	return nil
}

func sameShapeFloat(a, b float32) bool { return math.Float32bits(a) == math.Float32bits(b) }

func sameShapeVec(a, b mathx.Vec2) bool {
	return sameShapeFloat(a.X, b.X) && sameShapeFloat(a.Y, b.Y)
}

func (g *shapeGeometry) matches(c *components.Collider) bool {
	a := &g.source
	if a.Kind != c.Kind || !sameShapeVec(a.Common.Offset, c.Common.Offset) || !sameShapeVec(a.Size, c.Size) ||
		!sameShapeFloat(a.Radius, c.Radius) || !sameShapeFloat(a.Length, c.Length) || a.Loop != c.Loop || len(g.signature) != len(c.Points) {
		return false
	}
	for i := range g.signature {
		if !sameShapeVec(g.signature[i], c.Points[i]) {
			return false
		}
	}
	return true
}

func finiteShape(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

func localContour(c *components.Collider, dst []mathx.Vec2) ([]mathx.Vec2, bool, error) {
	if !finiteShape(c.Common.Offset.X) || !finiteShape(c.Common.Offset.Y) {
		return dst, false, errors.New("local shape offset must be finite")
	}
	closed := true
	switch c.Kind {
	case components.KindBox:
		if !finiteShape(c.Size.X) || !finiteShape(c.Size.Y) || c.Size.X <= 0 || c.Size.Y <= 0 {
			return dst, false, errors.New("box dimensions must be positive and finite")
		}
		hx, hy := c.Size.X*0.5, c.Size.Y*0.5
		dst = append(dst, mathx.Vec2{X: -hx, Y: -hy}, mathx.Vec2{X: hx, Y: -hy}, mathx.Vec2{X: hx, Y: hy}, mathx.Vec2{X: -hx, Y: hy})
	case components.KindCircle:
		if !finiteShape(c.Radius) || c.Radius <= 0 {
			return dst, false, errors.New("circle radius must be positive and finite")
		}
		for i := 0; i < shapeCurveSegments; i++ {
			a := float64(i) * 2 * math.Pi / shapeCurveSegments
			dst = append(dst, mathx.Vec2{X: c.Radius * float32(math.Cos(a)), Y: c.Radius * float32(math.Sin(a))})
		}
	case components.KindCapsule:
		if !finiteShape(c.Radius) || !finiteShape(c.Length) || c.Radius <= 0 || c.Length < 2*c.Radius {
			return dst, false, errors.New("capsule needs a finite positive radius and length at least its diameter")
		}
		half := c.Length*0.5 - c.Radius
		// Physics capsules extend along local X. Two half-circles include the
		// endpoints of the straight top and bottom segments.
		for side := 0; side < 2; side++ {
			for i := 0; i <= shapeCurveSegments/2; i++ {
				a := -math.Pi/2 + float64(side)*math.Pi + float64(i)*2*math.Pi/shapeCurveSegments
				x := half
				if side != 0 {
					x = -half
				}
				dst = append(dst, mathx.Vec2{X: x + c.Radius*float32(math.Cos(a)), Y: c.Radius * float32(math.Sin(a))})
			}
		}
		if half == 0 {
			// A zero straight section is a circle, not two zero-length seams.
			dst = dst[:0]
			for i := 0; i < shapeCurveSegments; i++ {
				a := float64(i) * 2 * math.Pi / shapeCurveSegments
				dst = append(dst, mathx.Vec2{X: c.Radius * float32(math.Cos(a)), Y: c.Radius * float32(math.Sin(a))})
			}
		}
	case components.KindPolygon:
		dst = append(dst, c.Points...)
	case components.KindEdge:
		dst, closed = append(dst, c.Points...), c.Loop
	default:
		return dst, false, errors.New("unknown local shape kind")
	}
	for i := range dst {
		dst[i] = dst[i].Add(c.Common.Offset)
	}
	if err := shapes2d.ValidateContour(dst, closed); err != nil {
		return dst, closed, err
	}
	return dst, closed, nil
}

var _ shapes2d.Provider = (*shapeProvider)(nil)
