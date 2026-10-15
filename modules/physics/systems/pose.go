package systems

import (
	"aego/core/mathx"
	"aego/scene"
)

type pose struct {
	p mathx.Vec2
	a float32
}

func Pack(h scene.NodeHandle) uint64 {
	return uint64(h.Gen)<<32 | uint64(h.Index)
}

func Unpack(v uint64) scene.NodeHandle {
	return scene.NodeHandle{Index: uint32(v), Gen: uint32(v >> 32)}
}

func frame(m mathx.Mat3) pose {
	return pose{p: mathx.Vec2{X: m[6], Y: m[7]}, a: mathx.Atan2(m[1], m[0])}
}

func relative(body, node mathx.Mat3) mathx.Mat3 {
	f := frame(body)
	inv, ok := mathx.Mat3TRS(f.p, f.a, mathx.Vec2{X: 1, Y: 1}).Inverse()
	if !ok {
		return node
	}
	return inv.Mul(node)
}

func near(a, b mathx.Mat3) bool {
	for i := range a {
		if mathx.Abs(a[i]-b[i]) > 1e-4*(1+mathx.Abs(b[i])) {
			return false
		}
	}
	return true
}

func toBody(f pose, p mathx.Vec2) mathx.Vec2 {
	d := p.Sub(f.p)
	c, s := mathx.Cos(f.a), mathx.Sin(f.a)
	return mathx.Vec2{X: c*d.X + s*d.Y, Y: -s*d.X + c*d.Y}
}

func wrapAngle(a float32) float32 {
	for a > mathx.Pi {
		a -= mathx.TwoPi
	}
	for a < -mathx.Pi {
		a += mathx.TwoPi
	}
	return a
}

func lerpPose(a, b pose, t float32) pose {
	return pose{p: a.p.Lerp(b.p, t), a: a.a + wrapAngle(b.a-a.a)*t}
}
