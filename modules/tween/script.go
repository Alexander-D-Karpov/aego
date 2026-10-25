package tween

import (
	"aego/core/curve"
	"aego/core/mathx"
	"aego/scene"
	"aego/script/abi"
	"aego/script/bind"
)

func scriptService(c *bind.Ctx) *Service {
	var s *Service
	if c.World != nil {
		s = Of(c.World)
	}
	if s == nil {
		c.Fail("tweens and timers run only during Play")
	}
	return s
}

func made(c *bind.Ctx, h Handle, err error) Handle {
	if err != nil {
		c.Fail(err.Error())
		return Handle{}
	}
	return h
}

func eased(e curve.Easing) Options {
	return Options{Ease: e}
}

func live(c *bind.Ctx) *Service {
	if c.World == nil {
		return nil
	}
	return Of(c.World)
}

type timerRef Handle

func timerOf(k uint64) timerRef {
	return timerRef(HandleOf(k))
}

func (t timerRef) key() uint64 {
	return Handle(t).Key()
}

func installTween(m *bind.Module) {
	bind.Handle(m, "Tween", true, HandleOf, Handle.Key,
		bind.Doc("Tween animates one property of a node. A finished, stopped or nil tween reads as done; yield t waits until it ends."))
	ps := bind.Params("node", "path", "to", "duration", "ease")
	bind.Func5(m, "ToFloat", func(c *bind.Ctx, n scene.NodeHandle, path string, to, dur float32, e curve.Easing) Handle {
		s := scriptService(c)
		if s == nil {
			return Handle{}
		}
		h, err := s.ToFloat(n, path, to, dur, eased(e))
		return made(c, h, err)
	}, bind.Doc("ToFloat animates a float or int property, or one lane of a vector or color.\nPaths: Component.Field, Component.Field.a, transform.rotation, script/Name.Field."),
		ps, bind.Example("tween.ToFloat(node, \"SpriteRenderer.Tint.a\", 0, 0.3, curve.OutQuad)"))
	bind.Func5(m, "ToVec2", func(c *bind.Ctx, n scene.NodeHandle, path string, to mathx.Vec2, dur float32, e curve.Easing) Handle {
		s := scriptService(c)
		if s == nil {
			return Handle{}
		}
		h, err := s.ToVec2(n, path, to, dur, eased(e))
		return made(c, h, err)
	}, bind.Doc("ToVec2 animates a Vec2 property."), ps,
		bind.Example("tween.ToVec2(node, \"transform.scale\", Vec2{1.2, 1.2}, 0.1, curve.OutBack)"))
	bind.Func5(m, "ToVec3", func(c *bind.Ctx, n scene.NodeHandle, path string, to mathx.Vec3, dur float32, e curve.Easing) Handle {
		s := scriptService(c)
		if s == nil {
			return Handle{}
		}
		h, err := s.ToVec3(n, path, to, dur, eased(e))
		return made(c, h, err)
	}, bind.Doc("ToVec3 animates a Vec3 property, such as the position of a 3D node."), ps,
		bind.Example("tween.ToVec3(node, \"transform.position\", Vec3{0, 2, 0}, 1, curve.InOutSine)"))
	bind.Func5(m, "ToColor", func(c *bind.Ctx, n scene.NodeHandle, path string, to mathx.Color, dur float32, e curve.Easing) Handle {
		s := scriptService(c)
		if s == nil {
			return Handle{}
		}
		h, err := s.ToColor(n, path, to, dur, eased(e))
		return made(c, h, err)
	}, bind.Doc("ToColor animates a Color property."), ps,
		bind.Example("tween.ToColor(node, \"SpriteRenderer.Tint\", HitTint, 0.2, curve.Linear)"))
	bind.Func5(m, "ToQuat", func(c *bind.Ctx, n scene.NodeHandle, path string, to mathx.Quat, dur float32, e curve.Easing) Handle {
		s := scriptService(c)
		if s == nil {
			return Handle{}
		}
		h, err := s.ToQuat(n, path, to, dur, eased(e))
		return made(c, h, err)
	}, bind.Doc("ToQuat turns a rotation along the short arc."), ps,
		bind.Example("tween.ToQuat(node, \"transform.rotation\", math.QuatEuler(0, 1.57, 0), 0.5, curve.InOutCubic)"))
	bind.Func4(m, "Position", func(c *bind.Ctx, n scene.NodeHandle, to mathx.Vec2, dur float32, e curve.Easing) Handle {
		s := scriptService(c)
		if s == nil {
			return Handle{}
		}
		h, err := s.Position(n, to, dur, eased(e))
		return made(c, h, err)
	}, bind.Doc("Position moves a 2D node to a local position."), bind.Params("node", "to", "duration", "ease"),
		bind.Example("yield tween.Position(node, Vec2{4, 0}, 0.5, curve.OutCubic)"))

	hm := m.Registry().HandleMethods(KindTween)
	bind.Func1(hm, "Done", func(c *bind.Ctx, h Handle) bool {
		s := live(c)
		return s == nil || s.Done(h)
	}, bind.Doc("Done reports whether the tween finished, was stopped or is nil."))
	bind.Func1(hm, "Running", func(c *bind.Ctx, h Handle) bool {
		s := live(c)
		return s != nil && s.Running(h)
	}, bind.Doc("Running is the opposite of Done."))
	bind.Proc1(hm, "Stop", func(c *bind.Ctx, h Handle) {
		if s := live(c); s != nil {
			s.Stop(h)
		}
	}, bind.Doc("Stop ends the tween where it is; the property keeps its current value."))
	bind.Func2(hm, "Delay", func(c *bind.Ctx, h Handle, d float32) Handle {
		if s := live(c); s != nil {
			s.SetDelay(h, d)
		}
		return h
	}, bind.Doc("Delay waits before the tween starts; the start value is read when the delay ends. Call it right after creating the tween."),
		bind.Params("seconds"), bind.Example("tween.ToFloat(node, \"Light.Range\", 4, 1, curve.Linear).Delay(0.5)"))
	bind.Func2(hm, "Loop", func(c *bind.Ctx, h Handle, n int64) Handle {
		if s := live(c); s != nil {
			s.SetLegs(h, int32(n), false)
		}
		return h
	}, bind.Doc("Loop plays the tween n times from the start; -1 repeats until Stop."), bind.Params("count"))
	bind.Func2(hm, "PingPong", func(c *bind.Ctx, h Handle, n int64) Handle {
		if s := live(c); s != nil {
			s.SetLegs(h, int32(n), true)
		}
		return h
	}, bind.Doc("PingPong plays n legs, every second one backwards; 2 goes there and back, -1 repeats until Stop."), bind.Params("legs"))
	bind.Func1(hm, "Unscaled", func(c *bind.Ctx, h Handle) Handle {
		if s := live(c); s != nil {
			s.SetUnscaled(h, true)
		}
		return h
	}, bind.Doc("Unscaled runs the tween in real time, also while the game is paused."))
	bind.Func2(hm, "Curve", func(c *bind.Ctx, h Handle, cv bind.Curve) Handle {
		p, err := cv.Resolve()
		if err != nil {
			c.FailErr(err)
			return h
		}
		if s := live(c); s != nil {
			s.SetCurve(h, p)
		}
		return h
	}, bind.Doc("Curve replaces the easing with a curve field; its time range is stretched over the duration."), bind.Params("curve"))
	bind.Signal(hm, "Completed", []abi.TypeRef{abi.T(abi.Bool)}, bind.Params("finished"),
		bind.Doc("Completed runs once when the tween ends. Its finished parameter is true at the natural end, false after Stop, when a new tween on the same property replaced it, or when its node or target is gone."))
}

