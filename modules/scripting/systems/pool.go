package systems

import (
	"aego/contracts/scriptpool"
	"aego/engine"
	"aego/scene"
)

type lifecycle struct {
	w *engine.World
}

func (l lifecycle) Park(h scene.NodeHandle) bool {
	rt := runtimeIn(l.w)
	return rt == nil || rt.ParkNode(h)
}

func (l lifecycle) Revive(h scene.NodeHandle) {
	if rt := runtimeIn(l.w); rt != nil {
		rt.ReviveNode(h)
	}
}

func Lifecycle(w *engine.World) scriptpool.Lifecycle {
	return lifecycle{w: w}
}
