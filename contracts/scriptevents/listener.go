package scriptevents

import (
	"aego/scene"
	"aego/script/vm"
)

// Listener is optional for a Sink: it reports whether a script on node handles ev.
type Listener interface {
	ListensOn(node scene.NodeHandle, ev EventID) bool
}

type Strings interface {
	Intern(s string) vm.Value
}

type Broadcaster interface {
	Broadcast(ev EventID, args []vm.Value)
}
