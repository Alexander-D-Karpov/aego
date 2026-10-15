package module

import (
	"strconv"

	"aego/contracts/scriptevents"
	"aego/script/abi"
)

type ownedEvent struct {
	ev    scriptevents.Event
	owner string
}

func (r *Registry) ScriptEvent(ev scriptevents.Event) scriptevents.EventID {
	switch {
	case !abi.Exported(ev.Name):
		r.fail("script event " + strconv.Quote(ev.Name) + " needs an exported name")
		return scriptevents.NoEvent
	case scriptevents.IsLifecycle(ev.Name):
		r.fail("script event " + ev.Name + " collides with a lifecycle method")
		return scriptevents.NoEvent
	case len(ev.Names) > 0 && len(ev.Names) != len(ev.Params):
		r.fail("script event " + ev.Name + " names " + strconv.Itoa(len(ev.Names)) + " parameters, it has " + strconv.Itoa(len(ev.Params)))
		return scriptevents.NoEvent
	case len(r.events) >= int(scriptevents.NoEvent):
		r.fail("too many script events")
		return scriptevents.NoEvent
	}
	for _, e := range r.events {
		if e.ev.Name == ev.Name {
			r.fail("script event " + ev.Name + " is declared by " + e.owner + " and " + r.Owner())
			return scriptevents.NoEvent
		}
	}
	for i, p := range ev.Params {
		if !abi.Plain(p) {
			r.fail("script event " + ev.Name + " param " + strconv.Itoa(i) + " has type " + p.String() + ", which cannot be an event parameter")
			return scriptevents.NoEvent
		}
	}
	r.events = append(r.events, ownedEvent{ev: ev, owner: r.Owner()})
	return scriptevents.EventID(len(r.events) - 1)
}

func (r *Registry) Events() []scriptevents.Event {
	out := make([]scriptevents.Event, len(r.events))
	for i, e := range r.events {
		out[i] = e.ev
	}
	return out
}
