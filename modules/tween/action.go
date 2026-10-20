package tween

import (
	"aego/core/errs"
	"aego/scene"
	"aego/script/abi"
	"aego/script/vm"
)

const (
	ScriptAction       = "aego/action"
	KindAction         = ScriptAction + ".Action"
	sigActionCompleted = 0
)

var typeAction = abi.HandleID(KindAction)

type stepKind uint8

const (
	stepTween stepKind = iota
	stepTimer
	stepAction
)

type actStep struct {
	kind stepKind
	h    Handle
	done bool
}

type action struct {
	parallel bool
	started  bool
	held     bool
	steps    []actStep
	cur      int
	left     int
	owner    scene.NodeHandle
	parent   Handle
}

func errAction(msg string) error {
	return errs.New(errs.EngineModule, "action: "+msg)
}

func (s *Service) newAction(parallel bool, owner scene.NodeHandle) Handle {
	h, a := s.actions.alloc()
	steps := a.steps[:0]
	*a = action{parallel: parallel, owner: owner, steps: steps}
	return h
}

func (s *Service) Sequence(owner scene.NodeHandle) Handle {
	return s.newAction(false, owner)
}

func (s *Service) Parallel(owner scene.NodeHandle) Handle {
	return s.newAction(true, owner)
}

func (s *Service) ActionDone(h Handle) bool {
	return s.actions.get(h) == nil
}

func (s *Service) open(ah Handle) (*action, error) {
	a := s.actions.get(ah)
	switch {
	case a == nil:
		return nil, errAction("the action is nil or finished")
	case a.started:
		return nil, errAction("steps can be added only before the action starts")
	}
	return a, nil
}

func (s *Service) ThenTween(ah, th Handle) error {
	a, err := s.open(ah)
	if err != nil {
		return err
	}
	tw := s.tweens.get(th)
	switch {
	case tw == nil:
		return errAction("the tween is nil or finished")
	case tw.moved || !tw.act.IsZero():
		return errAction("the tween has already started or belongs to another action")
	}
	tw.held, tw.act = true, ah
	a.steps = append(a.steps, actStep{kind: stepTween, h: th})
	return nil
}

func (s *Service) ThenTimer(ah, th Handle) error {
	a, err := s.open(ah)
	if err != nil {
		return err
	}
	t := s.timers.get(th)
	switch {
	case t == nil || t.done:
		return errAction("the timer is nil, finished or a ready cooldown")
	case t.moved || !t.act.IsZero():
		return errAction("the timer has already started or belongs to another action")
	}
	t.held, t.act = true, ah
	a.steps = append(a.steps, actStep{kind: stepTimer, h: th})
	return nil
}

func (s *Service) ThenAction(ah, ch Handle) error {
	if ah == ch {
		return errAction("an action cannot contain itself")
	}
	a, err := s.open(ah)
	if err != nil {
		return err
	}
	c := s.actions.get(ch)
	switch {
	case c == nil:
		return errAction("the inner action is nil or finished")
	case c.started || !c.parent.IsZero():
		return errAction("the inner action has already started or belongs to another action")
	}
	c.held, c.parent = true, ah
	a.steps = append(a.steps, actStep{kind: stepAction, h: ch})
	return nil
}

func (s *Service) Wait(ah Handle, sec float32) error {
	if _, err := s.open(ah); err != nil {
		return err
	}
	th, err := s.StartTimer(sec, TimerOptions{})
	if err != nil {
		return err
	}
	return s.ThenTimer(ah, th)
}

func (s *Service) stepActions() {
	for i := range s.actions.s {
		e := &s.actions.s[i]
		if !e.live {
			continue
		}
		h := s.actions.handle(uint32(i))
		a := &e.v
		switch {
		case !a.owner.IsZero() && !s.scene.IsAlive(a.owner):
			s.StopAction(h)
		case !a.started && !a.held:
			s.startAction(h)
		}
	}
}

func (s *Service) startAction(h Handle) {
	a := s.actions.get(h)
	if a == nil || a.started {
		return
	}
	a.started = true
	n := len(a.steps)
	switch {
	case n == 0:
		s.finishAction(h, true)
		return
	case !a.parallel:
		a.cur = 0
		s.release(h, a.steps[0])
		return
	}
	a.left = n
	for i := 0; i < n; i++ {
		if a = s.actions.get(h); a == nil || !a.started {
			return
		}
		s.release(h, a.steps[i])
	}
}

func (s *Service) release(parent Handle, st actStep) {
	switch st.kind {
	case stepTween:
		if tw := s.tweens.get(st.h); tw != nil {
			tw.held = false
			return
		}
	case stepTimer:
		if t := s.timers.get(st.h); t != nil {
			t.held = false
			return
		}
	case stepAction:
		if a := s.actions.get(st.h); a != nil {
			a.held = false
			s.startAction(st.h)
			return
		}
	}
	s.stepDone(parent, st.kind, st.h)
}

func (s *Service) stepDone(ah Handle, kind stepKind, h Handle) {
	a := s.actions.get(ah)
	if a == nil || !a.started {
		return
	}
	idx := -1
	for i := range a.steps {
		if st := &a.steps[i]; st.kind == kind && st.h == h && !st.done {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	a.steps[idx].done = true
	if a.parallel {
		a.left--
		if a.left == 0 {
			s.finishAction(ah, true)
		}
		return
	}
	prev := a.cur
	for a.cur < len(a.steps) && a.steps[a.cur].done {
		a.cur++
	}
	switch {
	case a.cur == len(a.steps):
		s.finishAction(ah, true)
	case a.cur != prev:
		s.release(ah, a.steps[a.cur])
	}
}

func (s *Service) finishAction(h Handle, finished bool) {
	a := s.actions.get(h)
	if a == nil {
		return
	}
	parent := a.parent
	key := h.Key()
	s.actions.release(h.Index)
	src := handleSource(typeAction, key)
	s.emitOne(src, sigActionCompleted, vm.Bool(finished))
	s.endHandle(src)
	s.wake(typeAction, key)
	if !parent.IsZero() {
		s.stepDone(parent, stepAction, h)
	}
}

func (s *Service) StopAction(h Handle) {
	a := s.actions.get(h)
	if a == nil {
		return
	}
	a.started = false
	for i := 0; ; i++ {
		a = s.actions.get(h)
		if a == nil || i >= len(a.steps) {
			break
		}
		st := a.steps[i]
		if st.done {
			continue
		}
		switch st.kind {
		case stepTween:
			s.Stop(st.h)
		case stepTimer:
			s.Cancel(st.h)
		case stepAction:
			s.StopAction(st.h)
		}
	}
	s.finishAction(h, false)
}
