package systems

import (
	"aego/contracts/persistence"
	"aego/engine"
	"aego/scene"
)

type persist struct {
	w *engine.World
}

func (p persist) Prepare() {
	if rt := runtimeIn(p.w); rt != nil {
		rt.Prepare()
	}
}

func (p persist) Persisted(h scene.NodeHandle, dst []persistence.ScriptValue) []persistence.ScriptValue {
	if rt := runtimeIn(p.w); rt != nil {
		return rt.Persisted(h, dst)
	}
	return dst
}

func (p persist) RestoreField(h scene.NodeHandle, v persistence.ScriptValue) bool {
	rt := runtimeIn(p.w)
	return rt != nil && rt.RestoreField(h, v)
}

func Persist(w *engine.World) persistence.Scripts {
	return persist{w: w}
}
