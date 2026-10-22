package physics2d

import (
	"aego/core/mathx"
	"aego/scene"
)

const ServiceID = "physics2d.queries"

type QueryFilter struct {
	LayerMask       uint32
	IncludeTriggers bool
	SkipDynamic     bool
	ExcludeNode     scene.NodeHandle
	ExcludeBody     scene.NodeHandle
}

type RayHit struct {
	Point        mathx.Vec2
	Normal       mathx.Vec2
	Distance     float32
	Node         scene.NodeHandle
	ColliderNode scene.NodeHandle
}

type CastHit struct {
	Point        mathx.Vec2
	Normal       mathx.Vec2
	Fraction     float32
	Depth        float32
	Node         scene.NodeHandle
	ColliderNode scene.NodeHandle
	Initial      bool
}

type Queries interface {
	Raycast(origin, dir mathx.Vec2, maxDist float32, f QueryFilter) (RayHit, bool)
	RaycastAll(origin, dir mathx.Vec2, maxDist float32, f QueryFilter, out []RayHit) int
	OverlapPoint(p mathx.Vec2, f QueryFilter, out []scene.NodeHandle) int
	OverlapCircle(c mathx.Vec2, r float32, f QueryFilter, out []scene.NodeHandle) int
	OverlapBox(c, half mathx.Vec2, angle float32, f QueryFilter, out []scene.NodeHandle) int
	BodyAt(p mathx.Vec2, f QueryFilter) (scene.NodeHandle, bool)
	CastCircle(c mathx.Vec2, r float32, delta mathx.Vec2, f QueryFilter) (CastHit, bool)
	CastBox(c, half mathx.Vec2, angle float32, delta mathx.Vec2, f QueryFilter) (CastHit, bool)
	CastCapsule(a, b mathx.Vec2, r float32, delta mathx.Vec2, f QueryFilter) (CastHit, bool)
	CastBody(body scene.NodeHandle, offset, delta mathx.Vec2, f QueryFilter) (CastHit, bool)
	Penetration(body scene.NodeHandle, f QueryFilter, out []CastHit) int
	BodyPose(body scene.NodeHandle) (mathx.Vec2, float32, bool)
	BodyVelocity(body scene.NodeHandle) (mathx.Vec2, float32, bool)
	MoveKinematic(body scene.NodeHandle, p mathx.Vec2, angle, dt float32) error
	Teleport(body scene.NodeHandle, p mathx.Vec2, angle float32) error
	ApplyImpulse(body scene.NodeHandle, j mathx.Vec2) error
	Gravity() mathx.Vec2
}
