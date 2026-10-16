package physics2d

import (
	"aego/core/mathx"
	"aego/scene"
)

const ServiceID = "physics2d.queries"

type QueryFilter struct {
	LayerMask       uint32
	IncludeTriggers bool
	ExcludeNode     scene.NodeHandle
}

type RayHit struct {
	Point        mathx.Vec2
	Normal       mathx.Vec2
	Distance     float32
	Node         scene.NodeHandle
	ColliderNode scene.NodeHandle
}

type Queries interface {
	Raycast(origin, dir mathx.Vec2, maxDist float32, f QueryFilter) (RayHit, bool)
	RaycastAll(origin, dir mathx.Vec2, maxDist float32, f QueryFilter, out []RayHit) int
	OverlapPoint(p mathx.Vec2, f QueryFilter, out []scene.NodeHandle) int
	OverlapCircle(c mathx.Vec2, r float32, f QueryFilter, out []scene.NodeHandle) int
	OverlapBox(c, half mathx.Vec2, angle float32, f QueryFilter, out []scene.NodeHandle) int
	BodyAt(p mathx.Vec2, f QueryFilter) (scene.NodeHandle, bool)
}
