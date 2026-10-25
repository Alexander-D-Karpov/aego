package animation

import (
	"slices"

	"aego/anim"
	"aego/contracts/physics2d"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/scene"
	"aego/scene/propclaim"
	"aego/scene/propref"
	"aego/schema"
)

type tmpRef struct {
	ref   propref.Ref
	node  scene.NodeHandle
	state int32
	track int32
}

type animator struct {
	node    scene.NodeHandle
	slot    int32
	ce      *ctrlEntry
	id      ids.AssetID
	gen     uint32
	binds   []binding
	prev    []binding
	vals    []anim.Value
	tracks  [][]int32
	samp    []anim.Sampler
	ids     [][]int32
	changes []anim.Change
	refs    []tmpRef
	root    []int32
	entered int32
	mark    uint32
}

func (s *Service) alookup(h scene.NodeHandle, a *Animator) *animator {
	if i := int(a.st.slot) - 1; i >= 0 && i < len(s.alist) {
		if r := s.alist[i]; r.node == h {
			return r
		}
	}
	r := s.anims[h]
	if r != nil {
		a.st.slot = r.slot
	}
	return r
}

func (s *Service) acreate(h scene.NodeHandle, a *Animator) *animator {
	r := &animator{node: h, slot: int32(len(s.alist) + 1), entered: -1}
	s.anims[h] = r
	s.alist = append(s.alist, r)
	a.st.slot = r.slot
	return r
}

func ctrlCompatible(a, b *ctrlEntry) bool {
	ga, gb := a.g, b.g
	if len(ga.States) != len(gb.States) || len(ga.Params) != len(gb.Params) {
		return false
	}
	for i := range ga.States {
		if ga.States[i].Name != gb.States[i].Name {
			return false
		}
	}
	for i := range ga.Params {
		if ga.Params[i].Name != gb.Params[i].Name || ga.Params[i].Kind != gb.Params[i].Kind {
			return false
		}
	}
	return true
}

func (s *Service) aloadOne(h scene.NodeHandle, a *Animator) {
	r := s.alookup(h, a)
	if r == nil {
		r = s.acreate(h, a)
		s.dirty = true
	}
	if a.PlayOnStart && !a.st.auto {
		a.st.auto, a.st.on = true, true
	}
	res := s.scene.Resolver()
	var gen uint32
	if res != nil {
		gen = res.Generation()
	}
	if r.id == a.Controller.ID && r.gen == gen {
		return
	}
	e := s.m.ctrl(res, a.Controller.ID)
	old, same := r.ce, r.id == a.Controller.ID
	r.id, r.gen = a.Controller.ID, gen
	switch {
	case e == old:
		return
	case same && e == nil:
		return
	case same && old != nil && !ctrlCompatible(old, e):
		if s.once(h, CodeReload, -1) {
			s.warn(h, CodeReload, "the controller was reimported with other states or parameters; the animator restarts")
		}
		e.reset(a)
	case e != nil:
		e.prepare(a)
	}
	r.ce = e
	r.ids = r.ids[:0]
	if e != nil {
		for st, clip := range e.clips {
			if clip == nil && !e.g.States[st].Clip.IsZero() && s.once(h, CodeCtrlClip, int32(st)) {
				s.warn(h, CodeCtrlClip, "state "+e.g.States[st].Name+": the clip is not loaded; the state plays nothing")
			}
			var list []int32
			if clip != nil {
				list = s.eventIDs(h, clip, nil)
			}
			r.ids = append(r.ids, list)
		}
	}
	s.dirty = true
}

func resizeTracks(t [][]int32, states, slots int) [][]int32 {
	if cap(t) < states {
		t = make([][]int32, states)
	}
	t = t[:states]
	for i := range t {
		if cap(t[i]) < slots {
			t[i] = make([]int32, slots)
		}
		t[i] = t[i][:slots]
		for j := range t[i] {
			t[i][j] = -1
		}
	}
	return t
}

