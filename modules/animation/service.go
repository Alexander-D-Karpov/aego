package animation

import (
	"cmp"
	"errors"
	"slices"

	"aego/anim"
	"aego/asset"
	"aego/contracts/motion2d"
	"aego/contracts/physics2d"
	"aego/contracts/scriptevents"
	"aego/contracts/scriptfields"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/rewind"
	"aego/engine"
	"aego/scene"
	"aego/scene/propclaim"
	"aego/scene/propref"
	"aego/schema"
	"aego/script/vm"
)

var errNoClip = errors.New("the clip is not loaded")

type binding struct {
	ref     propref.Ref
	key     propclaim.Key
	slot    propclaim.Slot
	label   string
	node    scene.NodeHandle
	kind    schema.Kind
	track   int32
	auth    anim.Authority
	end     anim.OnEnd
	base    anim.Value
	last    anim.Value
	hrev    uint64
	held    propclaim.Owner
	hasBase bool
	wrote   bool
	live    bool
	blocked bool
	idle    bool
	pose    bool
	shown   bool
}

type player struct {
	node    scene.NodeHandle
	slot    int32
	clip    *anim.Clip
	id      ids.AssetID
	gen     uint32
	ids     []int32
	binds   []binding
	prev    []binding
	sampler anim.Sampler
	vals    []anim.Value
	mark    uint32
}

const (
	pendEvent uint8 = iota
	pendFinished
	pendEntered
	pendExited
)

type pending struct {
	node    scene.NodeHandle
	name    string
	id      int32
	payload float32
	cycle   int64
	kind    uint8
	anim    bool
	pres    bool
}

type warnKey struct {
	node  scene.NodeHandle
	code  string
	track int32
}

type Service struct {
	scene      *scene.Scene
	world      *engine.World
	m          *Module
	props      *propref.Resolver
	cl         *propclaim.Table
	owner      propclaim.Owner
	log        *log.ChannelLogger
	q          physics2d.Queries
	rcv        motion2d.Receiver
	unsub      func()
	players    map[scene.NodeHandle]*player
	list       []*player
	owned      map[propclaim.Key][]propref.Ref
	warned     map[warnKey]bool
	pend       []pending
	cross      []anim.Crossing
	args       [2]vm.Value
	key        *animKey
	hh         *rewind.Hasher
	dirty      bool
	mark       uint32
	dt         float32
	alpha      float32
	fixed      float32
	unscaled   float32
	loadFn     func(scene.NodeHandle, *AnimationPlayer)
	bindFn     func(scene.NodeHandle, *AnimationPlayer)
	stepFn     func(scene.NodeHandle, *AnimationPlayer)
	presentFn  func(scene.NodeHandle, *AnimationPlayer)
	captureFn  func(scene.NodeHandle, *AnimationPlayer)
	hashFn     func(scene.NodeHandle, *AnimationPlayer)
	anims      map[scene.NodeHandle]*animator
	alist      []*animator
	aloadFn    func(scene.NodeHandle, *Animator)
	abindFn    func(scene.NodeHandle, *Animator)
	astepFn    func(scene.NodeHandle, *Animator)
	apresentFn func(scene.NodeHandle, *Animator)
	acaptureFn func(scene.NodeHandle, *Animator)
	ahashFn    func(scene.NodeHandle, *Animator)
}

func NewService(s *scene.Scene, w *engine.World, m *Module) *Service {
	svc := &Service{scene: s, world: w, m: m, dirty: true,
		players: map[scene.NodeHandle]*player{}, anims: map[scene.NodeHandle]*animator{}, owned: map[propclaim.Key][]propref.Ref{}, warned: map[warnKey]bool{}}
	svc.props = propref.New(s, svc.fields)
	if w != nil {
		svc.cl = propclaim.Ensure(w)
		svc.owner = svc.cl.Register(ID)
	}
	svc.unsub = s.Events().Subscribe(svc.onEvent)
	svc.loadFn, svc.bindFn, svc.stepFn, svc.presentFn = svc.loadOne, svc.bindOne, svc.stepOne, svc.presentOne
	svc.captureFn, svc.hashFn = svc.captureOne, svc.hashOne
	svc.aloadFn, svc.abindFn, svc.astepFn, svc.apresentFn = svc.aloadOne, svc.abindOne, svc.astepOne, svc.apresentOne
	svc.acaptureFn, svc.ahashFn = svc.acaptureOne, svc.ahashOne
	return svc
}

