package camera

import (
	"math"
	"slices"
	"strconv"

	"aego/contracts/signals"
	"aego/core/curve"
	"aego/core/errs"
	"aego/core/mathx"
	"aego/engine"
	"aego/render/view"
	"aego/scene"
)

const (
	kindBase uint8 = iota
	kindZone
	kindForced
)

type cand struct {
	vc   scene.NodeHandle
	zone scene.NodeHandle
	prio int32
	kind uint8
	seq  uint64
}

func (a cand) beats(b cand) bool {
	if a.prio != b.prio {
		return a.prio > b.prio
	}
	if a.kind != b.kind {
		return a.kind > b.kind
	}
	if a.kind == kindZone {
		return a.seq > b.seq
	}
	return a.vc.Index < b.vc.Index
}

type blend struct {
	key  uint64
	from Pose
	t    float32
	dur  float32
	on   bool
	// stale: from was not drawn last tick (a rewind replays without drawing); the next Adjust takes it from prev.
	stale    bool
	prev     scene.NodeHandle
	prevZone scene.NodeHandle
}

type output struct {
	name      string
	seed      uint32
	vcs       []scene.NodeHandle
	active    scene.NodeHandle
	zone      scene.NodeHandle
	forced    scene.NodeHandle
	forcedDur float32
	forcedCut bool
	blend     blend
	trauma    float32
	noiseT    float32
	game      bool
	logical   Pose
	render    Pose
	view      view.View
	hasView   bool
	frame     uint64
}

type follow struct {
	node   scene.NodeHandle
	target scene.NodeHandle
	pos    mathx.Vec2
	look   mathx.Vec2
	last   mathx.Vec2
	vel    mathx.Vec2
	ready  bool
	snap   bool
}

type zone struct {
	node   scene.NodeHandle
	vc     scene.NodeHandle
	inside bool
	seen   bool
	seq    uint64
}

type Service struct {
	scene      *scene.Scene
	world      *engine.World
	hub        signals.Hub
	acc        *access
	outputs    []*output
	follows    []follow
	zones      []zone
	seq        uint64
	nextKey    uint64
	ticks      uint64
	dtGame     float32
	dtReal     float32
	shakeScale float32
	reduced    bool
}

var (
	errNoVirtual = errs.New(errs.EngineModule, "camera: the node has no Virtual Camera 2D")
	errDisabled  = errs.New(errs.EngineModule, "camera: the virtual camera is disabled")
)

func NewService(s *scene.Scene, w *engine.World) *Service {
	svc := &Service{scene: s, world: w, shakeScale: 1}
	if w != nil {
		svc.hub, _ = engine.ServiceOf[signals.Hub](w, signals.ServiceID)
	}
	return svc
}

func (s *Service) SetAccessibility(shake float32, reduced bool) {
	s.acc = nil
	s.shakeScale, s.reduced = shake, reduced
}

func (s *Service) find(name string) *output {
	for _, o := range s.outputs {
		if o.name == name {
			return o
		}
	}
	return nil
}

func (s *Service) ViewCenter() (mathx.Vec2, bool) {
	o := s.find("")
	if o == nil || !o.hasView {
		return mathx.Vec2{}, false
	}
	return o.render.Position, true
}

func (s *Service) out(name string) *output {
	if o := s.find(name); o != nil {
		return o
	}
	o := &output{name: name, seed: seedOf(name)}
	s.outputs = append(s.outputs, o)
	return o
}

func (s *Service) usable(h scene.NodeHandle, name string) bool {
	if h.IsZero() || !s.scene.IsAlive(h) || !s.scene.EffectiveEnabled(h) {
		return false
	}
	vc, ok := scene.Get[VirtualCamera2D](s.scene, h)
	return ok && vc.Output == name
}

