package animation

import (
	"math"

	"aego/anim"
	"aego/contracts/physics2d"
	"aego/contracts/scriptevents"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine/recorder"
	"aego/scene"
	"aego/scene/propclaim"
	"aego/scene/propref"
	"aego/script/vm"
)

const (
	detailPresentation = "presentation"
	detailEnter        = "enter"
	detailExit         = "exit"
	nameFinished       = "Finished"
)

func loopOf(c *AnimationPlayer, clip *anim.Clip) anim.LoopMode {
	switch c.Loop {
	case LoopOnce:
		return anim.LoopOnce
	case LoopRepeat:
		return anim.LoopRepeat
	case LoopPingPong:
		return anim.LoopPingPong
	}
	return clip.Loop
}

func ended(loop anim.LoopMode, speed float32, t, l float64) bool {
	return loop == anim.LoopOnce && ((speed > 0 && t >= l) || (speed < 0 && t <= 0))
}

func (s *Service) Fixed(dt float32) {
	s.refreshProperties()
	s.dt = dt
	s.q = s.queries()
	s.rcv = s.receiver()
	scene.Query1[AnimationPlayer](s.scene).Each(s.loadFn)
	scene.Query1[Animator](s.scene).Each(s.aloadFn)
	if s.dirty {
		s.rebind()
	}
	s.cl.Begin(s.owner)
	scene.Query1[AnimationPlayer](s.scene).Each(s.stepFn)
	scene.Query1[Animator](s.scene).Each(s.astepFn)
	s.cl.End(s.owner)
	s.deliver()
}

func (s *Service) stepOne(h scene.NodeHandle, c *AnimationPlayer) {
	p := s.lookup(h, c)
	if p == nil {
		return
	}
	s.commands(h, c, p)
	if p.clip != nil {
		if c.st.playing && !c.st.paused && c.TimeDomain == DomainGame {
			s.advance(h, c, p)
		}
		s.gate(h, p.binds)
		if c.TimeDomain == DomainGame {
			local, _ := p.clip.Local(loopOf(c, p.clip), c.st.time)
			s.samplePlayer(p, local, false)
			s.writeValues(p.binds, p.vals)
			s.writePoses(p.binds, p.vals, true)
		}
		s.claim(p.binds)
	}
	s.publish(c, p)
}

func (s *Service) samplePlayer(p *player, local float32, poseOnly bool) {
	for i := range p.binds {
		if b := &p.binds[i]; b.live && !b.blocked && (!poseOnly || b.pose) {
			p.vals[i] = p.sampler.Track(p.clip, int(b.track), local)
		}
	}
}

func (s *Service) commands(h scene.NodeHandle, c *AnimationPlayer, p *player) {
	cmd := c.st.cmd
	if cmd == 0 {
		return
	}
	c.st.cmd = 0
	if cmd&cmdStop != 0 && c.st.playing {
		s.end(h, c, p, -1)
	}
	if cmd&cmdPlay != 0 {
		if p.clip == nil {
			c.st.cmd |= cmdPlay
		} else {
			s.start(c, p)
		}
	}
	if cmd&cmdPause != 0 && c.st.playing {
		c.st.paused = true
	}
	if cmd&cmdResume != 0 {
		c.st.paused = false
	}
	if cmd&cmdSeek != 0 && p.clip != nil {
		s.seek(c, p)
	}
}

func (s *Service) start(c *AnimationPlayer, p *player) {
	t := 0.0
	if c.Speed < 0 && loopOf(c, p.clip) == anim.LoopOnce {
		t = float64(p.clip.Length)
	}
	c.st.time, c.st.shown = t, t
	c.st.playing, c.st.paused, c.st.started = true, false, true
	c.st.first, c.st.pfirst = true, true
	for i := range p.binds {
		b := &p.binds[i]
		if !b.live {
			b.base, b.hasBase = s.read(b)
		}
		b.live, b.wrote = true, false
	}
}

func (s *Service) seek(c *AnimationPlayer, p *player) {
	l := float64(p.clip.Length)
	to := min(max(float64(c.st.seek), 0), l)
	if loopOf(c, p.clip) != anim.LoopOnce && l > 0 {
		to += math.Floor(c.st.time/l) * l
	}
	c.st.time, c.st.shown = to, to
	for i := range p.binds {
		p.binds[i].wrote = false
	}
}