func (s *Service) abindOne(h scene.NodeHandle, a *Animator) {
	if !s.scene.IsAlive(h) {
		return
	}
	r := s.alookup(h, a)
	if r == nil {
		r = s.acreate(h, a)
	}
	r.mark = s.mark
	r.prev, r.binds = r.binds, r.prev[:0]
	r.refs = r.refs[:0]
	if r.ce != nil {
		g := r.ce.g
		r.root = slices.Grow(r.root[:0], len(g.States))[:len(g.States)]
		for st, clip := range r.ce.clips {
			r.root[st] = -1
			if clip == nil {
				continue
			}
			for ti := range clip.Tracks {
				if clip.Tracks[ti].Root {
					r.root[st] = int32(ti)
					break
				}
			}
		}
		for st, clip := range r.ce.clips {
			if clip == nil {
				continue
			}
			for ti := range clip.Tracks {
				if !clip.Tracks[ti].Root {
					s.abindTrack(h, a, r, int32(st), int32(ti))
				}
			}
		}
		slices.SortFunc(r.binds, bindOrder)
		r.tracks = resizeTracks(r.tracks, len(g.States), len(r.binds))
		for _, t := range r.refs {
			for j := range r.binds {
				if r.binds[j].node == t.node && r.binds[j].ref.Same(&t.ref) {
					r.tracks[t.state][j] = t.track
					break
				}
			}
		}
		if len(r.samp) < len(g.States) {
			r.samp = make([]anim.Sampler, len(g.States))
		}
		for st, clip := range r.ce.clips {
			if clip != nil {
				r.samp[st].Reset(clip)
			}
		}
		for i := range r.binds {
			carry(&r.binds[i], r.prev)
		}
	}
	r.vals = sizeVals(r.vals, len(r.binds))
	for i := range r.prev {
		if o := &r.prev[i]; o.track >= 0 && o.shown && !posed(r.binds, o.node) {
			s.scene.DropPresentationLocal(o.node)
		}
	}
}

func (s *Service) abindTrack(h scene.NodeHandle, a *Animator, r *animator, st, ti int32) {
	t := &r.ce.clips[st].Tracks[ti]
	label := r.ce.g.States[st].Name + ": " + trackName(t)
	key := st*1024 + ti
	node := s.scene.Find(h, t.Target)
	if node.IsZero() || !s.scene.IsAlive(node) {
		if s.once(h, CodeLost, key) {
			s.warn(h, CodeLost, "track "+label+": the target node is missing")
		}
		return
	}
	ref, err := s.props.ResolveAny(node, t.Property)
	if err != nil {
		if s.once(h, CodeTrack, key) {
			s.warn(h, CodeTrack, "track "+label+": "+err.Error())
		}
		return
	}
	if !fits(t.Kind, &ref) {
		if s.once(h, CodeTrack, key) {
			s.warn(h, CodeTrack, "track "+label+" has kind "+t.Kind.String()+", which does not match the property")
		}
		return
	}
	if a.TimeDomain == DomainUnscaled && t.Authority == anim.AuthSimulation {
		if s.once(h, CodeUnscaled, key) {
			s.warn(h, CodeUnscaled, "track "+label+" has authority=simulation in an unscaled animator; it is off")
		}
		return
	}
	for j := range r.binds {
		if b := &r.binds[j]; b.node == node && b.ref.Same(&ref) {
			if b.kind != t.Kind {
				if s.once(h, CodeTrack, key) {
					s.warn(h, CodeTrack, "track "+label+" has kind "+t.Kind.String()+" here and "+b.kind.String()+" in another state; it is off here")
				}
				return
			}
			r.refs = append(r.refs, tmpRef{ref: ref, node: node, state: st, track: ti})
			return
		}
	}
	k := propclaim.KeyOf(node, &ref)
	for _, o := range s.owned[k] {
		if o.Overlaps(&ref) {
			if s.once(h, CodeConflict, key) {
				s.warn(h, CodeConflict, "track "+label+": another animation track drives this property; this one is off")
			}
			return
		}
	}
	s.owned[k] = append(s.owned[k], ref)
	r.refs = append(r.refs, tmpRef{ref: ref, node: node, state: st, track: ti})
	r.binds = append(r.binds, binding{
		ref: ref, key: k, label: label, node: node, kind: t.Kind, auth: t.Authority,
		pose: ref.Owner() == propref.OwnTransform && !ref.Three(),
	})
}

func (s *Service) astepOne(h scene.NodeHandle, a *Animator) {
	r := s.alookup(h, a)
	if r == nil || r.ce == nil {
		s.apublish(a, r)
		return
	}
	r.entered = -1
	if a.TimeDomain == DomainGame {
		s.acommands(h, a, r)
		if a.st.on {
			s.aadvance(h, a, r, s.dt)
		}
	}
	s.gate(h, r.binds)
	if a.TimeDomain == DomainGame {
		s.asample(a, r, 0, false)
		s.writeValues(r.binds, r.vals)
		s.writePoses(r.binds, r.vals, true)
	}
	s.claim(r.binds)
	s.apublish(a, r)
}

func (s *Service) apresentOne(h scene.NodeHandle, a *Animator) {
	r := s.alookup(h, a)
	if r == nil || r.ce == nil {
		return
	}
	if a.TimeDomain == DomainUnscaled {
		r.entered = -1
		s.acommands(h, a, r)
		if a.st.on {
			s.aadvance(h, a, r, s.unscaled)
		}
		s.asample(a, r, 0, false)
		s.writeValues(r.binds, r.vals)
		s.writePoses(r.binds, r.vals, false)
		s.apublish(a, r)
		return
	}
	var ahead float32
	if a.st.on {
		ahead = s.alpha * s.fixed * a.Speed
	}
	s.asample(a, r, ahead, true)
	s.writePoses(r.binds, r.vals, false)
}

