package scriptevents

import "aego/scene"

// Listener is optional for a Sink: it reports whether a script on node handles ev.
type Listener interface {
	ListensOn(node scene.NodeHandle, ev EventID) bool
}
