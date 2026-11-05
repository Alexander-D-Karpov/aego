package view

import (
	"aego/core/ids"
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

type Projection uint8

const (
	Ortho2D Projection = iota
	Perspective3D
	Ortho3D
)

type LayerMask uint64

const AllLayers LayerMask = ^LayerMask(0)

// ID identifies a logical view/output across resize and pooled target replacement, not by label or GPU handle; Scope separates Edit, Preview and Play instances of the same scene nodes.
type ID struct {
	Scope uint64
	Node  ids.NodeID
}

type OutputPolicy uint8

const (
	// OutputDisplay resolves once after all cameras of the logical output.
	OutputDisplay OutputPolicy = iota
	// OutputLinear preserves linear RGB (including emission with zero coverage)
	// in an explicitly supplied RGBA16F target for composition before tone mapping.
	OutputLinear
)

const (
	DefaultFOV       float32 = mathx.Pi / 3
	DefaultNear      float32 = 0.1
	DefaultFar       float32 = 1000
	DefaultOrthoSize float32 = 5
)

type View struct {
	ID                ID
	OutputID          ID
	OutputPolicy      OutputPolicy
	DeferResolve      bool
	Projection        Projection
	Camera            Camera2D
	Eye               mathx.Mat4
	FOV               float32
	OrthoSize         float32
	Near, Far         float32
	Viewport          mathx.Rect
	Target            gpu.TextureHandle
	DepthTarget       gpu.TextureHandle
	TargetSubresource gpu.TextureSubresource
	DepthSubresource  gpu.TextureSubresource
	TargetW           int
	TargetH           int
	PixelScale        float32
	PixelsPerUnit     float32
	ZMin, ZMax        float32
	Clear             mathx.Color
	DoClear           bool
	// ClearWholeTarget includes unused storage outside the drawing viewport.
	// Use it only for a presentation surface that owns its entire attachment.
	ClearWholeTarget bool
	Layers           LayerMask
	Scale            ScaleMode
	Virtual          mathx.Vec2
	Label            string
}

// ClearRegion is independent of projection: a pooled surface may clear its
// padded attachment without stretching the visible world to fill that storage.
func (v *View) ClearRegion() mathx.Rect {
	if v.ClearWholeTarget {
		return mathx.Rect{W: float32(v.TargetW), H: float32(v.TargetH)}
	}
	return v.Viewport
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

func PerspectiveView(eye mathx.Mat4, fov, near, far float32, vp mathx.Rect) View {
	return View{
		Projection:    Perspective3D,
		Camera:        Camera2D{Zoom: 1},
		Eye:           eye,
		FOV:           fov,
		Near:          near,
		Far:           far,
		Viewport:      vp,
		PixelScale:    1,
		PixelsPerUnit: 1,
		ZMax:          1,
		Layers:        AllLayers,
		Label:         "perspective",
	}
}

func OrthoView(eye mathx.Mat4, size, near, far float32, vp mathx.Rect) View {
	v := PerspectiveView(eye, 0, near, far, vp)
	v.Projection = Ortho3D
	v.OrthoSize = size
	v.Label = "ortho"
	return v
}

func (v View) Is3D() bool {
	return v.Projection == Perspective3D || v.Projection == Ortho3D
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
	if !v.Is3D() {
		return
	}
	if v.Eye == (mathx.Mat4{}) {
		v.Eye = mathx.Mat4Identity()
	}
	if v.Projection == Ortho3D {
		if v.OrthoSize <= 0 {
			v.OrthoSize = DefaultOrthoSize
		}
		if v.Far <= v.Near {
			v.Near, v.Far = DefaultNear, DefaultFar
		}
		return
	}
	if v.FOV <= 0 {
		v.FOV = DefaultFOV
	}
	if v.Near <= 0 {
		v.Near = DefaultNear
	}
	if v.Far <= v.Near {
		v.Far = max(DefaultFar, v.Near*2)
	}
}

func (v View) UsesDepth() bool {
	return v.Is3D() && (v.Target.IsZero() || !v.DepthTarget.IsZero())
}

func (v View) Flipped(caps gpu.Capabilities) bool {
	return !v.Target.IsZero() && caps.OriginBottomLeft
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
	if v.Is3D() || v.Scale == ScaleFree || v.Virtual.X <= 0 || v.Virtual.Y <= 0 {
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

func (v View) CameraPosition() mathx.Vec3 {
	if v.Is3D() {
		return v.Eye.Translation()
	}
	p := v.cameraPosition()
	return mathx.Vec3{X: p.X, Y: p.Y}
}

func (v View) ViewMatrix() mathx.Mat3 {
	z := v.EffectiveZoom()
	pos := v.cameraPosition()
	translate := mathx.Mat3TRS(pos.Neg(), 0, mathx.Vec2{X: 1, Y: 1})
	rotate := mathx.Mat3TRS(mathx.Vec2{}, -v.Camera.Rotation, mathx.Vec2{X: z, Y: z})
	return rotate.Mul(translate)
}

func (v View) ViewMatrix4() mathx.Mat4 {
	if v.Is3D() {
		if inv, ok := v.Eye.InverseAffine(); ok {
			return inv
		}
		return mathx.Mat4Identity()
	}
	return mathx.Mat4FromMat3(v.ViewMatrix())
}

func (v View) glProjection() mathx.Mat4 {
	vp := v.LetterboxViewport()
	aspect := float32(1)
	if vp.H > 0 {
		aspect = vp.W / vp.H
	}
	switch v.Projection {
	case Perspective3D:
		return mathx.Mat4Perspective(v.FOV, aspect, v.Near, v.Far)
	case Ortho3D:
		hh := v.OrthoSize
		hw := hh * aspect
		return mathx.Mat4Ortho(-hw, hw, -hh, hh, v.Near, v.Far)
	}
	hw, hh := vp.W/2, vp.H/2
	return mathx.Mat4Ortho(-hw, hw, hh, -hh, -1, 1)
}

func (v View) ProjectionMatrix(caps gpu.Capabilities) mathx.Mat4 {
	p := v.glProjection()
	if v.Flipped(caps) {
		flip := mathx.Mat4Identity()
		flip[5] = -1
		p = flip.Mul(p)
	}
	if caps.ClipZeroToOne {
		fix := mathx.Mat4Identity()
		fix[10] = 0.5
		fix[14] = 0.5
		p = fix.Mul(p)
	}
	return p
}

func (v View) ViewProjection(caps gpu.Capabilities) mathx.Mat4 {
	return v.ProjectionMatrix(caps).Mul(v.ViewMatrix4())
}

func (v View) ViewProj(caps gpu.Capabilities) mathx.Mat4 {
	return v.ViewProjection(caps)
}

func (v View) clipFromWorld() mathx.Mat4 {
	return v.glProjection().Mul(v.ViewMatrix4())
}

func (v View) Frustum() mathx.Frustum {
	return mathx.FrustumFromMat4(v.clipFromWorld())
}

func (v View) ScreenToRay(p mathx.Vec2) mathx.Ray3 {
	vp := v.LetterboxViewport()
	if vp.W <= 0 || vp.H <= 0 {
		return mathx.Ray3{}
	}
	nx := (p.X-vp.X)/vp.W*2 - 1
	ny := 1 - (p.Y-vp.Y)/vp.H*2
	inv, ok := v.clipFromWorld().Inverse()
	if !ok {
		return mathx.Ray3{}
	}
	a := inv.TransformPoint(mathx.Vec3{X: nx, Y: ny, Z: -1})
	b := inv.TransformPoint(mathx.Vec3{X: nx, Y: ny, Z: 1})
	return mathx.Ray3{Origin: a, Dir: b.Sub(a).Normalized()}
}

func (v View) WorldToScreen3(p mathx.Vec3) (mathx.Vec2, bool) {
	c := v.clipFromWorld().MulVec4(mathx.Vec4{X: p.X, Y: p.Y, Z: p.Z, W: 1})
	if c.W <= 1e-6 {
		return mathx.Vec2{}, false
	}
	vp := v.LetterboxViewport()
	nx, ny := c.X/c.W, c.Y/c.W
	return mathx.Vec2{X: vp.X + (nx+1)/2*vp.W, Y: vp.Y + (1-ny)/2*vp.H}, true
}

func (v View) WorldToScreen(p mathx.Vec2) mathx.Vec2 {
	vp := v.LetterboxViewport()
	local := v.ViewMatrix().Apply(p)
	return mathx.Vec2{X: vp.X + vp.W/2 + local.X, Y: vp.Y + vp.H/2 + local.Y}
}

func (v View) ScreenToWorld(p mathx.Vec2) mathx.Vec2 {
	vp := v.LetterboxViewport()
	local := mathx.Vec2{X: p.X - vp.X - vp.W/2, Y: p.Y - vp.Y - vp.H/2}
	inv, ok := v.ViewMatrix().Inverse()
	if !ok {
		return v.cameraPosition()
	}
	return inv.Apply(local)
}

func (v View) VisibleBounds() mathx.AABB2 {
	if v.Is3D() {
		inf := mathx.Inf()
		return mathx.AABB2{Min: mathx.Vec2{X: -inf, Y: -inf}, Max: mathx.Vec2{X: inf, Y: inf}}
	}
	vp := v.LetterboxViewport()
	c0 := v.ScreenToWorld(mathx.Vec2{X: vp.X, Y: vp.Y})
	c1 := v.ScreenToWorld(mathx.Vec2{X: vp.X + vp.W, Y: vp.Y})
	c2 := v.ScreenToWorld(mathx.Vec2{X: vp.X, Y: vp.Y + vp.H})
	c3 := v.ScreenToWorld(mathx.Vec2{X: vp.X + vp.W, Y: vp.Y + vp.H})
	return mathx.AABB2{
		Min: mathx.Vec2{X: min(min(c0.X, c1.X), min(c2.X, c3.X)), Y: min(min(c0.Y, c1.Y), min(c2.Y, c3.Y))},
		Max: mathx.Vec2{X: max(max(c0.X, c1.X), max(c2.X, c3.X)), Y: max(max(c0.Y, c1.Y), max(c2.Y, c3.Y))},
	}
}
