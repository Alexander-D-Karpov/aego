package tween

import (
	"aego/core/errs"
	"aego/scene"
	"aego/script/vm"
)

type TimerOptions struct {
	Repeat   int32
	Unscaled bool
	Keep     bool
	Owner    scene.NodeHandle
}

type timer struct {
	dur      float32
	left     float32
	legs     int32
	repeat   int32
	count    uint32
	owner    scene.NodeHandle
	paused   bool
	unscaled bool
	keep     bool
	done     bool
	act      Handle
	held     bool
	moved    bool
}

func (s *Service) Timers() int {
	return s.timers.n
}

func (s *Service) StartTimer(dur float32, o TimerOptions) (Handle, error) {
	if dur != dur || dur < 0 {
		return Handle{}, errs.New(errs.EngineModule, "timer: duration must be zero or positive")
	}
	rep := normLegs(o.Repeat)
	h, t := s.timers.alloc()
	*t = timer{dur: dur, left: dur, legs: rep, repeat: rep, owner: o.Owner, unscaled: o.Unscaled, keep: o.Keep}
	return h, nil
}

func (s *Service) SetRepeat(h Handle, n int32) {
	if t := s.timers.get(h); t != nil {
		t.repeat = normLegs(n)
		if !t.done {
			t.legs = t.repeat
		}
	}
}

func (s *Service) SetTimerUnscaled(h Handle, on bool) {
	if t := s.timers.get(h); t != nil {
		t.unscaled = on
	}
}

func (s *Service) Cooldown(dur float32, o TimerOptions) (Handle, error) {
	o.Keep = true
	h, err := s.StartTimer(dur, o)
	if err == nil {
		t := s.timers.get(h)
		t.done, t.left = true, 0
	}
	return h, err
}

func (s *Service) TimerDone(h Handle) bool {
	t := s.timers.get(h)
	return t == nil || t.done
}

func (s *Service) Remaining(h Handle) float32 {
	if t := s.timers.get(h); t != nil && !t.done {
		return max(t.left, 0)
	}
	return 0
}

func (s *Service) Fired(h Handle) uint32 {
	if t := s.timers.get(h); t != nil {
		return t.count
	}
	return 0
}

func (s *Service) Pause(h Handle) {
	if t := s.timers.get(h); t != nil {
		t.paused = true
	}
}

func (s *Service) Resume(h Handle) {
	if t := s.timers.get(h); t != nil {
		t.paused = false
	}
}

func (s *Service) Restart(h Handle) bool {
	t := s.timers.get(h)
	if t == nil {
		return false
	}
	t.left, t.legs, t.done, t.paused = t.dur, t.repeat, false, false
	return true
}

func (s *Service) Cancel(h Handle) {
	if s.timers.get(h) != nil {
		s.dropTimer(h.Index)
	}
}

func (s *Service) Ready(h Handle) bool {
	t := s.timers.get(h)
	return t != nil && t.done
}

func (s *Service) Use(h Handle) bool {
	return s.Ready(h) && s.Restart(h)
}

func (s *Service) OnFired(h Handle, fn func(count int)) vm.Value {
	if s.hub == nil || fn == nil || s.timers.get(h) == nil {
		return vm.Value{}
	}
	return s.hub.Connect(handleSource(typeTimer, h.Key()), sigFired, func(a []vm.Value) { fn(int(a[0].Int())) })
}

func (s *Service) dropTimer(i uint32) {
	h := s.timers.handle(i)
	act := s.timers.s[i].v.act
	key := h.Key()
	s.timers.release(i)
	s.endHandle(handleSource(typeTimer, key))
	s.wake(typeTimer, key)
	if !act.IsZero() {
		s.stepDone(act, stepTimer, h)
	}
}

func (s *Service) stepTimers(dt float32, unscaled bool) {
	for i := range s.timers.s {
		e := &s.timers.s[i]
		if !e.live || e.v.unscaled != unscaled {
			continue
		}
		t := &e.v
		if !t.owner.IsZero() && !s.scene.IsAlive(t.owner) {
			s.dropTimer(uint32(i))
			continue
		}
		if t.done || t.paused || t.held {
			continue
		}
		t.moved = true
		t.left -= dt
		gen := e.gen
		for {
			e = &s.timers.s[i]
			t = &e.v
			if !e.live || e.gen != gen || t.done || t.left > 0 {
				break
			}
			t.count++
			h := s.timers.handle(uint32(i))
			key := h.Key()
			src := handleSource(typeTimer, key)
			count := vm.Int(int64(t.count))
			if t.legs > 0 {
				t.legs--
			}
			if t.legs == 0 || t.dur <= 0 {
				t.done, t.left = true, 0
				keep, act := t.keep, t.act
				if !keep {
					s.timers.release(uint32(i))
				}
				s.emitOne(src, sigFired, count)
				if !keep {
					s.endHandle(src)
				}
				s.wake(typeTimer, key)
				if !act.IsZero() {
					s.stepDone(act, stepTimer, h)
				}
				break
			}
			t.left += t.dur
			s.emitOne(src, sigFired, count)
			s.wake(typeTimer, key)
		}
	}
}
