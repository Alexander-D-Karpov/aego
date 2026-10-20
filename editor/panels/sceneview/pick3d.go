package sceneview

import (
	"math"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/render/view"
	"aego/scene"
)

func visible(s *scene.Scene, h scene.NodeHandle, mask uint64) bool {
	return h != s.Root() && s.EffectiveEnabled(h) && s.Layers(h)&mask != 0
}

func NodeBounds(s *scene.Scene, sh shape.Set, h scene.NodeHandle) (mathx.AABB3, bool) {
	if !s.IsAlive(h) {
		return mathx.AABB3{}, false
	}
	if b, ok := sh.Of(s, h); ok && b.Kind == shape.Box {
		return b.Box, true
	}
	p := s.WorldPosition3D(h)
	e := mathx.Vec3{X: 0.5, Y: 0.5, Z: 0.5}
	return mathx.AABB3{Min: p.Sub(e), Max: p.Add(e)}, true
}

func union(a, b mathx.AABB3) mathx.AABB3 {
	return mathx.AABB3{Min: mathx.Vec3Min(a.Min, b.Min), Max: mathx.Vec3Max(a.Max, b.Max)}
}

func SelectionBounds(s *scene.Scene, sh shape.Set, nodes []ids.NodeID) (mathx.AABB3, bool) {
	var out mathx.AABB3
	found := false
	for _, id := range nodes {
		b, ok := NodeBounds(s, sh, s.Handle(id))
		if !ok {
			continue
		}
		if found {
			out = union(out, b)
		} else {
			out, found = b, true
		}
	}
	return out, found
}

func SceneBounds(s *scene.Scene, sh shape.Set) (mathx.AABB3, bool) {
	var out mathx.AABB3
	found := false
	for _, h := range s.PreOrder() {
		if !visible(s, h, ^uint64(0)) || s.Space(h) != scene.Space3D {
			continue
		}
		b, ok := sh.Of(s, h)
		if !ok || b.Kind != shape.Box {
			continue
		}
		if found {
			out = union(out, b.Box)
		} else {
			out, found = b.Box, true
		}
	}
	return out, found
}

func intersectPlane(r mathx.Ray3, point, normal mathx.Vec3) (float32, bool) {
	den := normal.Dot(r.Dir)
	if mathx.Abs(den) < 1e-6 {
		return 0, false
	}
	t := normal.Dot(point.Sub(r.Origin)) / den
	return t, t >= 0
}

func Pick3D(s *scene.Scene, sh shape.Set, ray mathx.Ray3, mask uint64) (scene.NodeHandle, float32, bool) {
	s.UpdateTransforms()
	best := scene.NodeHandle{}
	bestT := float32(math.MaxFloat32)
	for _, h := range s.PreOrder() {
		if !visible(s, h, mask) {
			continue
		}
		b, ok := sh.Of(s, h)
		if !ok {
			continue
		}
		var t float32
		hit := false
		switch {
		case b.Kind == shape.Box && s.Space(h) == scene.Space3D:
			t, hit = ray.IntersectAABB(b.Box)
		case b.Kind == shape.Rect && s.Space(h) == scene.Space2D:
			t, hit = pickRect(s, h, b.Local, ray)
		}
		if hit && t >= 0 && t < bestT {
			best, bestT = h, t
		}
	}
	return best, bestT, !best.IsZero()
}

func pickRect(s *scene.Scene, h scene.NodeHandle, lr mathx.Rect, ray mathx.Ray3) (float32, bool) {
	inv, ok := s.World4(h).InverseAffine()
	if !ok {
		return 0, false
	}
	o := inv.TransformPoint(ray.Origin)
	d := inv.TransformDir(ray.Dir)
	if mathx.Abs(d.Z) < 1e-8 {
		return 0, false
	}
	t := -o.Z / d.Z
	if t < 0 {
		return 0, false
	}
	p := o.Add(d.Scale(t))
	x, y := p.X, -p.Y
	if x < lr.X || x > lr.X+lr.W || y < lr.Y || y > lr.Y+lr.H {
		return 0, false
	}
	return t, true
}

func PickPoint3D(s *scene.Scene, sh shape.Set, ray mathx.Ray3, mask uint64, radius func(mathx.Vec3) float32) (scene.NodeHandle, float32, bool) {
	s.UpdateTransforms()
	best := scene.NodeHandle{}
	bestT := float32(math.MaxFloat32)
	dd := ray.Dir.Dot(ray.Dir)
	if dd == 0 {
		return best, 0, false
	}
	for _, h := range s.PreOrder() {
		if !visible(s, h, mask) || s.Space(h) != scene.Space3D || sh.Hidden(s, h) {
			continue
		}
		if b, ok := sh.Of(s, h); ok && b.Kind == shape.Box {
			continue
		}
		p := s.WorldPosition3D(h)
		t := p.Sub(ray.Origin).Dot(ray.Dir) / dd
		if t < 0 || t >= bestT {
			continue
		}
		if ray.At(t).Sub(p).Len() <= radius(p) {
			best, bestT = h, t
		}
	}
	return best, bestT, !best.IsZero()
}

func BoxSelect3D(s *scene.Scene, sh shape.Set, v view.View, a, b mathx.Vec2, mask uint64) []ids.NodeID {
	s.UpdateTransforms()
	lo := mathx.Vec2{X: min(a.X, b.X), Y: min(a.Y, b.Y)}
	hi := mathx.Vec2{X: max(a.X, b.X), Y: max(a.Y, b.Y)}
	var out []ids.NodeID
	for _, h := range s.PreOrder() {
		if !visible(s, h, mask) || sh.Hidden(s, h) {
			continue
		}
		p, ok := v.WorldToScreen3(s.WorldPosition3D(h))
		if !ok || p.X < lo.X || p.X > hi.X || p.Y < lo.Y || p.Y > hi.Y {
			continue
		}
		out = append(out, s.ID(h))
	}
	return out
}
