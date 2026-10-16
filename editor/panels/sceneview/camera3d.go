package sceneview

import (
	"math"

	"aego/core/mathx"
	"aego/render/view"
)

type Mode uint8

const (
	Mode2D Mode = iota
	Mode3D
)

func (m Mode) String() string {
	if m == Mode3D {
		return "3D"
	}
	return "2D"
}

const (
	orbitSpeed  float32 = 0.006
	minDistance float32 = 0.05
	maxDistance float32 = 1e5
	maxPitch    float32 = 1.55
	dollyBase           = 0.85
)

type Camera3D struct {
	Target   mathx.Vec3
	Distance float32
	Yaw      float32
	Pitch    float32
	FOV      float32
	Ortho    bool
}

func DefaultCamera3D() Camera3D {
	return Camera3D{Distance: 10, Yaw: 0.6, Pitch: -0.45, FOV: 60}
}

func (c Camera3D) dist() float32 {
	if c.Distance < minDistance {
		return minDistance
	}
	return c.Distance
}

func (c Camera3D) fov() float32 {
	f := c.FOV
	if f <= 1 || f >= 179 {
		f = 60
	}
	return mathx.Deg2Rad(f)
}

func (c Camera3D) Rotation() mathx.Quat {
	return mathx.QuatYawPitch(c.Yaw, c.Pitch)
}

func (c Camera3D) Position() mathx.Vec3 {
	return c.Target.Add(c.Rotation().Rotate(mathx.Vec3{Z: c.dist()}))
}

func (c Camera3D) Forward() mathx.Vec3 {
	return c.Rotation().Rotate(mathx.Vec3{Z: -1})
}

func (c Camera3D) Right() mathx.Vec3 {
	return c.Rotation().Rotate(mathx.Vec3{X: 1})
}

func (c Camera3D) Up() mathx.Vec3 {
	return c.Rotation().Rotate(mathx.Vec3{Y: 1})
}

func (c Camera3D) Eye() mathx.Mat4 {
	return mathx.Mat4TRS(c.Position(), c.Rotation(), mathx.Vec3{X: 1, Y: 1, Z: 1})
}

func (c Camera3D) clip() (float32, float32) {
	d := c.dist()
	return mathx.Clamp(d*0.002, 0.01, 1), max(d*50, 1000)
}

func (c Camera3D) halfHeight() float32 {
	return c.dist() * float32(math.Tan(float64(c.fov())/2))
}

func (c Camera3D) View(vp mathx.Rect) view.View {
	near, far := c.clip()
	if c.Ortho {
		return view.OrthoView(c.Eye(), c.halfHeight(), -far, far, vp)
	}
	return view.PerspectiveView(c.Eye(), c.fov(), near, far, vp)
}

func (c Camera3D) WorldPerPixel(h float32) float32 {
	if h <= 0 {
		h = 1
	}
	return 2 * c.halfHeight() / h
}

func (c *Camera3D) Orbit(dx, dy float32) {
	c.Yaw -= dx * orbitSpeed
	c.Pitch = mathx.Clamp(c.Pitch-dy*orbitSpeed, -maxPitch, maxPitch)
}

func (c *Camera3D) Pan(dx, dy, viewportH float32) {
	w := c.WorldPerPixel(viewportH)
	c.Target = c.Target.Sub(c.Right().Scale(dx * w)).Add(c.Up().Scale(dy * w))
}

func (c *Camera3D) Dolly(steps float32) {
	c.Distance = mathx.Clamp(c.dist()*float32(math.Pow(dollyBase, float64(steps))), minDistance, maxDistance)
}

func (c *Camera3D) Frame(b mathx.AABB3) {
	c.Target = b.Min.Add(b.Max).Scale(0.5)
	r := b.Max.Sub(b.Min).Len() * 0.5
	if r < 0.25 {
		r = 0.25
	}
	half := float64(c.fov()) / 2
	d := r / float32(math.Sin(half)) * 1.1
	if c.Ortho {
		d = r / float32(math.Tan(half)) * 1.1
	}
	c.Distance = mathx.Clamp(d, minDistance, maxDistance)
}
