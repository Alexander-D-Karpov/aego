package sceneview

import (
	"math"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/view"
	"aego/scene"
	"aego/scene/components"
)

func visible(s *scene.Scene, h scene.NodeHandle, mask uint64) bool {
	return h != s.Root() && s.EffectiveEnabled(h) && s.Layers(h)&mask != 0
}

func MeshBounds(s *scene.Scene, res asset.Resolver, h scene.NodeHandle, mr *components.MeshRenderer) (mathx.AABB3, bool) {
	m, ok := mr.Mesh.Mesh(res)
	if !ok || m == nil {
		return mathx.AABB3{}, false
	}
	return m.Bounds.Transformed(s.World4(h)), true
}

func NodeBounds(s *scene.Scene, res asset.Resolver, h scene.NodeHandle) (mathx.AABB3, bool) {
	if !s.IsAlive(h) {
		return mathx.AABB3{}, false
	}
	if mr, ok := scene.Get[components.MeshRenderer](s, h); ok {
		if b, ok := MeshBounds(s, res, h, mr); ok {
			return b, true
		}
	}
	p := s.WorldPosition3D(h)
	e := mathx.Vec3{X: 0.5, Y: 0.5, Z: 0.5}
	return mathx.AABB3{Min: p.Sub(e), Max: p.Add(e)}, true
}

func union(a, b mathx.AABB3) mathx.AABB3 {
	return mathx.AABB3{Min: mathx.Vec3Min(a.Min, b.Min), Max: mathx.Vec3Max(a.Max, b.Max)}
}

func SelectionBounds(s *scene.Scene, res asset.Resolver, nodes []ids.NodeID) (mathx.AABB3, bool) {
	var out mathx.AABB3
	found := false
	for _, id := range nodes {
		b, ok := NodeBounds(s, res, s.Handle(id))
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

func SceneBounds(s *scene.Scene, res asset.Resolver) (mathx.AABB3, bool) {
	var out mathx.AABB3
	found := false
	scene.Query1[components.MeshRenderer](s).Each(func(h scene.NodeHandle, mr *components.MeshRenderer) {
		if !visible(s, h, ^uint64(0)) || s.Space(h) != scene.Space3D {
			return
		}
		b, ok := MeshBounds(s, res, h, mr)
		if !ok {
			return
		}
		if found {
			out = union(out, b)
		} else {
			out, found = b, true
		}
	})
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

func Pick3D(s *scene.Scene, res asset.Resolver, ray mathx.Ray3, mask uint64) (scene.NodeHandle, float32, bool) {
	s.UpdateTransforms()
	best := scene.NodeHandle{}
	bestT := float32(math.MaxFloat32)
	consider := func(h scene.NodeHandle, t float32) {
		if t >= 0 && t < bestT {
			best, bestT = h, t
		}
	}
	scene.Query1[components.MeshRenderer](s).Each(func(h scene.NodeHandle, mr *components.MeshRenderer) {
		if !visible(s, h, mask) || s.Space(h) != scene.Space3D {
			return
		}
		b, ok := MeshBounds(s, res, h, mr)
		if !ok {
			return
		}
		if t, hit := ray.IntersectAABB(b); hit {
			consider(h, t)
		}
	})
	scene.Query1[components.SpriteRenderer](s).Each(func(h scene.NodeHandle, sr *components.SpriteRenderer) {
		if !visible(s, h, mask) || s.Space(h) != scene.Space2D {
			return
		}
		if t, hit := pickSprite(s, res, h, sr, ray); hit {
			consider(h, t)
		}
	})
	return best, bestT, !best.IsZero()
}

func pickSprite(s *scene.Scene, res asset.Resolver, h scene.NodeHandle, sr *components.SpriteRenderer, ray mathx.Ray3) (float32, bool) {
	region, ok := sr.Sprite.Region(res)
	if !ok {
		return 0, false
	}
	size := sr.Size
	if size.X == 0 && size.Y == 0 {
		size = region.Size()
	}
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
	x0, y0 := -sr.Origin.X*size.X, -sr.Origin.Y*size.Y
	if x < x0 || x > x0+size.X || y < y0 || y > y0+size.Y {
		return 0, false
	}
	return t, true
}

func PickPoint3D(s *scene.Scene, ray mathx.Ray3, mask uint64, radius func(mathx.Vec3) float32) (scene.NodeHandle, float32, bool) {
	s.UpdateTransforms()
	best := scene.NodeHandle{}
	bestT := float32(math.MaxFloat32)
	dd := ray.Dir.Dot(ray.Dir)
	if dd == 0 {
		return best, 0, false
	}
	for _, h := range s.PreOrder() {
		if !visible(s, h, mask) || s.Space(h) != scene.Space3D {
			continue
		}
		if _, ok := scene.Get[components.MeshRenderer](s, h); ok {
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

func BoxSelect3D(s *scene.Scene, v view.View, a, b mathx.Vec2, mask uint64) []ids.NodeID {
	s.UpdateTransforms()
	lo := mathx.Vec2{X: min(a.X, b.X), Y: min(a.Y, b.Y)}
	hi := mathx.Vec2{X: max(a.X, b.X), Y: max(a.Y, b.Y)}
	var out []ids.NodeID
	for _, h := range s.PreOrder() {
		if !visible(s, h, mask) {
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
