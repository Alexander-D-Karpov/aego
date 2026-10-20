package camera

import (
	"aego/contracts/scriptwait"
	"aego/contracts/signals"
	"aego/engine"
	"aego/script/abi"
	"aego/script/vm"
)

const (
	KindBlend    = ScriptModule + ".Blend"
	sigCompleted = 0
)

var typeBlend = abi.HandleID(KindBlend)

func (s *Service) endBlend(o *output, finished bool) {
	if !o.blend.on {
		return
	}
	key := o.blend.key
	o.blend.on = false
	src := signals.Source{Kind: signals.KindHandle, A: key, B: typeBlend}
	if s.hub != nil {
		if s.hub.Wants(src) {
			args := s.hub.Args(1)
			args[0] = vm.Bool(finished)
			s.hub.Emit(src, sigCompleted, args)
		}
		s.hub.End(src)
	}
	if w, ok := engine.ServiceOf[scriptwait.Waker](s.world, scriptwait.ServiceID); ok {
		w.Complete(typeBlend, key)
	}
}

func (s *Service) blendOutput(key uint64) *output {
	if key == 0 {
		return nil
	}
	for _, o := range s.outputs {
		if o.blend.on && o.blend.key == key {
			return o
		}
	}
	return nil
}

func (s *Service) blendAlive(key uint64) bool {
	return s.blendOutput(key) != nil
}

func (s *Service) BlendDone(key uint64) bool {
	return !s.blendAlive(key)
}

func (s *Service) StopBlend(key uint64) {
	if o := s.blendOutput(key); o != nil {
		s.endBlend(o, false)
	}
}

func (s *Service) OnBlendCompleted(key uint64, fn func(finished bool)) vm.Value {
	if s.hub == nil || fn == nil || !s.blendAlive(key) {
		return vm.Value{}
	}
	return s.hub.Connect(signals.Source{Kind: signals.KindHandle, A: key, B: typeBlend}, sigCompleted, func(a []vm.Value) { fn(a[0].Bool()) })
}
