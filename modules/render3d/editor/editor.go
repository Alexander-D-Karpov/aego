package editor

import (
	"math"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/editor/ext"
	"aego/modules/render3d"
	"aego/render/debug"
	"aego/scene"
)

func Hook(e *ext.Registry) error {
	if err := e.Bounds("engine.MeshRenderer", MeshShape); err != nil {
		return err
	}
	e.Overlay(frustums)
	if err := e.AssetDrop("mesh", scene.Space3D, dropMesh); err != nil {
		return err
	}
	return e.AssetDrop("texture", scene.Space3D, dropTexture)
}

func MeshShape(s *scene.Scene, res asset.Resolver, h scene.NodeHandle) (shape.Shape, bool) {
	mr, ok := scene.Get[render3d.MeshRenderer](s, h)
	if !ok || res == nil {
		return shape.Shape{}, false
	}
	m, ok := mr.Mesh.Mesh(res)
	if !ok || m == nil {
		return shape.Shape{}, false
	}
	return shape.Shape{Kind: shape.Box, Box: m.Bounds.Transformed(s.World4(h))}, true
}

func frustums(c *shape.OverlayCtx) {
	if !c.Mode3D {
		return
	}
	s := c.Scene
	scene.Query1[render3d.Camera3D](s).Each(func(n scene.NodeHandle, cam *render3d.Camera3D) {
		if s.Space(n) != scene.Space3D || !s.EffectiveEnabled(n) {
			return
		}
		if cam.Orthographic {
			drawOrthoBox(c.Debug, s.World4(n), cam.OrthoSize, c.Aspect, max(cam.Near, 0), cam.OrthoSize, c.Color)
			return
		}
		drawFrustum(c.Debug, s.World4(n), cam.FOV, c.Aspect, 1, c.Color)
	})
}

func drawOrthoBox(dbg *debug.Draw, world mathx.Mat4, size, aspect, near, depth float32, col mathx.Color) {
	hh, hw := size, size*aspect
	xs := [4]float32{-hw, hw, hw, -hw}
	ys := [4]float32{hh, hh, -hh, -hh}
	var front, back [4]mathx.Vec3
	for i := range front {
		front[i] = world.TransformPoint(mathx.Vec3{X: xs[i], Y: ys[i], Z: -near})
		back[i] = world.TransformPoint(mathx.Vec3{X: xs[i], Y: ys[i], Z: -near - depth})
	}
	for i := range front {
		j := (i + 1) % 4
		dbg.Line3(front[i], front[j], col)
		dbg.Line3(back[i], back[j], col)
		dbg.Line3(front[i], back[i], col)
	}
}

func drawFrustum(dbg *debug.Draw, world mathx.Mat4, fovDeg, aspect, depth float32, col mathx.Color) {
	hh := depth * float32(math.Tan(float64(mathx.Deg2Rad(fovDeg))/2))
	hw := hh * aspect
	o := world.Translation()
	corners := [4]mathx.Vec3{
		world.TransformPoint(mathx.Vec3{X: -hw, Y: hh, Z: -depth}),
		world.TransformPoint(mathx.Vec3{X: hw, Y: hh, Z: -depth}),
		world.TransformPoint(mathx.Vec3{X: hw, Y: -hh, Z: -depth}),
		world.TransformPoint(mathx.Vec3{X: -hw, Y: -hh, Z: -depth}),
	}
	for i := range corners {
		dbg.Line3(o, corners[i], col)
		dbg.Line3(corners[i], corners[(i+1)%4], col)
	}
}

func dropMesh(s *scene.Scene, h scene.NodeHandle, id ids.AssetID) {
	mr := scene.Add[render3d.MeshRenderer](s, h)
	*mr = render3d.DefaultMeshRenderer()
	mr.Mesh.SetAssetRefID(id)
}

func dropTexture(s *scene.Scene, h scene.NodeHandle, id ids.AssetID) {
	mr := scene.Add[render3d.MeshRenderer](s, h)
	*mr = render3d.DefaultMeshRenderer()
	mr.Mesh.SetAssetRefID(asset.BuiltinPlane)
	mr.Texture.SetAssetRefID(id)
}
