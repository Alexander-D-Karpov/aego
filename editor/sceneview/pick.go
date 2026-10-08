package sceneview

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/scene"
	"aego/scene/components"
)

func spriteRect(sr *components.SpriteRenderer, res asset.Resolver) (mathx.Rect, bool) {
	size := sr.Size
	if size.X == 0 && size.Y == 0 {
		if res == nil {
			return mathx.Rect{}, false
		}
		reg, ok := sr.Sprite.Region(res)
		if !ok {
			return mathx.Rect{}, false
		}
		size = reg.Size()
	}
	return mathx.Rect{X: -sr.Origin.X * size.X, Y: -sr.Origin.Y * size.Y, W: size.X, H: size.Y}, true
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

func Quad(s *scene.Scene, res asset.Resolver, h scene.NodeHandle) ([4]mathx.Vec2, bool) {
	sr, ok := scene.Get[components.SpriteRenderer](s, h)
	if !ok {
		return [4]mathx.Vec2{}, false
	}
	lr, ok := spriteRect(sr, res)
	if !ok {
		return [4]mathx.Vec2{}, false
	}
	return quadOf(s, h, lr), true
}

func Bounds(s *scene.Scene, res asset.Resolver, h scene.NodeHandle) (mathx.AABB2, bool) {
	q, ok := Quad(s, res, h)
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

func Pick(s *scene.Scene, res asset.Resolver, p mathx.Vec2) (scene.NodeHandle, bool) {
	s.UpdateTransforms()
	var best scene.NodeHandle
	var bestKey int64
	found := false
	idx := int64(0)
	scene.Query1[components.SpriteRenderer](s).Each(func(h scene.NodeHandle, sr *components.SpriteRenderer) {
		idx++
		lr, ok := spriteRect(sr, res)
		if !ok {
			return
		}
		inv, ok := s.World(h).Inverse()
		if !ok {
			return
		}
		local := inv.Apply(p)
		if local.X < lr.X || local.X > lr.X+lr.W || local.Y < lr.Y || local.Y > lr.Y+lr.H {
			return
		}
		key := (int64(sr.Layer)+1<<15)<<40 | (int64(sr.Order)+1<<15)<<24 | idx
		if !found || key > bestKey {
			best, bestKey, found = h, key, true
		}
	})
	return best, found
}

func BoxSelect(s *scene.Scene, res asset.Resolver, box mathx.AABB2) []ids.NodeID {
	s.UpdateTransforms()
	root := s.Root()
	var out []ids.NodeID
	for _, h := range s.PreOrder() {
		if h == root || !s.IsAlive(h) || !s.EffectiveEnabled(h) {
			continue
		}
		b, isSprite := Bounds(s, res, h)
		if isSprite && b.Intersects(box) || !isSprite && box.Contains(s.WorldPosition(h)) {
			out = append(out, s.ID(h))
		}
	}
	return out
}