func (s *Service) Close() {
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
	s.cl.Drop(s.owner)
	for _, p := range s.players {
		s.unbind(p.binds)
	}
	for _, r := range s.anims {
		s.unbind(r.binds)
	}
	clear(s.players)
	clear(s.anims)
	clear(s.list)
	clear(s.alist)
	s.list, s.alist = s.list[:0], s.alist[:0]
}

func (s *Service) onEvent(ev scene.Event) {
	switch ev.Kind {
	case scene.EvNodeCreated, scene.EvNodeDestroyed, scene.EvNodeReparented, scene.EvNodeRenamed,
		scene.EvComponentAdded, scene.EvComponentRemoved, scene.EvSpaceChanged:
		s.dirty = true
	}
}

func (s *Service) fields() scriptfields.Fields {
	if s.world == nil {
		return nil
	}
	f, _ := engine.ServiceOf[scriptfields.Fields](s.world, scriptfields.ServiceID)
	return f
}

func (s *Service) queries() physics2d.Queries {
	if s.world == nil {
		return nil
	}
	q, _ := engine.ServiceOf[physics2d.Queries](s.world, physics2d.ServiceID)
	return q
}

func (s *Service) receiver() motion2d.Receiver {
	if s.world == nil {
		return nil
	}
	r, _ := engine.ServiceOf[motion2d.Receiver](s.world, motion2d.ServiceID)
	return r
}

func (s *Service) sink() scriptevents.Sink {
	if s.world == nil {
		return nil
	}
	sk, _ := engine.ServiceOf[scriptevents.Sink](s.world, scriptevents.ServiceID)
	return sk
}

func (s *Service) once(owner scene.NodeHandle, code string, track int32) bool {
	k := warnKey{node: owner, code: code, track: track}
	if s.warned[k] {
		return false
	}
	s.warned[k] = true
	return true
}

func (s *Service) warn(owner scene.NodeHandle, code, msg string) {
	if s.log != nil {
		s.log.Warn(code+": "+msg, log.F("node", s.scene.Path(owner)))
	}
}

func trackName(t *anim.Track) string {
	if t.Target == "" {
		return t.Property
	}
	return t.Target + ": " + t.Property
}

func (m *Module) clip(res asset.Resolver, id ids.AssetID) *anim.Clip {
	if res == nil || id.IsZero() {
		return nil
	}
	dec, ok := res.(asset.Decoder)
	src, ok2 := res.(asset.AnimSource)
	if !ok || !ok2 {
		return nil
	}
	v, err := dec.Decoded(id, "anim.clip", func() (any, error) {
		data, ok := src.Anim(id)
		if !ok {
			return nil, errNoClip
		}
		c, err := anim.Unmarshal(data)
		if err != nil {
			return nil, err
		}
		return c, nil
	})
	if err != nil {
		return nil
	}
	c, _ := v.(*anim.Clip)
	return c
}

func compatible(a, b *anim.Clip) bool {
	if b == nil || len(a.Tracks) != len(b.Tracks) {
		return false
	}
	for i := range a.Tracks {
		x, y := &a.Tracks[i], &b.Tracks[i]
		if x.Target != y.Target || x.Property != y.Property || x.Kind != y.Kind {
			return false
		}
	}
	return true
}

func (s *Service) eventIDs(h scene.NodeHandle, clip *anim.Clip, dst []int32) []int32 {
	if clip == nil {
		return dst
	}
	for i := range clip.Events {
		id, ok := s.m.EventID(clip.Events[i].Name)
		if !ok {
			id = -1
			if s.once(h, CodeEvent, int32(i)) {
				s.warn(h, CodeEvent, "event "+clip.Events[i].Name+" is not declared in Project Settings → Animation")
			}
		}
		dst = append(dst, id)
	}
	return dst
}

