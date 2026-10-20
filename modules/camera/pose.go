package camera

import (
	"math"

	"aego/core/mathx"
	"aego/render/view"
)

type Pose struct {
	Position mathx.Vec2
	Rotation float32
	Zoom     float32
}

func poseOf(c view.Camera2D) Pose {
	return Pose{Position: c.Position, Rotation: c.Rotation, Zoom: c.Zoom}
}

func (p Pose) camera() view.Camera2D {
	return view.Camera2D{Position: p.Position, Rotation: p.Rotation, Zoom: p.Zoom}
}

func wrapAngle(a float32) float32 {
	return float32(math.Remainder(float64(a), 2*math.Pi))
}

func logLerp(a, b, k float32) float32 {
	if a <= 0 || b <= 0 {
		return a + (b-a)*k
	}
	la, lb := math.Log(float64(a)), math.Log(float64(b))
	return float32(math.Exp(la + (lb-la)*float64(k)))
}

func lerpPose(a, b Pose, k float32) Pose {
	return Pose{
		Position: a.Position.Lerp(b.Position, k),
		Rotation: a.Rotation + wrapAngle(b.Rotation-a.Rotation)*k,
		Zoom:     logLerp(a.Zoom, b.Zoom, k),
	}
}

func ViewSize(v view.View, zoom float32) mathx.Vec2 {
	if zoom > 0 {
		v.Camera.Zoom = zoom
	}
	ez := v.EffectiveZoom()
	if ez <= 0 {
		return mathx.Vec2{}
	}
	lb := v.LetterboxViewport()
	return mathx.Vec2{X: lb.W / ez, Y: lb.H / ez}
}

func Frame(center, size mathx.Vec2) mathx.Rect {
	return mathx.Rect{X: center.X - size.X/2, Y: center.Y - size.Y/2, W: size.X, H: size.Y}
}

func DeadzoneRect(center, size mathx.Vec2, dz mathx.Rect) mathx.Rect {
	return mathx.Rect{X: center.X + (dz.X-0.5)*size.X, Y: center.Y + (dz.Y-0.5)*size.Y, W: dz.W * size.X, H: dz.H * size.Y}
}

func ZoneCorners(m mathx.Mat3, a mathx.Rect) [4]mathx.Vec2 {
	return [4]mathx.Vec2{
		m.Apply(mathx.Vec2{X: a.X, Y: a.Y}),
		m.Apply(mathx.Vec2{X: a.X + a.W, Y: a.Y}),
		m.Apply(mathx.Vec2{X: a.X + a.W, Y: a.Y + a.H}),
		m.Apply(mathx.Vec2{X: a.X, Y: a.Y + a.H}),
	}
}

func zoneContains(m mathx.Mat3, a mathx.Rect, p mathx.Vec2) bool {
	c := ZoneCorners(m, a)
	lo, hi := c[0], c[0]
	for _, v := range c[1:] {
		lo = mathx.Vec2{X: min(lo.X, v.X), Y: min(lo.Y, v.Y)}
		hi = mathx.Vec2{X: max(hi.X, v.X), Y: max(hi.Y, v.Y)}
	}
	return p.X >= lo.X && p.X <= hi.X && p.Y >= lo.Y && p.Y <= hi.Y
}

func clampAxis(c, lo, size, half float32) float32 {
	if size <= 2*half {
		return lo + size/2
	}
	return mathx.Clamp(c, lo+half, lo+size-half)
}

func clampView(base view.View, p Pose, b mathx.Rect) mathx.Vec2 {
	size := ViewSize(base, p.Zoom)
	return mathx.Vec2{
		X: clampAxis(p.Position.X, b.X, b.W, size.X/2),
		Y: clampAxis(p.Position.Y, b.Y, b.H, size.Y/2),
	}
}

func expK(rate, dt float32) float32 {
	if rate <= 0 {
		return 1
	}
	return 1 - float32(math.Exp(-float64(rate*dt)))
}

func axisStep(pos, goal, size, dzMin, dzSize, rate, dt float32) float32 {
	lo := pos + (dzMin-0.5)*size
	hi := lo + dzSize*size
	var err float32
	switch {
	case goal < lo:
		err = goal - lo
	case goal > hi:
		err = goal - hi
	}
	return pos + err*expK(rate, dt)
}

func followStep(pos, goal, size mathx.Vec2, dz mathx.Rect, damping mathx.Vec2, dt float32) mathx.Vec2 {
	return mathx.Vec2{
		X: axisStep(pos.X, goal.X, size.X, dz.X, dz.W, damping.X, dt),
		Y: axisStep(pos.Y, goal.Y, size.Y, dz.Y, dz.H, damping.Y, dt),
	}
}

func limit(v, m float32) float32 {
	if m <= 0 {
		return v
	}
	return mathx.Clamp(v, -m, m)
}
