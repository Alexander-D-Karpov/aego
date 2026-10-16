package stdlib

import (
	"aego/core/mathx"
	"aego/engine/module"
	"aego/modules/physics/components"
	"aego/modules/physics/systems"
	"aego/modules/physics/world"
	"aego/script/bind"
)

func body(c *bind.Ctx) (*systems.Sim, world.BodyHandle, bool) {
	s := need(c)
	if s == nil {
		return nil, world.BodyHandle{}, false
	}
	h, ok := s.Body(c.Recv)
	if !ok {
		c.Fail("node has no simulated physics body")
		return nil, world.BodyHandle{}, false
	}
	return s, h, true
}

func BodyMethods(b *module.ComponentBuilder[components.Body2D]) {
	b.Proc1("ApplyForce", func(c *bind.Ctx, _ *components.Body2D, f mathx.Vec2) {
		if s, h, ok := body(c); ok {
			fail(c, s.World().ApplyForce(h, f))
		}
	}, bind.Doc("ApplyForce pushes the body's center of mass during the next physics step. Call it from FixedUpdate."), bind.Params("force"))

	b.Proc2("ApplyForceAt", func(c *bind.Ctx, _ *components.Body2D, f, p mathx.Vec2) {
		if s, h, ok := body(c); ok {
			fail(c, s.World().ApplyForceAtPoint(h, f, p))
		}
	}, bind.Doc("ApplyForceAt pushes the body at a world point, which also spins it."), bind.Params("force", "point"))

	b.Proc1("ApplyImpulse", func(c *bind.Ctx, _ *components.Body2D, j mathx.Vec2) {
		if s, h, ok := body(c); ok {
			fail(c, s.World().ApplyImpulse(h, j))
		}
	}, bind.Doc("ApplyImpulse changes the velocity at once, scaled by mass."), bind.Params("impulse"),
		bind.Example("body.ApplyImpulse(Vec2{Y: -jumpImpulse})"))

	b.Proc2("ApplyImpulseAt", func(c *bind.Ctx, _ *components.Body2D, j, p mathx.Vec2) {
		if s, h, ok := body(c); ok {
			fail(c, s.World().ApplyImpulseAtPoint(h, j, p))
		}
	}, bind.Doc("ApplyImpulseAt applies an impulse at a world point."), bind.Params("impulse", "point"))

	b.Proc1("ApplyTorque", func(c *bind.Ctx, _ *components.Body2D, t float32) {
		if s, h, ok := body(c); ok {
			fail(c, s.World().ApplyTorque(h, t))
		}
	}, bind.Doc("ApplyTorque spins the body during the next physics step."), bind.Params("torque"))

	b.Proc1("SetVelocity", func(c *bind.Ctx, _ *components.Body2D, v mathx.Vec2) {
		if s, h, ok := body(c); ok {
			fail(c, s.World().SetLinearVelocity(h, v))
		}
	}, bind.Doc("SetVelocity sets the linear velocity in units per second."), bind.Params("velocity"))

	b.Proc1("SetAngularVelocity", func(c *bind.Ctx, _ *components.Body2D, w float32) {
		if s, h, ok := body(c); ok {
			fail(c, s.World().SetAngularVelocity(h, w))
		}
	}, bind.Doc("SetAngularVelocity sets the spin in radians per second."), bind.Params("velocity"))

	b.Proc2("MoveTo", func(c *bind.Ctx, _ *components.Body2D, p mathx.Vec2, angle float32) {
		s, h, ok := body(c)
		if !ok {
			return
		}
		dt := c.Fixed
		if dt <= 0 {
			dt = c.Dt
		}
		fail(c, s.World().MoveKinematic(h, world.Transform{Position: p, Angle: angle}, dt))
	}, bind.Doc("MoveTo drives a kinematic body to the pose by the end of the next physics step, pushing dynamic bodies on the way. Call it from FixedUpdate."),
		bind.Params("position", "angle"))

	b.Proc2("Teleport", func(c *bind.Ctx, _ *components.Body2D, p mathx.Vec2, angle float32) {
		if s := need(c); s != nil {
			fail(c, s.Teleport(c.Recv, p, angle))
		}
	}, bind.Doc("Teleport moves the body to a world pose without sweeping, keeping its velocity."), bind.Params("position", "angle"))

	b.Proc0("WakeUp", func(c *bind.Ctx, _ *components.Body2D) {
		if s, h, ok := body(c); ok {
			fail(c, s.World().WakeUp(h))
		}
	}, bind.Doc("WakeUp makes a sleeping body simulate again."))

	b.Proc0("Sleep", func(c *bind.Ctx, _ *components.Body2D) {
		if s, h, ok := body(c); ok {
			fail(c, s.World().Sleep(h))
		}
	}, bind.Doc("Sleep stops the body until something touches it."))

	b.Method0("Mass", func(c *bind.Ctx, _ *components.Body2D) float32 {
		s, h, ok := body(c)
		if !ok {
			return 0
		}
		m, _, _, _ := s.World().BodyMass(h)
		return m
	}, bind.Doc("Mass returns the mass computed from the colliders or the override."))
}
