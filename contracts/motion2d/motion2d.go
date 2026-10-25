package motion2d

import (
	"aego/core/mathx"
	"aego/scene"
)

const ServiceID = "motion2d.receiver"

// Receiver moves a character by a world-space offset on its next motion step, through its collisions; false means the node has no controller.
type Receiver interface {
	AddDisplacement(node scene.NodeHandle, d mathx.Vec2) bool
}
