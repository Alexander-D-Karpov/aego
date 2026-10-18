package scriptpool

import "aego/scene"

const ServiceID = "script.pool"

type Lifecycle interface {
	Park(node scene.NodeHandle) bool
	Revive(node scene.NodeHandle)
}
