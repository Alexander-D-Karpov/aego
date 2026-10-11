package sceneview

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/shape"
	"aego/scene"
)

func shapes(ctx *context.Editor) shape.Set {
	return shape.Set{Reg: ctx.Shapes, Res: resolver(ctx)}
}

func quadOf(s *scene.Scene, h scene.NodeHandle, lr mathx.Rect) [4]mathx.Vec2 {
	m := s.World(h)
	return [4]mathx.Vec2{
		m.Apply(mathx.Vec2{X: lr.X, Y: lr.Y}),
		m.Apply(mathx.Vec2{X: lr.X + lr.W, Y: lr.Y}),
		m.Apply(mathx.Vec2{X: lr.X + lr.W, Y: lr.Y + lr.H}),
		m.Apply(mathx.Vec2{X: lr.X, Y: lr.Y + lr.H}),
	}
}

func Quad(s *scene.Scene, sh shape.Set, h scene.NodeHandle) ([4]mathx.Vec2, bool) {
	r, ok := sh.Of(s, h)
	if !ok || r.Kind != shape.Rect {
		return [4]mathx.Vec2{}, false
	}
	return quadOf(s, h, r.Local), true
}

func Bounds(s *scene.Scene, sh shape.Set, h scene.NodeHandle) (mathx.AABB2, bool) {
	q, ok := Quad(s, sh, h)
	if !ok {
		p := s.WorldPosition(h)
		return mathx.AABB2{Min: p.Sub(mathx.Vec2{X: 0.5, Y: 0.5}), Max: p.Add(mathx.Vec2{X: 0.5, Y: 0.5})}, false
	}
	b := mathx.AABB2{Min: q[0], Max: q[0]}
	for _, v := range q[1:] {
		b = b.Union(mathx.AABB2{Min: v, Max: v})
	}
	return b, true
}

func Pick(s *scene.Scene, sh shape.Set, p mathx.Vec2) (scene.NodeHandle, bool) {
	s.UpdateTransforms()
	var best scene.NodeHandle
	var bestKey int64
	found := false
	idx := int64(0)
	root := s.Root()
	for _, h := range s.PreOrder() {
		if h == root || !s.IsAlive(h) || !s.EffectiveEnabled(h) {
			continue
		}
		r, ok := sh.Of(s, h)
		if !ok || r.Kind != shape.Rect {
			continue
		}
		idx++
		inv, ok := s.World(h).Inverse()
		if !ok {
			continue
		}
		local := inv.Apply(p)
		lr := r.Local
		if local.X < lr.X || local.X > lr.X+lr.W || local.Y < lr.Y || local.Y > lr.Y+lr.H {
			continue
		}
		key := r.Order<<24 | idx
		if !found || key > bestKey {
			best, bestKey, found = h, key, true
		}
	}
	return best, found
}

func BoxSelect(s *scene.Scene, sh shape.Set, box mathx.AABB2) []ids.NodeID {
	s.UpdateTransforms()
	root := s.Root()
	var out []ids.NodeID
	for _, h := range s.PreOrder() {
		if h == root || !s.IsAlive(h) || !s.EffectiveEnabled(h) {
			continue
		}
		b, isShape := Bounds(s, sh, h)
		if isShape && b.Intersects(box) || !isShape && box.Contains(s.WorldPosition(h)) {
			out = append(out, s.ID(h))
		}
	}
	return out
}
