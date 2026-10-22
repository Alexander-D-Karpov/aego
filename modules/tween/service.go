package tween

import (
	"aego/contracts/scriptfields"
	"aego/contracts/scriptwait"
	"aego/contracts/signals"
	"aego/core/curve"
	"aego/core/errs"
	"aego/core/mathx"
	"aego/engine"
	"aego/scene"
	"aego/schema"
	"aego/script/vm"
)

type Options struct {
	Ease     curve.Easing
	Curve    *curve.Curve
	Delay    float32
	Legs     int32
	PingPong bool
	Unscaled bool
}

type tween struct {
	node     scene.NodeHandle
	tgt      target
	from     [4]float32
	to       [4]float32
	dur      float32
	t        float32
	delay    float32
	legs     int32
	ease     curve.Easing
	cv       curve.Curve
	hasCurve bool
	ping     bool
	rev      bool
	started  bool
	unscaled bool
	act      Handle
	held     bool
	moved    bool
}

type Service struct {
	scene   *scene.Scene
	world   *engine.World
	reg     *schema.Registry
	short   map[string]schema.TypeID
	tweens  pool[tween]
	timers  pool[timer]
	actions pool[action]
	hub     signals.Hub
}

func NewService(s *scene.Scene, w *engine.World) *Service {
	svc := &Service{scene: s, world: w, reg: s.Registry(), short: map[string]schema.TypeID{}}
	for _, td := range svc.reg.Components() {
		n := schema.ShortName(td.FullName)
		if _, done := svc.short[n]; done {
			continue
		}
		svc.short[n] = 0
		if c, ok := svc.reg.ComponentNamed(n); ok {
			svc.short[n] = c.ID
		}
	}
	if w != nil {
		svc.hub, _ = engine.ServiceOf[signals.Hub](w, signals.ServiceID)
	}
	return svc
}

func handleSource(typ, key uint64) signals.Source {
	return signals.Source{Kind: signals.KindHandle, A: key, B: typ}
}

func (s *Service) emitOne(src signals.Source, sig int, v vm.Value) {
	if s.hub == nil || !s.hub.Wants(src) {
		return
	}
	args := s.hub.Args(1)
	args[0] = v
	s.hub.Emit(src, sig, args)
}

func (s *Service) endHandle(src signals.Source) {
	if s.hub != nil {
		s.hub.End(src)
	}
}

func (s *Service) tweenAlive(key uint64) bool {
	return s.tweens.get(HandleOf(key)) != nil
}

func (s *Service) timerAlive(key uint64) bool {
	return s.timers.get(HandleOf(key)) != nil
}

func (s *Service) OnCompleted(h Handle, fn func(finished bool)) vm.Value {
	if s.hub == nil || fn == nil || s.tweens.get(h) == nil {
		return vm.Value{}
	}
	return s.hub.Connect(handleSource(typeTween, h.Key()), sigCompleted, func(a []vm.Value) { fn(a[0].Bool()) })
}

func (s *Service) Disconnect(c vm.Value) {
	if s.hub != nil {
		s.hub.Disconnect(c)
	}
}

func (s *Service) Close() {
	s.tweens = pool[tween]{}
	s.timers = pool[timer]{}
	s.actions = pool[action]{}
}

func (s *Service) fields() scriptfields.Fields {
	if s.world == nil {
		return nil
	}
	f, _ := engine.ServiceOf[scriptfields.Fields](s.world, scriptfields.ServiceID)
	return f
}

func (s *Service) wake(typ, key uint64) {
	if s.world == nil {
		return
	}
	if w, ok := engine.ServiceOf[scriptwait.Waker](s.world, scriptwait.ServiceID); ok {
		w.Complete(typ, key)
	}
}

func (s *Service) Tweens() int {
	return s.tweens.n
}

func (s *Service) ToFloat(node scene.NodeHandle, path string, to, dur float32, o Options) (Handle, error) {
	return s.start(node, path, shFloat, [4]float32{to}, dur, &o)
}

