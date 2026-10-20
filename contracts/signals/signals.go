package signals

import (
	"aego/scene"
	"aego/script/vm"
)

const ServiceID = "signal.hub"

const (
	KindScript uint8 = iota
	KindComponent
	KindHandle
)

type Source struct {
	Kind uint8
	A    uint64
	B    uint64
}

type Handler func(args []vm.Value)

type Hub interface {
	Wants(src Source) bool
	Args(n int) []vm.Value
	Emit(src Source, sig int, args []vm.Value)
	End(src Source)
	Connect(src Source, sig int, fn Handler) vm.Value
	ConnectScript(node scene.NodeHandle, script, signal string, fn Handler) vm.Value
	Disconnect(c vm.Value)
	Connected(c vm.Value) bool
	Probe(typ uint64, alive func(key uint64) bool)
	HandleAlive(typ, key uint64) bool
}