func (s *Service) lookup(h scene.NodeHandle, c *AnimationPlayer) *player {
	if i := int(c.st.slot) - 1; i >= 0 && i < len(s.list) {
		if p := s.list[i]; p.node == h {
			return p
		}
	}
	p := s.players[h]
	if p != nil {
		c.st.slot = p.slot
	}
	return p
}

func (s *Service) create(h scene.NodeHandle, c *AnimationPlayer) *player {
	p := &player{node: h, slot: int32(len(s.list) + 1)}
	s.players[h] = p
	s.list = append(s.list, p)
	c.st.slot = p.slot
	return p
}

func (s *Service) loadOne(h scene.NodeHandle, c *AnimationPlayer) {
	p := s.lookup(h, c)
	if p == nil {
		p = s.create(h, c)
		s.dirty = true
	}
	if c.PlayOnStart && !c.st.auto {
		c.st.auto = true
		c.st.cmd |= cmdPlay
	}
	res := s.scene.Resolver()
	var gen uint32
	if res != nil {
		gen = res.Generation()
	}
	if p.id == c.Clip.ID && p.gen == gen {
		return
	}
	clip := s.m.clip(res, c.Clip.ID)
	old, same := p.clip, p.id == c.Clip.ID
	p.id, p.gen = c.Clip.ID, gen
	switch {
	case clip == old:
		return
	case same && clip == nil:
		return
	case same && old != nil && !compatible(old, clip):
		if s.once(h, CodeReload, -1) {
			s.warn(h, CodeReload, "the clip was reimported with other tracks; the player stopped")
		}
		if c.st.playing {
			c.st.playing, c.st.paused = false, false
			s.pend = append(s.pend, pending{node: h, cycle: -1, kind: pendFinished})
		}
	case same && old != nil && old.Length > 0:
		k := float64(clip.Length) / float64(old.Length)
		c.st.time *= k
		c.st.shown *= k
	}
	p.clip = clip
	p.ids = s.eventIDs(h, clip, p.ids[:0])
	s.dirty = true
}

func (s *Service) rebind() {
	s.dirty = false
	s.mark++
	clear(s.owned)
	scene.Store[AnimationPlayer](s.scene).EachAll(s.bindFn)
	scene.Store[Animator](s.scene).EachAll(s.abindFn)
	clear(s.list)
	s.list = s.list[:0]
	for h, p := range s.players {
		if p.mark != s.mark {
			s.unbind(p.binds)
			delete(s.players, h)
			continue
		}
		p.slot = int32(len(s.list) + 1)
		s.list = append(s.list, p)
	}
	clear(s.alist)
	s.alist = s.alist[:0]
	for h, r := range s.anims {
		if r.mark != s.mark {
			s.unbind(r.binds)
			delete(s.anims, h)
			continue
		}
		r.slot = int32(len(s.alist) + 1)
		s.alist = append(s.alist, r)
	}
}

func (s *Service) unbind(binds []binding) {
	for i := range binds {
		if binds[i].shown {
			s.scene.DropPresentationLocal(binds[i].node)
		}
	}
}

func sizeVals(v []anim.Value, n int) []anim.Value {
	if cap(v) < n {
		return make([]anim.Value, n)
	}
	return v[:n]
}

func bindOrder(a, b binding) int {
	switch {
	case a.node.Index != b.node.Index:
		return cmp.Compare(a.node.Index, b.node.Index)
	case a.node.Gen != b.node.Gen:
		return cmp.Compare(a.node.Gen, b.node.Gen)
	case a.pose != b.pose:
		if a.pose {
			return -1
		}
		return 1
	}
	return cmp.Compare(a.track, b.track)
}

func carry(dst *binding, prev []binding) {
	for i := range prev {
		o := &prev[i]
		if o.track == dst.track && o.node == dst.node && o.ref.Same(&dst.ref) {
			dst.base, dst.hasBase, dst.last, dst.wrote = o.base, o.hasBase, o.last, o.wrote
			dst.live, dst.shown = o.live, o.shown
			o.track = -1
			return
		}
	}
}