func (s *Service) Update(game, real float32) {
	if game != game || game < 0 {
		game = 0
	}
	if real != real || real < 0 {
		real = 0
	}
	s.ticks++
	s.dtGame, s.dtReal = game, real
	if s.acc != nil {
		s.shakeScale, s.reduced = s.acc.read()
	}
	for _, o := range s.outputs {
		o.vcs = o.vcs[:0]
	}
	scene.Query1[VirtualCamera2D](s.scene).Each(func(h scene.NodeHandle, vc *VirtualCamera2D) {
		if s.scene.EffectiveEnabled(h) {
			o := s.out(vc.Output)
			o.vcs = append(o.vcs, h)
		}
	})
	s.updateZones()
	s.pruneFollows()
	for i := 0; i < len(s.outputs); i++ {
		o := s.outputs[i]
		if !o.forced.IsZero() && !s.usable(o.forced, o.name) {
			o.forced = scene.NodeHandle{}
		}
		s.reselect(o)
		s.advance(o)
	}
}

func (s *Service) choose(o *output) cand {
	var best cand
	found := false
	try := func(c cand) {
		if !found || c.beats(best) {
			best, found = c, true
		}
	}
	for _, h := range o.vcs {
		if vc, ok := scene.Get[VirtualCamera2D](s.scene, h); ok {
			try(cand{vc: h, prio: vc.Priority})
		}
	}
	for i := range s.zones {
		z := &s.zones[i]
		if !z.inside || !slices.Contains(o.vcs, z.vc) {
			continue
		}
		if zc, ok := scene.Get[CameraZone2D](s.scene, z.node); ok {
			try(cand{vc: z.vc, zone: z.node, prio: zc.Priority, kind: kindZone, seq: z.seq})
		}
	}
	if !o.forced.IsZero() {
		try(cand{vc: o.forced, prio: math.MaxInt32, kind: kindForced})
	}
	return best
}

func (s *Service) reselect(o *output) {
	c := s.choose(o)
	if c.vc != o.active || c.zone != o.zone {
		s.switchTo(o, c)
	}
	o.forcedDur, o.forcedCut = 0, false
}

func (s *Service) switchTo(o *output, c cand) {
	prev, prevZone := o.active, o.zone
	s.endBlend(o, false)
	o.active, o.zone = c.vc, c.zone
	if prev.IsZero() || c.vc.IsZero() || o.forcedCut {
		return
	}
	dur := o.forcedDur
	if dur <= 0 {
		if vc, ok := scene.Get[VirtualCamera2D](s.scene, c.vc); ok {
			dur = vc.BlendTime
		}
	}
	if dur <= 0 {
		return
	}
	s.nextKey++
	fresh := o.hasView && o.frame+1 >= s.ticks
	o.blend = blend{key: s.nextKey, from: o.logical, dur: dur, on: true, stale: !fresh, prev: prev, prevZone: prevZone}
}

func (s *Service) advance(o *output) {
	vc, ok := scene.Get[VirtualCamera2D](s.scene, o.active)
	o.game = ok && vc.TimeDomain == Game
	dt := s.dtReal
	if o.game {
		dt = s.dtGame
	}
	if o.blend.on {
		o.blend.t += dt / o.blend.dur
		if o.blend.t >= 1 {
			s.endBlend(o, true)
		}
	}
	sh := s.shakeOf(o.active)
	o.trauma = max(0, o.trauma-sh.Decay*dt)
	o.noiseT += sh.Frequency * dt
	if o.noiseT > 4096 {
		o.noiseT -= 4096
	}
}

func (s *Service) shakeOf(h scene.NodeHandle) CameraShake2D {
	if sh, ok := scene.Get[CameraShake2D](s.scene, h); ok {
		return *sh
	}
	return DefaultShake()
}

func (s *Service) blendK(o *output) float32 {
	u := mathx.Clamp(o.blend.t, 0, 1)
	vc, ok := scene.Get[VirtualCamera2D](s.scene, o.active)
	if !ok {
		return u
	}
	cv := &vc.BlendCurve
	if cv.Ease == curve.None && len(cv.Keys) == 0 {
		return u
	}
	t0, t1 := cv.Span()
	return cv.EvalClamped(t0 + (t1-t0)*u)
}