func (s *Service) acommands(h scene.NodeHandle, a *Animator, r *animator) {
	cmd := a.st.cmd
	if cmd == 0 {
		return
	}
	a.st.cmd = 0
	a.st.on = true
	g := r.ce.g
	if cmd == acmdFade {
		r.changes = a.st.m.CrossFade(g, a.st.target, a.st.fade, r.changes[:0])
	} else {
		r.changes = a.st.m.Play(g, a.st.target, r.changes[:0])
	}
	s.changed(h, a, r)
}

func (s *Service) aadvance(h scene.NodeHandle, a *Animator, r *animator, dt float32) {
	g := r.ce.g
	n := r.ce.nParams()
	step := dt * a.Speed
	r.changes = a.st.m.Step(g, step, a.st.params[:n], r.changes[:0])
	g.Consume(a.st.params[:n])
	s.changed(h, a, r)
	m := &a.st.m
	if !m.Started {
		return
	}
	s.stateEvents(h, a, r, m.State, m.Last, m.Time, r.entered == m.State, true, step)
	if m.Blend && m.Prev >= 0 {
		s.stateEvents(h, a, r, m.Prev, m.PrevLast, m.PrevTime, false, false, step)
	}
	if a.RootMotion == RootNone {
		return
	}
	if a.TimeDomain != DomainGame {
		if s.once(h, CodeUnscaled, -4) {
			s.warn(h, CodeUnscaled, "root motion of an unscaled animator is off; it changes the game state")
		}
		return
	}
	s.rootMotion(h, a, r)
}

func (s *Service) rootDelta(r *animator, st int32, t0, t1 float64) [2]float32 {
	if st < 0 || int(st) >= len(r.root) || r.root[st] < 0 {
		return [2]float32{}
	}
	return anim.RootDelta(r.ce.clips[st], &r.samp[st], int(r.root[st]), r.ce.g.States[st].Loop, t0, t1)
}

func (s *Service) rootMotion(h scene.NodeHandle, a *Animator, r *animator) {
	if s.scene.Space(h) != scene.Space2D {
		return
	}
	m := &a.st.m
	d := s.rootDelta(r, m.State, m.Last, m.Time)
	if m.Blend {
		var o [2]float32
		if m.Prev >= 0 {
			o = s.rootDelta(r, m.Prev, m.PrevLast, m.PrevTime)
		}
		w := m.Weight(r.ce.g)
		d = [2]float32{o[0] + (d[0]-o[0])*w, o[1] + (d[1]-o[1])*w}
	}
	if d == ([2]float32{}) {
		return
	}
	local := mathx.Vec2{X: d[0] * a.RootScale.X, Y: d[1] * a.RootScale.Y}
	if a.RootMotion == RootToMotion {
		world := s.scene.World(s.scene.Parent(h)).ApplyVector(local)
		if s.rcv != nil && s.rcv.AddDisplacement(h, world) {
			return
		}
		if s.once(h, CodeRoot, -1) {
			s.warn(h, CodeRoot, "root motion needs a Motion 2D on the node to move it")
		}
		if !a.RootFallback {
			return
		}
	}
	s.rootTransform(h, local)
}

func (s *Service) rootTransform(h scene.NodeHandle, d mathx.Vec2) {
	ref := propref.TransformRef(s.scene, h, propref.PartPosition)
	if o, held := s.cl.Held(h, &ref, s.owner); held {
		if s.once(h, CodeConflict, -3) {
			s.warn(h, CodeConflict, "root motion: "+s.cl.Name(o)+" drives the position; use rootMotion toMotion")
		}
		return
	}
	if s.q != nil {
		if bt, ok := s.q.BodyType(h); ok && bt == physics2d.Dynamic {
			if s.once(h, CodeBody, -3) {
				s.warn(h, CodeBody, "root motion: a dynamic body moves this node; root motion is off")
			}
			return
		}
	}
	tr := s.scene.Local(h)
	tr.Position = tr.Position.Add(d)
	s.moveNode(h, tr)
	s.cl.Claim(s.owner, h, &ref)
}

func (s *Service) changed(h scene.NodeHandle, a *Animator, r *animator) {
	g := r.ce.g
	entered := false
	for _, ch := range r.changes {
		switch ch.Kind {
		case anim.ChangeEnter:
			entered = true
			r.entered = ch.State
			s.pend = append(s.pend, pending{node: h, name: g.States[ch.State].Name, id: ch.State, kind: pendEntered})
		case anim.ChangeExit:
			s.pend = append(s.pend, pending{node: h, name: g.States[ch.State].Name, id: ch.State, kind: pendExited})
		}
	}
	if !entered {
		return
	}
	blend := a.st.m.Blend
	for i := range r.binds {
		b := &r.binds[i]
		switch {
		case !b.live:
			b.base, b.hasBase = s.read(b)
			b.live, b.wrote = true, false
		case blend:
			b.base = s.current(b)
		}
	}
}