func (s *Service) ToVec2(node scene.NodeHandle, path string, to mathx.Vec2, dur float32, o Options) (Handle, error) {
	return s.start(node, path, shVec2, [4]float32{to.X, to.Y}, dur, &o)
}

func (s *Service) ToVec3(node scene.NodeHandle, path string, to mathx.Vec3, dur float32, o Options) (Handle, error) {
	return s.start(node, path, shVec3, [4]float32{to.X, to.Y, to.Z}, dur, &o)
}

func (s *Service) ToColor(node scene.NodeHandle, path string, to mathx.Color, dur float32, o Options) (Handle, error) {
	return s.start(node, path, shVec4, [4]float32{to.R, to.G, to.B, to.A}, dur, &o)
}

func (s *Service) ToRect(node scene.NodeHandle, path string, to mathx.Rect, dur float32, o Options) (Handle, error) {
	return s.start(node, path, shVec4, [4]float32{to.X, to.Y, to.W, to.H}, dur, &o)
}

func (s *Service) ToQuat(node scene.NodeHandle, path string, to mathx.Quat, dur float32, o Options) (Handle, error) {
	return s.start(node, path, shQuat, [4]float32{to.X, to.Y, to.Z, to.W}, dur, &o)
}

func (s *Service) Position(node scene.NodeHandle, to mathx.Vec2, dur float32, o Options) (Handle, error) {
	return s.ToVec2(node, ownerTransform+".position", to, dur, o)
}

func (s *Service) start(node scene.NodeHandle, path string, want shape, to [4]float32, dur float32, o *Options) (Handle, error) {
	if !s.scene.IsAlive(node) {
		return Handle{}, errs.New(errs.SceneDeadNode, "tween: the node is nil or destroyed")
	}
	if dur != dur || dur < 0 || o.Delay != o.Delay || o.Delay < 0 {
		return Handle{}, badPath(path, "duration and delay must be zero or positive")
	}
	t, err := s.resolve(node, path)
	if err != nil {
		return Handle{}, err
	}
	if !fits(&t, want) {
		return Handle{}, badPath(path, "the value does not match the type of the property")
	}
	s.replace(node, &t)
	legs := normLegs(o.Legs)
	h, tw := s.tweens.alloc()
	keys := tw.cv.Keys[:0]
	*tw = tween{
		node: node, tgt: t, to: to, dur: dur, delay: o.Delay, legs: legs,
		ease: o.Ease, ping: o.PingPong, unscaled: o.Unscaled,
	}
	tw.cv.Keys = keys
	if o.Curve != nil {
		tw.cv.Ease, tw.cv.Pre, tw.cv.Post = o.Curve.Ease, o.Curve.Pre, o.Curve.Post
		tw.cv.Keys = append(keys, o.Curve.Keys...)
		tw.hasCurve = true
	}
	return h, nil
}

func (s *Service) replace(node scene.NodeHandle, t *target) {
	for i := range s.tweens.s {
		e := &s.tweens.s[i]
		if e.live && e.v.node == node && e.v.tgt.same(t) {
			s.finish(uint32(i), false)
		}
	}
}

func (s *Service) Stop(h Handle) {
	if s.tweens.get(h) != nil {
		s.finish(h.Index, false)
	}
}

func (s *Service) Done(h Handle) bool {
	return s.tweens.get(h) == nil
}

func (s *Service) Running(h Handle) bool {
	return !s.Done(h)
}

func (s *Service) finish(i uint32, finished bool) {
	h := s.tweens.handle(i)
	act := s.tweens.s[i].v.act
	key := h.Key()
	s.tweens.release(i)
	src := handleSource(typeTween, key)
	s.emitOne(src, sigCompleted, vm.Bool(finished))
	s.endHandle(src)
	s.wake(typeTween, key)
	if !act.IsZero() {
		s.stepDone(act, stepTween, h)
	}
}

func normLegs(n int32) int32 {
	switch {
	case n == 0:
		return 1
	case n < 0:
		return -1
	}
	return n
}

