package view

import (
	"aego/core/mathx"
	"aego/gpu"
)

type Camera2D struct {
	Position mathx.Vec2
	Rotation float32
	Zoom     float32
}

type ScaleMode uint8

const (
	ScaleFree ScaleMode = iota
	ScalePixelPerfect
	ScaleFitVirtual
)

type LayerMask uint32

const AllLayers LayerMask = 0xffffffff

type View struct {
	Camera        Camera2D
	Viewport      mathx.Rect
	Target        gpu.TextureHandle
	TargetW       int
	TargetH       int
	PixelScale    float32
	PixelsPerUnit float32
	ZMin, ZMax    float32
	Clear         mathx.Color
	DoClear       bool
	Layers        LayerMask
	Scale         ScaleMode
	Virtual       mathx.Vec2
	Label         string
}

func ScreenView(pixelW, pixelH int, pixelScale float32) View {
	if pixelScale <= 0 {
		pixelScale = 1
	}
	lw := float32(pixelW) / pixelScale
	lh := float32(pixelH) / pixelScale
	return View{
		Camera:        Camera2D{Position: mathx.Vec2{X: lw / 2, Y: lh / 2}, Zoom: pixelScale},
		Viewport:      mathx.Rect{W: float32(pixelW), H: float32(pixelH)},
		TargetW:       pixelW,
		TargetH:       pixelH,
		PixelScale:    pixelScale,
		PixelsPerUnit: 1,
		ZMax:          1,
		Layers:        AllLayers,
		Label:         "screen",
	}
}

func (v *View) Normalize(pixelW, pixelH int, pixelScale float32) {
	if v.TargetW <= 0 || v.TargetH <= 0 {
		v.TargetW, v.TargetH = pixelW, pixelH
	}
	if v.Viewport.W <= 0 || v.Viewport.H <= 0 {
		v.Viewport = mathx.Rect{W: float32(v.TargetW), H: float32(v.TargetH)}
	}
	if v.PixelScale <= 0 {
		v.PixelScale = pixelScale
	}
	if v.PixelScale <= 0 {
		v.PixelScale = 1
	}
	if v.PixelsPerUnit <= 0 {
		v.PixelsPerUnit = 1
	}
	if v.Camera.Zoom <= 0 {
		v.Camera.Zoom = 1
	}
	if v.Layers == 0 {
		v.Layers = AllLayers
	}
	if v.ZMax <= v.ZMin {
		v.ZMin, v.ZMax = 0, 1
	}
}

func (v View) EffectiveZoom() float32 {
	z := v.Camera.Zoom
	if z <= 0 {
		z = 1
	}
	switch v.Scale {
	case ScaleFitVirtual:
		return z * v.virtualFit()
	case ScalePixelPerfect:
		f := float32(int(v.virtualFit()))
		if f < 1 {
			f = 1
		}
		return z * f
	}
	return z
}

func (v View) virtualFit() float32 {
	if v.Virtual.X <= 0 || v.Virtual.Y <= 0 {
		return 1
	}
	return min(v.Viewport.W/v.Virtual.X, v.Viewport.H/v.Virtual.Y)
}

func (v View) LetterboxViewport() mathx.Rect {
	if v.Scale == ScaleFree || v.Virtual.X <= 0 || v.Virtual.Y <= 0 {
		return v.Viewport
	}
	zoom := v.EffectiveZoom() / max(v.Camera.Zoom, 0.0001)
	w := v.Virtual.X * zoom
	h := v.Virtual.Y * zoom
	return mathx.Rect{
		X: v.Viewport.X + (v.Viewport.W-w)/2,
		Y: v.Viewport.Y + (v.Viewport.H-h)/2,
		W: w,
		H: h,
	}
}

func (v View) cameraPosition() mathx.Vec2 {
	p := v.Camera.Position
	if v.Scale != ScalePixelPerfect {
		return p
	}
	z := v.EffectiveZoom()
	if z <= 0 {
		return p
	}
	return mathx.Vec2{
		X: float32(int(p.X*z+0.5)) / z,
		Y: float32(int(p.Y*z+0.5)) / z,
	}
}

func (v View) ViewMatrix() mathx.Mat3 {
	z := v.EffectiveZoom()
	pos := v.cameraPosition()
	translate := mathx.Mat3TRS(pos.Neg(), 0, mathx.Vec2{X: 1, Y: 1})
	rotate := mathx.Mat3TRS(mathx.Vec2{}, -v.Camera.Rotation, mathx.Vec2{X: z, Y: z})
	return rotate.Mul(translate)
}

func (v View) Projection(caps gpu.Capabilities) mathx.Mat4 {
	vp := v.LetterboxViewport()
	hw := vp.W / 2
	hh := vp.H / 2
	proj := mathx.Mat4Ortho(-hw, hw, hh, -hh, -1, 1)
	if !v.Target.IsZero() && caps.OriginBottomLeft {
		flip := mathx.Mat4Identity()
		flip[5] = -1
		proj = flip.Mul(proj)
	}
	return proj
}

func (v View) ViewProjection(caps gpu.Capabilities) mathx.Mat4 {
	return v.Projection(caps).Mul(mathx.Mat4FromMat3(v.ViewMatrix()))
}

func (v View) WorldToScreen(p mathx.Vec2) mathx.Vec2 {
	vp := v.LetterboxViewport()
	local := v.ViewMatrix().Apply(p)
	return mathx.Vec2{X: vp.X + vp.W/2 + local.X, Y: vp.Y + vp.H/2 + local.Y}
}

func (v View) ScreenToWorld(p mathx.Vec2) mathx.Vec2 {
	vp := v.LetterboxViewport()
	local := mathx.Vec2{X: p.X - vp.X - vp.W/2, Y: p.Y - vp.Y - vp.H/2}
	return v.ViewMatrix().Inverse().Apply(local)
}

func (v View) VisibleBounds() mathx.AABB {
	vp := v.LetterboxViewport()
	c0 := v.ScreenToWorld(mathx.Vec2{X: vp.X, Y: vp.Y})
	c1 := v.ScreenToWorld(mathx.Vec2{X: vp.X + vp.W, Y: vp.Y})
	c2 := v.ScreenToWorld(mathx.Vec2{X: vp.X, Y: vp.Y + vp.H})
	c3 := v.ScreenToWorld(mathx.Vec2{X: vp.X + vp.W, Y: vp.Y + vp.H})
	return mathx.AABB{
		Min: mathx.Vec2{X: min(min(c0.X, c1.X), min(c2.X, c3.X)), Y: min(min(c0.Y, c1.Y), min(c2.Y, c3.Y))},
		Max: mathx.Vec2{X: max(max(c0.X, c1.X), max(c2.X, c3.X)), Y: max(max(c0.Y, c1.Y), max(c2.Y, c3.Y))},
	}
}
