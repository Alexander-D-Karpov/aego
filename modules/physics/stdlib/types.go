package stdlib

import (
	"aego/core/mathx"
	"aego/scene"
)

const ModPhysics = "aego/physics"

type Contact struct {
	Point            mathx.Vec2
	Normal           mathx.Vec2
	NormalImpulse    float32
	RelativeVelocity mathx.Vec2
	SelfCollider     scene.NodeHandle
	OtherCollider    scene.NodeHandle
}

type RayHit struct {
	Hit      bool
	Point    mathx.Vec2
	Normal   mathx.Vec2
	Distance float32
	Node     scene.NodeHandle
	Collider scene.NodeHandle
}