func (s *Service) end(h scene.NodeHandle, c *AnimationPlayer, p *player, cycle int64) {
	c.st.playing, c.st.paused = false, false
	s.pend = append(s.pend, pending{node: h, cycle: cycle, kind: pendFinished})
	for i := range p.binds {
		b := &p.binds[i]
		if !b.live || b.end != anim.EndRestore {
			continue
		}
		b.live = false
		if b.hasBase && !(b.pose && b.auth == anim.AuthPresentation) {
			b.wrote = false
			s.write(b, b.base)
		}
	}
}

func (s *Service) advance(h scene.NodeHandle, c *AnimationPlayer, p *player) {
	clip := p.clip
	loop := loopOf(c, clip)
	l := float64(clip.Length)
	t0 := c.st.time
	t1 := t0 + float64(s.dt)*float64(c.Speed)
	if loop == anim.LoopOnce {
		t1 = min(max(t1, 0), l)
	}
	if len(clip.Events) > 0 {
		s.cross = anim.Crossings(clip, loop, t0, t1, c.st.first, s.cross)
		for _, x := range s.cross {
			e := &clip.Events[x.Event]
			if id := p.ids[x.Event]; id >= 0 && e.Mode == anim.EventGameplay {
				s.pend = append(s.pend, pending{node: h, name: e.Name, id: id, payload: e.Payload, cycle: x.Cycle})
			}
		}
	}
	c.st.first = false
	c.st.time = t1
	if ended(loop, c.Speed, t1, l) {
		s.end(h, c, p, 0)
	}
}

func (s *Service) gate(h scene.NodeHandle, binds []binding) {
	rev := s.cl.Revision() + 1
	for i := range binds {
		b := &binds[i]
		b.blocked = false
		if !b.live {
			continue
		}
		if b.hrev != rev {
			b.hrev = rev
			b.held, _ = s.cl.HeldKey(b.key, &b.ref, s.owner)
		}
		if b.held != propclaim.NoOwner {
			b.blocked = true
			if s.once(h, CodeConflict, b.track) {
				s.warn(h, CodeConflict, "track "+b.labelText()+": "+s.cl.Name(b.held)+" drives this property; the track is off")
			}
			continue
		}
		if s.q == nil || b.ref.Owner() != propref.OwnTransform {
			continue
		}
		bt, ok := s.q.BodyType(b.node)
		if !ok {
			continue
		}
		switch {
		case bt == physics2d.Dynamic:
			b.blocked = true
			if s.once(h, CodeBody, b.track) {
				s.warn(h, CodeBody, "track "+b.labelText()+": a dynamic body moves this node; its transform tracks are off")
			}
		case bt == physics2d.Kinematic && b.auth == anim.AuthPresentation:
			b.blocked = true
			if s.once(h, CodeBody, b.track) {
				s.warn(h, CodeBody, "track "+b.labelText()+": a kinematic body moves this node; animate it with authority=simulation")
			}
		case bt == physics2d.Static && b.auth == anim.AuthSimulation:
			if s.once(h, CodeBody, b.track) {
				s.warn(h, CodeBody, "track "+b.labelText()+": a static body is teleported on every step; make it kinematic")
			}
		}
	}
}

func groupEnd(binds []binding, i int) int {
	j := i + 1
	for j < len(binds) && binds[j].pose && binds[j].node == binds[i].node {
		j++
	}
	return j
}

func (s *Service) writeValues(binds []binding, vals []anim.Value) {
	for i := range binds {
		if b := &binds[i]; !b.pose && b.live && !b.blocked && !b.idle {
			s.write(b, vals[i])
		}
	}
}

func (s *Service) writePoses(binds []binding, vals []anim.Value, sim bool) {
	for i := 0; i < len(binds); {
		if !binds[i].pose {
			i++
			continue
		}
		j := groupEnd(binds, i)
		s.writePose(binds, vals, i, j, sim)
		i = j
	}
}

func setVec(dst *mathx.Vec2, lane int, l [4]float32) {
	switch lane {
	case -1:
		*dst = mathx.Vec2{X: l[0], Y: l[1]}
	case 0:
		dst.X = l[0]
	case 1:
		dst.Y = l[0]
	}
}

