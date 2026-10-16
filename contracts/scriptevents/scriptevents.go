package scriptevents

import (
	"aego/scene"
	"aego/script/abi"
	"aego/script/vm"
)

const ServiceID = "script.events"

type EventID uint16

const NoEvent EventID = 0xFFFF

type Event struct {
	Name   string
	Params []abi.TypeRef
	Names  []string
	Doc    string
}

type Sink interface {
	Emit(node scene.NodeHandle, ev EventID, args []vm.Value)
	Flush()
}

func IsLifecycle(name string) bool {
	switch name {
	case "Ready", "FixedUpdate", "Update", "Reloaded":
		return true
	}
	return false
}