func dominates(g *anim.Graph, m *anim.Machine, pol anim.EventPolicy, incoming bool, tau, t0, t1 float64, step float32) bool {
	switch pol {
	case anim.EventsBoth:
		return true
	case anim.EventsIncoming:
		return incoming
	}
	frac := float32(1)
	if t1 != t0 {
		frac = float32((tau - t0) / (t1 - t0))
	}
	bt0 := max(m.BlendT-step, 0)
	w := m.WeightAt(g, bt0+frac*(m.BlendT-bt0))
	if incoming {
		return w >= 0.5
	}
	return w < 0.5
}

func (s *Service) stateEvents(h scene.NodeHandle, a *Animator, r *animator, st int32, t0, t1 float64, start, incoming bool, step float32) {
	clip := r.ce.clips[st]
	if clip == nil || len(clip.Events) == 0 {
		return
	}
	g := r.ce.g
	m := &a.st.m
	s.cross = anim.Crossings(clip, g.States[st].Loop, t0, t1, start, s.cross)
	pol := g.States[m.State].Events
	unscaled := a.TimeDomain == DomainUnscaled
	for _, x := range s.cross {
		e := &clip.Events[x.Event]
		id := r.ids[st][x.Event]
		if id < 0 || e.Mode == anim.EventPreview {
			continue
		}
		if m.Blend && !dominates(g, m, pol, incoming, x.T, t0, t1, step) {
			continue
		}
		pres := e.Mode == anim.EventPresentation || unscaled
		if pres && s.world.Replaying() {
			continue
		}
		if unscaled && e.Mode == anim.EventGameplay && s.once(h, CodeUnscaled, -2-x.Event) {
			s.warn(h, CodeUnscaled, "gameplay event "+e.Name+" of an unscaled animator is delivered as a presentation event")
		}
		s.pend = append(s.pend, pending{node: h, name: e.Name, id: id, payload: e.Payload, cycle: x.Cycle, anim: true, pres: pres})
	}
}

func (r *animator) value(st int32, slot int, t float64) (anim.Value, bool) {
	ti := r.tracks[st][slot]
	if ti < 0 {
		return anim.Value{}, false
	}
	clip := r.ce.clips[st]
	local, _ := clip.Local(r.ce.g.States[st].Loop, t)
	return r.samp[st].Track(clip, int(ti), local), true
}

func mix(k schema.Kind, a, b anim.Value, w float32) anim.Value {
	switch {
	case k == schema.KindQuat:
		return anim.Lanes(anim.Slerp(a.L, b.L, w))
	case anim.Interpolates(k):
		var v anim.Value
		for i := range v.L {
			v.L[i] = a.L[i] + (b.L[i]-a.L[i])*w
		}
		return v
	case w >= 0.5:
		return b
	}
	return a
}

func (s *Service) asample(a *Animator, r *animator, ahead float32, poseOnly bool) {
	g := r.ce.g
	m := &a.st.m
	if !m.Started {
		for i := range r.binds {
			r.binds[i].idle = true
		}
		return
	}
	w := float32(1)
	if m.Blend {
		w = m.WeightAt(g, m.BlendT+ahead)
	}
	cur := m.Time + float64(ahead)*float64(g.States[m.State].Speed)
	var prev float64
	if m.Blend && m.Prev >= 0 {
		prev = m.PrevTime + float64(ahead)*float64(g.States[m.Prev].Speed)
	}
	for i := range r.binds {
		b := &r.binds[i]
		if poseOnly && !b.pose {
			continue
		}
		b.idle = false
		if !b.live || b.blocked {
			continue
		}
		in, ok := r.value(m.State, i, cur)
		if !m.Blend {
			if ok {
				r.vals[i] = in
			} else {
				b.idle = true
			}
			continue
		}
		if !ok {
			in = b.base
		}
		out := b.base
		if m.Prev >= 0 {
			if v, ok := r.value(m.Prev, i, prev); ok {
				out = v
			}
		}
		r.vals[i] = mix(b.kind, out, in, w)
	}
}

func (s *Service) apublish(a *Animator, r *animator) {
	m := &a.st.m
	a.Blending = m.Blend
	if r == nil || r.ce == nil || !m.Started {
		a.State, a.Time, a.Progress = "", 0, 0
		return
	}
	g := r.ce.g
	a.State = g.States[m.State].Name
	a.Time = float32(m.Time)
	a.Progress = m.Progress(g)
}