func applyPose(tr *scene.Transform2D, b *binding, v anim.Value) {
	switch b.ref.Part() {
	case propref.PartPosition:
		setVec(&tr.Position, b.ref.Lane(), v.L)
	case propref.PartRotation:
		tr.Rotation = v.L[0]
	default:
		setVec(&tr.Scale, b.ref.Lane(), v.L)
	}
}

func (s *Service) writePose(binds []binding, vals []anim.Value, i, j int, sim bool) {
	want := anim.AuthPresentation
	if sim {
		want = anim.AuthSimulation
	}
	node := binds[i].node
	tr := s.scene.Local(node)
	hit := false
	for k := i; k < j; k++ {
		b := &binds[k]
		if !b.live || b.blocked || b.idle || b.auth != want {
			continue
		}
		applyPose(&tr, b, vals[k])
		hit = true
	}
	if sim {
		if hit {
			s.moveNode(node, tr)
		}
		return
	}
	shown := false
	for k := i; k < j; k++ {
		shown = shown || binds[k].shown
		binds[k].shown = false
	}
	switch {
	case hit:
		s.scene.SetPresentationLocal(node, tr)
		binds[i].shown = true
	case shown:
		s.scene.DropPresentationLocal(node)
	}
}

func (s *Service) moveNode(node scene.NodeHandle, tr scene.Transform2D) {
	if s.q != nil {
		if bt, ok := s.q.BodyType(node); ok && bt == physics2d.Kinematic {
			pw := s.scene.World(s.scene.Parent(node))
			_ = s.q.MoveKinematic(node, pw.Apply(tr.Position), tr.Rotation+mathx.Atan2(pw[1], pw[0]), s.dt)
			return
		}
	}
	s.scene.SetLocal(node, tr)
}

func (s *Service) claim(binds []binding) {
	for i := range binds {
		if b := &binds[i]; b.live && !b.blocked {
			s.cl.ClaimAt(s.owner, b.key, &b.ref, &b.slot)
		}
	}
}

func (s *Service) publish(c *AnimationPlayer, p *player) {
	c.Playing = c.st.playing && !c.st.paused
	if p.clip == nil {
		c.Time, c.Cycle = 0, 0
		return
	}
	c.Time, c.Cycle = p.clip.Local(loopOf(c, p.clip), c.st.time)
}

func (s *Service) read(b *binding) (anim.Value, bool) {
	switch b.ref.Shape() {
	case propref.Discrete:
		i, a, ok := s.props.ReadDiscrete(&b.ref, b.node)
		return anim.Value{I: i, A: a}, ok
	case propref.Int:
		i, ok := s.props.ReadInt(&b.ref, b.node)
		return anim.Int(i), ok
	}
	l, ok := s.props.Read(&b.ref, b.node)
	return anim.Lanes(l), ok
}

func (s *Service) current(b *binding) anim.Value {
	if b.wrote {
		return b.last
	}
	v, _ := s.read(b)
	return v
}

func (s *Service) write(b *binding, v anim.Value) {
	if b.wrote && b.last == v {
		return
	}
	b.last, b.wrote = v, true
	switch b.ref.Shape() {
	case propref.Discrete:
		s.props.WriteDiscrete(&b.ref, b.node, v.I, v.A)
	case propref.Int:
		s.props.WriteInt(&b.ref, b.node, v.I)
	default:
		s.props.Write(&b.ref, b.node, v.L)
	}
}

func (s *Service) deliver() {
	if len(s.pend) == 0 {
		return
	}
	sk := s.sink()
	for i := range s.pend {
		s.emit(sk, &s.pend[i])
	}
	s.pend = s.pend[:0]
}

func (s *Service) record(h scene.NodeHandle, name, detail string) {
	if r := s.world.Recorder; r != nil {
		r.Add(recorder.Animation, s.scene.ID(h), ids.NodeID{}, name, detail)
	}
}

