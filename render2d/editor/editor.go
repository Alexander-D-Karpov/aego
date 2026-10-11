package editor

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/extension"
	"aego/editor/shape"
	"aego/render2d"
	"aego/scene"
)

const PixelsPerUnit = 32

func Hook(e *extension.Registry) error {
	if err := e.Bounds("engine.SpriteRenderer", SpriteShape); err != nil {
		return err
	}
	e.Overlay(cameraFrames)
	return e.AssetDrop("texture", scene.Space2D, dropTexture)
}

func Rect(sr *render2d.SpriteRenderer, res asset.Resolver) (mathx.Rect, bool) {
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

func SpriteShape(s *scene.Scene, res asset.Resolver, h scene.NodeHandle) (shape.Shape, bool) {
	sr, ok := scene.Get[render2d.SpriteRenderer](s, h)
	if !ok {
		return shape.Shape{}, false
	}
	lr, ok := Rect(sr, res)
	if !ok {
		return shape.Shape{}, false
	}
	order := (int64(sr.Layer)+1<<15)<<16 | (int64(sr.Order) + 1<<15)
	return shape.Shape{Kind: shape.Rect, Local: lr, Order: order}, true
}

func cameraFrames(c *shape.OverlayCtx) {
	if c.Mode3D {
		return
	}
	s := c.Scene
	scene.Query1[render2d.Camera2D](s).Each(func(h scene.NodeHandle, cam *render2d.Camera2D) {
		if cam.Zoom <= 0 {
			return
		}
		m := s.World(h)
		pos := mathx.Vec2{X: m[6], Y: m[7]}
		size := mathx.Vec2{X: c.Window.X / cam.Zoom, Y: c.Window.Y / cam.Zoom}
		c.Debug.RectRot(pos, size, mathx.Atan2(m[1], m[0]), c.Color)
		c.Debug.Cross(pos, c.Px*10, c.Color)
	})
}

func dropTexture(s *scene.Scene, h scene.NodeHandle, id ids.AssetID) {
	sr := scene.Add[render2d.SpriteRenderer](s, h)
	*sr = render2d.DefaultSpriteRenderer()
	sr.Sprite.SetAssetRefID(id)
	sr.Size = mathx.Vec2{X: 1, Y: 1}
	if res := s.Resolver(); res != nil && !id.IsZero() {
		if reg, ok := res.Texture(id); ok && reg.W > 0 && reg.H > 0 {
			sr.Size = mathx.Vec2{X: float32(reg.W) / PixelsPerUnit, Y: float32(reg.H) / PixelsPerUnit}
		}
	}
}