func installTimer(m *bind.Module) {
	bind.Handle(m, "Timer", true, timerOf, timerRef.key,
		bind.Doc("Timer counts down and fires. yield t waits for the next fire or the end; a timer dies with the node of the script that made it."))
	start := func(c *bind.Ctx, sec float32, keep bool) timerRef {
		s := scriptService(c)
		if s == nil {
			return timerRef{}
		}
		h, err := s.StartTimer(sec, TimerOptions{Owner: c.Node, Keep: keep})
		return timerRef(made(c, h, err))
	}
	bind.Func1(m, "Start", func(c *bind.Ctx, sec float32) timerRef {
		return start(c, sec, false)
	}, bind.Doc("Start makes a one-shot timer that frees itself when it fires."), bind.Params("seconds"),
		bind.Example("yield timer.Start(0.5)"))
	bind.Func1(m, "New", func(c *bind.Ctx, sec float32) timerRef {
		return start(c, sec, true)
	}, bind.Doc("New makes a timer that stays after it ends, so Restart can run it again."), bind.Params("seconds"),
		bind.Example("spawn = timer.New(3).Repeat(-1)"))
	bind.Func1(m, "Cooldown", func(c *bind.Ctx, sec float32) timerRef {
		s := scriptService(c)
		if s == nil {
			return timerRef{}
		}
		h, err := s.Cooldown(sec, TimerOptions{Owner: c.Node})
		return timerRef(made(c, h, err))
	}, bind.Doc("Cooldown makes a timer that starts ready; Use takes it and starts the wait."), bind.Params("seconds"),
		bind.Example("if attack.Use() {\n\tfire()\n}"))

	hm := m.Registry().HandleMethods(KindTimer)
	bind.Func1(hm, "Done", func(c *bind.Ctx, h timerRef) bool {
		s := live(c)
		return s == nil || s.TimerDone(Handle(h))
	}, bind.Doc("Done reports whether the timer ended, was cancelled or is nil."))
	bind.Func1(hm, "Remaining", func(c *bind.Ctx, h timerRef) float32 {
		if s := live(c); s != nil {
			return s.Remaining(Handle(h))
		}
		return 0
	}, bind.Doc("Remaining returns the seconds left until the next fire."))
	bind.Func1(hm, "Count", func(c *bind.Ctx, h timerRef) int64 {
		if s := live(c); s != nil {
			return int64(s.Fired(Handle(h)))
		}
		return 0
	}, bind.Doc("Count returns how many times the timer has fired."))
	bind.Proc1(hm, "Pause", func(c *bind.Ctx, h timerRef) {
		if s := live(c); s != nil {
			s.Pause(Handle(h))
		}
	}, bind.Doc("Pause stops the countdown."))
	bind.Proc1(hm, "Resume", func(c *bind.Ctx, h timerRef) {
		if s := live(c); s != nil {
			s.Resume(Handle(h))
		}
	}, bind.Doc("Resume continues a paused countdown."))
	bind.Func1(hm, "Restart", func(c *bind.Ctx, h timerRef) bool {
		s := live(c)
		return s != nil && s.Restart(Handle(h))
	}, bind.Doc("Restart starts the countdown again; it does nothing on a one-shot timer that already fired."))
	bind.Proc1(hm, "Cancel", func(c *bind.Ctx, h timerRef) {
		if s := live(c); s != nil {
			s.Cancel(Handle(h))
		}
	}, bind.Doc("Cancel ends the timer without firing."))
	bind.Func2(hm, "Repeat", func(c *bind.Ctx, h timerRef, n int64) timerRef {
		if s := live(c); s != nil {
			s.SetRepeat(Handle(h), int32(n))
		}
		return h
	}, bind.Doc("Repeat makes the timer fire n times; -1 repeats until Cancel."), bind.Params("count"))
	bind.Func1(hm, "Unscaled", func(c *bind.Ctx, h timerRef) timerRef {
		if s := live(c); s != nil {
			s.SetTimerUnscaled(Handle(h), true)
		}
		return h
	}, bind.Doc("Unscaled counts real time, also while the game is paused."))
	bind.Func1(hm, "Ready", func(c *bind.Ctx, h timerRef) bool {
		s := live(c)
		return s != nil && s.Ready(Handle(h))
	}, bind.Doc("Ready reports whether a cooldown can be used."))
	bind.Func1(hm, "Use", func(c *bind.Ctx, h timerRef) bool {
		s := live(c)
		return s != nil && s.Use(Handle(h))
	}, bind.Doc("Use takes a ready cooldown and starts its wait; it returns false while the cooldown runs."))
	bind.Signal(hm, "Fired", []abi.TypeRef{abi.T(abi.Int)}, bind.Params("count"),
		bind.Doc("Fired runs on every fire of the timer; count is the number of fires so far."))
}

