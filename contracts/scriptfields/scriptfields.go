package scriptfields

import (
	"aego/scene"
	"aego/script/abi"
	"aego/script/vm"
)

const ServiceID = "script.fields"

type Ref struct {
	Node     scene.NodeHandle
	Instance vm.InstanceID
	Script   string
	Field    string
	Kind     abi.Kind
	Slot     int
	Layout   any
}

type Fields interface {
	ResolveField(node scene.NodeHandle, script, field string) (Ref, bool)
	LoadField(ref *Ref) (vm.Value, bool)
	StoreField(ref *Ref, v vm.Value) bool
}