func (s *Service) Adjust(node scene.NodeHandle, v view.View) view.View {
	name := ""
	if b, ok := scene.Get[CameraBrain2D](s.scene, node); ok {
		name = b.Output
	}
	o := s.find(name)
	if o == nil {
		return v
	}
	if o.hasView && o.frame == s.ticks {
		v.Camera = o.render.camera()
		return v
	}
	o.frame = s.ticks
	base := v
	if o.active.IsZero() || !s.scene.IsAlive(o.active) {
		o.logical = poseOf(base.Camera)
		o.render = o.logical
	} else {
		s.stepFollows(o, base)
		p := s.vcPose(o.active, o.zone, base)
		if o.blend.on && o.blend.stale {
			o.blend.from, o.blend.stale = p, false
			if s.scene.IsAlive(o.blend.prev) {
				o.blend.from = s.vcPose(o.blend.prev, o.blend.prevZone, base)
			}
		}
		if o.blend.on {
			p = lerpPose(o.blend.from, p, s.blendK(o))
		}
		o.logical = p
		o.render = s.snapped(o, base, s.shaken(o, p))
	}
	v.Camera = o.render.camera()
	o.view = v
	o.hasView = true
	return v
}

func (s *Service) vcPose(h, zh scene.NodeHandle, base view.View) Pose {
	m := s.scene.RenderWorld(h)
	p := Pose{Position: mathx.Vec2{X: m[6], Y: m[7]}, Rotation: mathx.Atan2(m[1], m[0]), Zoom: base.Camera.Zoom}
	vc, ok := scene.Get[VirtualCamera2D](s.scene, h)
	if !ok {
		return p
	}
	if vc.Zoom > 0 {
		p.Zoom = vc.Zoom
	}
	if f := s.followOf(h); f != nil && f.ready {
		p.Position = f.pos
	}
	use, b := vc.UseBounds, vc.Bounds
	if !zh.IsZero() {
		if z, ok := scene.Get[CameraZone2D](s.scene, zh); ok {
			if z.Zoom > 0 {
				p.Zoom = z.Zoom
			}
			if z.UseBounds {
				use, b = true, z.Bounds
			}
		}
	}
	if use && !empty(b) {
		p.Position = clampView(base, p, b)
	}
	return p
}

func (s *Service) shaken(o *output, p Pose) Pose {
	scale := min(s.shakeScale, 1)
	if o.trauma <= 0 || s.reduced || scale <= 0 {
		return p
	}
	sh := s.shakeOf(o.active)
	amt := float32(math.Pow(float64(o.trauma), float64(max(sh.Exponent, 1)))) * scale
	t := o.noiseT
	p.Position.X += sh.MaxOffset.X * noise1(o.seed^1, t) * amt
	p.Position.Y += sh.MaxOffset.Y * noise1(o.seed^2, t) * amt
	p.Rotation += sh.MaxAngle * noise1(o.seed^3, t) * amt
	return p
}

func (s *Service) snapped(o *output, base view.View, p Pose) Pose {
	vc, ok := scene.Get[VirtualCamera2D](s.scene, o.active)
	if !ok || !vc.SnapToPixels || base.Scale == view.ScalePixelPerfect {
		return p
	}
	base.Camera.Zoom = p.Zoom
	ez := base.EffectiveZoom()
	if ez <= 0 {
		return p
	}
	p.Position = mathx.Vec2{
		X: float32(math.Round(float64(p.Position.X*ez))) / ez,
		Y: float32(math.Round(float64(p.Position.Y*ez))) / ez,
	}
	return p
}

func (s *Service) Active(name string) scene.NodeHandle {
	if o := s.find(name); o != nil {
		return o.active
	}
	return scene.NodeHandle{}
}

func (s *Service) IsActive(h scene.NodeHandle) bool {
	vc, ok := scene.Get[VirtualCamera2D](s.scene, h)
	if !ok {
		return false
	}
	o := s.find(vc.Output)
	return o != nil && o.active == h
}