func (s *Service) SetDelay(h Handle, d float32) {
	if tw := s.tweens.get(h); tw != nil && !tw.started && d >= 0 && d == d {
		tw.delay = d
	}
}

func (s *Service) SetLegs(h Handle, legs int32, ping bool) {
	if tw := s.tweens.get(h); tw != nil {
		tw.legs, tw.ping = normLegs(legs), ping
	}
}

func (s *Service) SetUnscaled(h Handle, on bool) {
	if tw := s.tweens.get(h); tw != nil {
		tw.unscaled = on
	}
}

func (s *Service) SetCurve(h Handle, c *curve.Curve) {
	tw := s.tweens.get(h)
	if tw == nil || c == nil {
		return
	}
	tw.cv.Ease, tw.cv.Pre, tw.cv.Post = c.Ease, c.Pre, c.Post
	tw.cv.Keys = append(tw.cv.Keys[:0], c.Keys...)
	tw.hasCurve = true
}

func (s *Service) Step(dt float32, unscaled bool) {
	if dt != dt || dt < 0 {
		dt = 0
	}
	s.stepActions()
	for i := range s.tweens.s {
		e := &s.tweens.s[i]
		if e.live && e.v.unscaled == unscaled {
			s.advance(uint32(i), &e.v, dt)
		}
	}
	s.stepTimers(dt, unscaled)
}

func (s *Service) advance(i uint32, tw *tween, dt float32) {
	if !s.scene.IsAlive(tw.node) {
		s.finish(i, false)
		return
	}
	if tw.held {
		return
	}
	tw.moved = true
	if tw.delay > 0 {
		tw.delay -= dt
		if tw.delay > 0 {
			return
		}
		dt = -tw.delay
		tw.delay = 0
	}
	if !tw.started {
		from, ok := s.read(&tw.tgt, tw.node)
		if !ok {
			s.finish(i, false)
			return
		}
		tw.from, tw.started = from, true
	}
	tw.t += dt
	for tw.dur <= 0 || tw.t >= tw.dur {
		if tw.legs > 0 {
			tw.legs--
		}
		if tw.legs == 0 || tw.dur <= 0 {
			ok := s.write(&tw.tgt, tw.node, tw.end())
			s.finish(i, ok)
			return
		}
		tw.t -= tw.dur
		if tw.ping {
			tw.rev = !tw.rev
		}
	}
	if !s.write(&tw.tgt, tw.node, tw.at(tw.t/tw.dur)) {
		s.finish(i, false)
	}
}

func (s *Service) actionAlive(key uint64) bool {
	return s.actions.get(HandleOf(key)) != nil
}

func (tw *tween) end() [4]float32 {
	if tw.rev {
		return tw.from
	}
	return tw.to
}

func (tw *tween) eased(u float32) float32 {
	if tw.hasCurve {
		t0, t1 := tw.cv.Span()
		return tw.cv.EvalClamped(t0 + (t1-t0)*u)
	}
	if tw.ease == curve.None {
		return u
	}
	return curve.Ease(tw.ease, u)
}

func (tw *tween) at(u float32) [4]float32 {
	e := tw.eased(u)
	a, b := tw.from, tw.to
	if tw.rev {
		a, b = b, a
	}
	if tw.tgt.shape == shQuat && tw.tgt.lane < 0 {
		return nlerp(a, b, e)
	}
	var out [4]float32
	for i := range out {
		out[i] = a[i] + (b[i]-a[i])*e
	}
	return out
}

func nlerp(a, b [4]float32, t float32) [4]float32 {
	if a[0]*b[0]+a[1]*b[1]+a[2]*b[2]+a[3]*b[3] < 0 {
		for i := range b {
			b[i] = -b[i]
		}
	}
	var o [4]float32
	var n float32
	for i := range o {
		o[i] = a[i] + (b[i]-a[i])*t
		n += o[i] * o[i]
	}
	if n > 0 {
		k := 1 / mathx.Sqrt(n)
		for i := range o {
			o[i] *= k
		}
	}
	return o
}
