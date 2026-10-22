package scriptcall

import (
	"aego/scene"
	"aego/script/vm"
)

const ServiceID = "script.call"

// Ref names a plain method of one script instance; a new program generation makes it stale.
type Ref struct {
	Node     scene.NodeHandle
	Instance vm.InstanceID
	Index    int32
	Gen      uint64
}

type Caller interface {
	Method(node scene.NodeHandle, name string, params int) (Ref, bool)
	Call(ref *Ref, args []vm.Value) bool
	Generation() uint64
}