func (s *Service) BlendTo(h scene.NodeHandle, seconds float32, cut bool) (uint64, error) {
	vc, ok := scene.Get[VirtualCamera2D](s.scene, h)
	if !ok || !s.scene.IsAlive(h) {
		return 0, errNoVirtual
	}
	if !s.scene.EffectiveEnabled(h) {
		return 0, errDisabled
	}
	o := s.out(vc.Output)
	if !slices.Contains(o.vcs, h) {
		o.vcs = append(o.vcs, h)
	}
	o.forced, o.forcedDur, o.forcedCut = h, seconds, cut
	if cut && o.active == h {
		s.endBlend(o, true)
	}
	s.reselect(o)
	if o.blend.on && o.active == h {
		return o.blend.key, nil
	}
	return 0, nil
}

func (s *Service) Release(name string, seconds float32) uint64 {
	o := s.find(name)
	if o == nil {
		return 0
	}
	o.forced, o.forcedDur = scene.NodeHandle{}, seconds
	s.reselect(o)
	if o.blend.on {
		return o.blend.key
	}
	return 0
}

func (s *Service) Shake(name string, trauma float32) {
	if trauma != trauma {
		return
	}
	o := s.out(name)
	o.trauma = mathx.Clamp(o.trauma+trauma, 0, 1)
}

func (s *Service) ShakeNode(h scene.NodeHandle, trauma float32) {
	if vc, ok := scene.Get[VirtualCamera2D](s.scene, h); ok {
		s.Shake(vc.Output, trauma)
	}
}

func (s *Service) Trauma(name string) float32 {
	if o := s.find(name); o != nil {
		return o.trauma
	}
	return 0
}

func (s *Service) Logical(name string) Pose {
	if o := s.find(name); o != nil {
		return o.logical
	}
	return Pose{}
}

func (s *Service) Render(name string) Pose {
	if o := s.find(name); o != nil {
		return o.render
	}
	return Pose{}
}

func (s *Service) View(name string) (view.View, bool) {
	if o := s.find(name); o != nil && o.hasView {
		return o.view, true
	}
	return view.View{}, false
}

type Status struct {
	Active   bool
	Blending bool
	Output   string
	Priority int32
	Blend    float32
	Trauma   float32
	Pose     Pose
}

func (st Status) Text() string {
	out := "inactive"
	if st.Active {
		out = "active"
	}
	out += " · priority " + strconv.Itoa(int(st.Priority))
	if st.Output != "" {
		out += " · output " + st.Output
	}
	if st.Blending {
		out += " · blend " + strconv.Itoa(int(st.Blend*100)) + "%"
	}
	if st.Trauma > 0 {
		out += " · trauma " + strconv.FormatFloat(float64(st.Trauma), 'f', 2, 32)
	}
	return out
}

func (s *Service) Status(h scene.NodeHandle) (Status, bool) {
	vc, ok := scene.Get[VirtualCamera2D](s.scene, h)
	if !ok {
		return Status{}, false
	}
	st := Status{Output: vc.Output, Priority: vc.Priority}
	o := s.find(vc.Output)
	if o != nil {
		st.Trauma = o.trauma
		if o.active == h {
			st.Active, st.Blending, st.Blend, st.Pose = true, o.blend.on, o.blend.t, o.logical
			if z, ok := scene.Get[CameraZone2D](s.scene, o.zone); ok && !o.zone.IsZero() {
				st.Priority = z.Priority
			}
		}
	}
	if !st.Active {
		m := s.scene.RenderWorld(h)
		st.Pose = Pose{Position: mathx.Vec2{X: m[6], Y: m[7]}, Rotation: mathx.Atan2(m[1], m[0]), Zoom: vc.Zoom}
		if f := s.followOf(h); f != nil && f.ready {
			st.Pose.Position = f.pos
		}
		if st.Pose.Zoom <= 0 && o != nil && o.hasView {
			st.Pose.Zoom = o.view.Camera.Zoom
		}
	}
	return st, true
}

type OutputInfo struct {
	Name    string
	Active  scene.NodeHandle
	Zone    scene.NodeHandle
	Logical Pose
	View    view.View
	HasView bool
}

func (s *Service) Outputs(fn func(OutputInfo)) {
	for _, o := range s.outputs {
		fn(OutputInfo{Name: o.name, Active: o.active, Zone: o.zone, Logical: o.logical, View: o.view, HasView: o.hasView})
	}
}