func posed(binds []binding, node scene.NodeHandle) bool {
	for i := range binds {
		if binds[i].pose && binds[i].node == node {
			return true
		}
	}
	return false
}

func (s *Service) bindOne(h scene.NodeHandle, c *AnimationPlayer) {
	if !s.scene.IsAlive(h) {
		return
	}
	p := s.lookup(h, c)
	if p == nil {
		p = s.create(h, c)
	}
	p.mark = s.mark
	p.prev, p.binds = p.binds, p.prev[:0]
	if p.clip != nil {
		p.sampler.Reset(p.clip)
		for i := range p.clip.Tracks {
			s.bindTrack(h, c, p, int32(i))
		}
		slices.SortFunc(p.binds, bindOrder)
		for i := range p.binds {
			carry(&p.binds[i], p.prev)
		}
	}
	for i := range p.prev {
		if o := &p.prev[i]; o.track >= 0 && o.shown && !posed(p.binds, o.node) {
			s.scene.DropPresentationLocal(o.node)
		}
	}
	p.vals = sizeVals(p.vals, len(p.binds))
}

func fits(k schema.Kind, r *propref.Ref) bool {
	isFloat := k == schema.KindFloat32 || k == schema.KindFloat64
	if r.Lane() >= 0 {
		return isFloat
	}
	switch r.Shape() {
	case propref.Discrete:
		return k == r.Kind()
	case propref.Float:
		return isFloat
	case propref.Int:
		return k == schema.KindInt32 || k == schema.KindInt64
	case propref.Vec2:
		return k == schema.KindVec2
	case propref.Vec3:
		return k == schema.KindVec3
	case propref.Vec4:
		return k == schema.KindVec4 || k == schema.KindColor || k == schema.KindRect
	case propref.Quat:
		return k == schema.KindQuat
	}
	return false
}

func (s *Service) bindTrack(h scene.NodeHandle, c *AnimationPlayer, p *player, i int32) {
	t := &p.clip.Tracks[i]
	if t.Root {
		if s.once(h, CodeRoot, i) {
			s.warn(h, CodeRoot, "track "+trackName(t)+": root motion plays only in an Animator; the track is off")
		}
		return
	}
	node := s.scene.Find(h, t.Target)
	if node.IsZero() || !s.scene.IsAlive(node) {
		if s.once(h, CodeLost, i) {
			s.warn(h, CodeLost, "track "+trackName(t)+": the target node is missing")
		}
		return
	}
	ref, err := s.props.ResolveAny(node, t.Property)
	if err != nil {
		if s.once(h, CodeTrack, i) {
			s.warn(h, CodeTrack, "track "+trackName(t)+": "+err.Error())
		}
		return
	}
	if !fits(t.Kind, &ref) {
		if s.once(h, CodeTrack, i) {
			s.warn(h, CodeTrack, "track "+trackName(t)+" has kind "+t.Kind.String()+", which does not match the property")
		}
		return
	}
	if c.TimeDomain == DomainUnscaled && t.Authority == anim.AuthSimulation {
		if s.once(h, CodeUnscaled, i) {
			s.warn(h, CodeUnscaled, "track "+trackName(t)+" has authority=simulation in an unscaled player; it is off")
		}
		return
	}
	k := propclaim.KeyOf(node, &ref)
	for j := range s.owned[k] {
		if s.owned[k][j].Overlaps(&ref) {
			if s.once(h, CodeConflict, i) {
				s.warn(h, CodeConflict, "track "+trackName(t)+": another animation track drives this property; this one is off")
			}
			return
		}
	}
	s.owned[k] = append(s.owned[k], ref)
	p.binds = append(p.binds, binding{
		ref: ref, key: k, label: trackName(t), node: node, kind: t.Kind, track: i, auth: t.Authority, end: t.OnEnd,
		pose: ref.Owner() == propref.OwnTransform && !ref.Three(),
	})
}
