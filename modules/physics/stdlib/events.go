package stdlib

import (
	"aego/contracts/scriptevents"
	"aego/engine"
	"aego/engine/module"
	"aego/modules/physics/systems"
	"aego/modules/physics/world"
	"aego/scene"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/vm"
)

type EventIDs struct {
	Enter        scriptevents.EventID
	Stay         scriptevents.EventID
	Exit         scriptevents.EventID
	TriggerEnter scriptevents.EventID
	TriggerExit  scriptevents.EventID
}

func RegisterEvents(r *module.Registry) EventIDs {
	node := abi.T(abi.NodeRef)
	contact := abi.StructOf(ModPhysics + ".Contact")
	pair := []abi.TypeRef{node, contact}
	ev := func(name, doc string, params []abi.TypeRef, names ...string) scriptevents.EventID {
		return r.ScriptEvent(scriptevents.Event{Name: name, Params: params, Names: names, Doc: doc})
	}
	return EventIDs{
		Enter: ev("OnCollisionEnter", "OnCollisionEnter runs after the physics step in which this body started touching other.",
			pair, "other", "contact"),
		Stay: ev("OnCollisionStay", "OnCollisionStay runs after every later physics step while the bodies keep touching. It costs nothing when no script declares it.",
			pair, "other", "contact"),
		Exit: ev("OnCollisionExit", "OnCollisionExit runs when the bodies stop touching, including when one of them is destroyed or disabled. Contact.Point is the last known point.",
			pair, "other", "contact"),
		TriggerEnter: ev("OnTriggerEnter", "OnTriggerEnter runs when other's collider entered a trigger of this body, or this body entered other's trigger.",
			[]abi.TypeRef{node}, "other"),
		TriggerExit: ev("OnTriggerExit", "OnTriggerExit runs when the overlap with a trigger ends.",
			[]abi.TypeRef{node}, "other"),
	}
}

type side struct {
	body scene.NodeHandle
	col  scene.NodeHandle
}

type Emitter struct {
	IDs  EventIDs
	Lib  *Lib
	args [16]vm.Value
}

func (e *Emitter) Run(ctx *engine.Context) {
	if e.Lib == nil || ctx.World == nil {
		return
	}
	sim, _ := engine.ServiceOf[*systems.Sim](ctx.World, systems.ServiceID)
	sink, _ := engine.ServiceOf[scriptevents.Sink](ctx.World, scriptevents.ServiceID)
	if sim == nil || sink == nil {
		return
	}
	w := sim.World()
	evs := w.Events().Contacts
	for i := range evs {
		ev := &evs[i]
		switch ev.Kind {
		case world.BeginContact:
			e.collision(sink, nil, sim, ev, e.IDs.Enter)
		case world.EndContact:
			e.collision(sink, nil, sim, ev, e.IDs.Exit)
		case world.BeginTrigger:
			e.trigger(sink, sim, ev, e.IDs.TriggerEnter)
		case world.EndTrigger:
			e.trigger(sink, sim, ev, e.IDs.TriggerExit)
		}
	}
	l, ok := sink.(scriptevents.Listener)
	if !ok || e.IDs.Stay == scriptevents.NoEvent {
		return
	}
	for _, t := range sim.Touches() {
		if t.Fresh || !e.wanted(l, sim, t) {
			continue
		}
		if ev, ok := w.ContactBetween(t.ShapeA, t.ShapeB); ok {
			e.collision(sink, l, sim, &ev, e.IDs.Stay)
		}
	}
}

func (e *Emitter) wanted(l scriptevents.Listener, sim *systems.Sim, t systems.Touch) bool {
	w := sim.World()
	for _, sh := range [2]world.ShapeHandle{t.ShapeA, t.ShapeB} {
		col := sim.ShapeNode(sh)
		if l.ListensOn(col, e.IDs.Stay) {
			return true
		}
		if bh, ok := w.ShapeBody(sh); ok && l.ListensOn(sim.BodyNode(bh), e.IDs.Stay) {
			return true
		}
	}
	return false
}

func (e *Emitter) sides(sim *systems.Sim, ev *world.ContactEvent) (side, side) {
	return side{body: sim.BodyNode(ev.BodyA), col: sim.ShapeNode(ev.ShapeA)},
		side{body: sim.BodyNode(ev.BodyB), col: sim.ShapeNode(ev.ShapeB)}
}

func (e *Emitter) collision(sink scriptevents.Sink, l scriptevents.Listener, sim *systems.Sim, ev *world.ContactEvent, id scriptevents.EventID) {
	if id == scriptevents.NoEvent {
		return
	}
	a, b := e.sides(sim, ev)
	ca := Contact{
		Point: ev.Point, Normal: ev.Normal, NormalImpulse: ev.NormalImpulse,
		RelativeVelocity: ev.RelativeVelocity, SelfCollider: a.col, OtherCollider: b.col,
	}
	cb := Contact{
		Point: ev.Point, Normal: ev.Normal.Neg(), NormalImpulse: ev.NormalImpulse,
		RelativeVelocity: ev.RelativeVelocity.Neg(), SelfCollider: b.col, OtherCollider: a.col,
	}
	e.deliver(sink, l, id, a, b.body, &ca)
	e.deliver(sink, l, id, b, a.body, &cb)
}

func (e *Emitter) trigger(sink scriptevents.Sink, sim *systems.Sim, ev *world.ContactEvent, id scriptevents.EventID) {
	if id == scriptevents.NoEvent {
		return
	}
	a, b := e.sides(sim, ev)
	e.deliver(sink, nil, id, a, b.body, nil)
	e.deliver(sink, nil, id, b, a.body, nil)
}

func (e *Emitter) deliver(sink scriptevents.Sink, l scriptevents.Listener, id scriptevents.EventID, self side, other scene.NodeHandle, c *Contact) {
	targets := [2]scene.NodeHandle{self.body, self.col}
	for i, t := range targets {
		if i == 1 && t == self.body {
			return
		}
		if l != nil && !l.ListensOn(t, id) {
			continue
		}
		e.args[0] = bind.NodeValue(other)
		n := 1
		if c != nil {
			e.Lib.contact.Put(e.args[1:], *c)
			n += e.Lib.contact.Slots()
		}
		sink.Emit(t, id, e.args[:n])
	}
}