type actionRef Handle

func actionOf(k uint64) actionRef {
	return actionRef(HandleOf(k))
}

func (a actionRef) key() uint64 {
	return Handle(a).Key()
}

func added(c *bind.Ctx, a actionRef, err error) actionRef {
	if err != nil {
		c.Fail(err.Error())
	}
	return a
}

func installAction(m *bind.Module) {
	bind.Handle(m, "Action", true, actionOf, actionRef.key,
		bind.Doc("Action runs tweens, timers and other actions one after another or together; yield a waits until it ends."))
	bind.Func0(m, "Sequence", func(c *bind.Ctx) actionRef {
		s := scriptService(c)
		if s == nil {
			return actionRef{}
		}
		return actionRef(s.Sequence(c.Node))
	}, bind.Doc("Sequence makes an action that runs its steps one after another, starting at the next tween update."),
		bind.Example("yield action.Sequence().Then(tween.Position(node, Vec2{4, 0}, 0.5, curve.OutCubic)).Wait(0.2).Then(tween.ToFloat(node, \"SpriteRenderer.Tint.a\", 0, 0.3, curve.Linear))"))
	bind.Func0(m, "Parallel", func(c *bind.Ctx) actionRef {
		s := scriptService(c)
		if s == nil {
			return actionRef{}
		}
		return actionRef(s.Parallel(c.Node))
	}, bind.Doc("Parallel makes an action that runs all its steps at once and ends after the last one."))

	hm := m.Registry().HandleMethods(KindAction)
	bind.Func2(hm, "Then", func(c *bind.Ctx, a actionRef, t Handle) actionRef {
		if s := live(c); s != nil {
			return added(c, a, s.ThenTween(Handle(a), t))
		}
		return a
	}, bind.Doc("Then adds a tween created in this frame; it waits for its turn."), bind.Params("tween"))
	bind.Func2(hm, "ThenTimer", func(c *bind.Ctx, a actionRef, t timerRef) actionRef {
		if s := live(c); s != nil {
			return added(c, a, s.ThenTimer(Handle(a), Handle(t)))
		}
		return a
	}, bind.Doc("ThenTimer adds a timer created in this frame; the step ends when the timer ends."), bind.Params("timer"))
	bind.Func2(hm, "ThenAction", func(c *bind.Ctx, a actionRef, b actionRef) actionRef {
		if s := live(c); s != nil {
			return added(c, a, s.ThenAction(Handle(a), Handle(b)))
		}
		return a
	}, bind.Doc("ThenAction adds another action that has not started."), bind.Params("action"))
	bind.Func2(hm, "Wait", func(c *bind.Ctx, a actionRef, sec float32) actionRef {
		if s := live(c); s != nil {
			return added(c, a, s.Wait(Handle(a), sec))
		}
		return a
	}, bind.Doc("Wait adds a pause in game seconds."), bind.Params("seconds"))
	bind.Func1(hm, "Done", func(c *bind.Ctx, a actionRef) bool {
		s := live(c)
		return s == nil || s.ActionDone(Handle(a))
	}, bind.Doc("Done reports whether the action ended, was stopped or is nil."))
	bind.Func1(hm, "Running", func(c *bind.Ctx, a actionRef) bool {
		s := live(c)
		return s != nil && !s.ActionDone(Handle(a))
	}, bind.Doc("Running is the opposite of Done."))
	bind.Proc1(hm, "Stop", func(c *bind.Ctx, a actionRef) {
		if s := live(c); s != nil {
			s.StopAction(Handle(a))
		}
	}, bind.Doc("Stop stops every unfinished step; the properties keep their current values."))
	bind.Signal(hm, "Completed", []abi.TypeRef{abi.T(abi.Bool)}, bind.Params("finished"),
		bind.Doc("Completed runs once when the action ends; finished is false after Stop or when the node of its script died."))
}