func (s *Service) emit(sk scriptevents.Sink, e *pending) {
	if s.world == nil {
		return
	}
	w, h := s.world, e.node
	switch e.kind {
	case pendFinished:
		s.m.sig.finished.Emit(w, h, e.cycle)
		s.record(h, nameFinished, "")
		return
	case pendEntered:
		s.m.asig.entered.Emit(w, h, int64(e.id))
		s.record(h, e.name, detailEnter)
		return
	case pendExited:
		s.m.asig.exited.Emit(w, h, int64(e.id))
		s.record(h, e.name, detailExit)
		return
	}
	if e.anim {
		s.m.asig.event.Emit(w, h, int64(e.id), e.payload)
	} else {
		s.m.sig.event.Emit(w, h, int64(e.id), e.payload)
	}
	if sk != nil && s.m.ev != scriptevents.NoEvent {
		s.args[0], s.args[1] = vm.Int(int64(e.id)), vm.Float(e.payload)
		sk.Emit(h, s.m.ev, s.args[:])
	}
	detail := ""
	if e.pres {
		detail = detailPresentation
	}
	s.record(h, e.name, detail)
}

func (s *Service) Present(alpha, fixed, unscaled float32) {
	s.refreshProperties()
	s.alpha, s.fixed, s.unscaled = alpha, fixed, unscaled
	scene.Query1[AnimationPlayer](s.scene).Each(s.presentFn)
	scene.Query1[Animator](s.scene).Each(s.apresentFn)
	s.deliver()
}

func (s *Service) presentOne(h scene.NodeHandle, c *AnimationPlayer) {
	p := s.lookup(h, c)
	if p == nil || p.clip == nil || !c.st.started {
		return
	}
	loop := loopOf(c, p.clip)
	l := float64(p.clip.Length)
	run := c.st.playing && !c.st.paused
	if c.TimeDomain == DomainUnscaled {
		if run {
			t := c.st.time + float64(s.unscaled)*float64(c.Speed)
			if loop == anim.LoopOnce {
				t = min(max(t, 0), l)
			}
			s.presentEvents(h, c, p, loop, c.st.time, t, c.st.first)
			c.st.first, c.st.time = false, t
			if ended(loop, c.Speed, t, l) {
				s.end(h, c, p, 0)
			}
		}
		local, _ := p.clip.Local(loop, c.st.time)
		s.samplePlayer(p, local, false)
		s.writeValues(p.binds, p.vals)
		s.writePoses(p.binds, p.vals, false)
		s.publish(c, p)
		return
	}
	t := c.st.time
	if run {
		t += float64(s.alpha) * float64(s.fixed) * float64(c.Speed)
		if loop == anim.LoopOnce {
			t = min(max(t, 0), l)
		}
	}
	if !s.world.Replaying() {
		s.presentEvents(h, c, p, loop, c.st.shown, t, c.st.pfirst)
	}
	c.st.pfirst, c.st.shown = false, t
	local, _ := p.clip.Local(loop, t)
	s.samplePlayer(p, local, true)
	s.writePoses(p.binds, p.vals, false)
}

func (s *Service) presentEvents(h scene.NodeHandle, c *AnimationPlayer, p *player, loop anim.LoopMode, t0, t1 float64, first bool) {
	if len(p.clip.Events) == 0 {
		return
	}
	s.cross = anim.Crossings(p.clip, loop, t0, t1, first, s.cross)
	if len(s.cross) == 0 {
		return
	}
	sk := s.sink()
	unscaled := c.TimeDomain == DomainUnscaled
	for _, x := range s.cross {
		e := &p.clip.Events[x.Event]
		id := p.ids[x.Event]
		switch {
		case id < 0, e.Mode == anim.EventPreview:
			continue
		case e.Mode == anim.EventGameplay && !unscaled:
			continue
		case e.Mode == anim.EventGameplay && s.once(h, CodeUnscaled, -2-x.Event):
			s.warn(h, CodeUnscaled, "gameplay event "+e.Name+" of an unscaled player is delivered as a presentation event")
		}
		ev := pending{node: h, name: e.Name, id: id, payload: e.Payload, cycle: x.Cycle, pres: true}
		s.emit(sk, &ev)
	}
}

// Resource generation changes are resolved before sampling. A removed field
// invalidates its binding; retained fields continue at the current play time.
func (s *Service) refreshProperties() {
	for _, p := range s.list {
		for i := range p.binds {
			b := &p.binds[i]
			if b.track >= 0 && !s.props.Refresh(&b.ref, b.node) {
				s.dirty = true
			}
		}
	}
	for _, p := range s.alist {
		for i := range p.binds {
			b := &p.binds[i]
			if b.track >= 0 && !s.props.Refresh(&b.ref, b.node) {
				s.dirty = true
			}
		}
	}
}
