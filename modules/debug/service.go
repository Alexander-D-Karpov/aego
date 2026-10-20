package debug

import (
	"math"
	"strings"

	"aego/contracts/debugdraw"
	"aego/core/mathx"
	"aego/engine"
	"aego/render"
	rdebug "aego/render/debug"
	"aego/render/text"
	"aego/scene"
	"aego/script/vm"
)

type kind uint8

const (
	kLine kind = iota
	kArrow
	kCircle
	kRect
	kPoly
	kCross
	kPoint
	kText
)

const (
	defaultChannel = "default"
	scriptChannel  = "script"
	maxChannels    = 1024
)

type prim struct {
	kind   kind
	closed bool
	gone   bool
	space  debugdraw.Space
	layer  debugdraw.Layer
	fade   debugdraw.Fade
	life   debugdraw.LifeKind
	dom    debugdraw.Domain
	ch     uint16
	pts    int32
	n      int32
	id     debugdraw.Handle
	a, b   mathx.Vec2
	r      float32
	thick  float32
	left   float32
	total  float32
	color  mathx.Color
	owner  scene.NodeHandle
	text   string
	src    string
}

type channel struct {
	name    string
	count   int
	dropped uint64
	state   int8
}

type pen struct {
	ch   string
	life debugdraw.Lifetime
}

type Service struct {
	cfg     Settings
	prims   []prim
	pts     []mathx.Vec2
	spare   []mathx.Vec2
	chans   []channel
	byName  map[string]uint16
	hidden  []string
	next    debugdraw.Handle
	dropped uint64
	pens    map[vm.InstanceID]pen
	def     debugdraw.Style
	tmp     debugdraw.Style
}

var _ debugdraw.Draw = (*Service)(nil)

func NewService(cfg Settings) *Service {
	cfg = cfg.normalized()
	n := int(cfg.MaxPrimitives)
	s := &Service{
		cfg:    cfg,
		prims:  make([]prim, 0, n),
		pts:    make([]mathx.Vec2, 0, 2*n),
		spare:  make([]mathx.Vec2, 0, 2*n),
		byName: map[string]uint16{},
		def:    debugdraw.Style{Color: mathx.Color{R: 1, G: 1, B: 1, A: 1}, Channel: defaultChannel},
	}
	s.channel(defaultChannel)
	return s
}

func Of(w *engine.World) *Service {
	s, _ := engine.ServiceOf[*Service](w, debugdraw.ServiceID)
	return s
}

func (s *Service) Enabled() bool {
	return s.cfg.Enabled
}

func (s *Service) channel(name string) uint16 {
	if name == "" {
		name = defaultChannel
	}
	if i, ok := s.byName[name]; ok {
		return i
	}
	if len(s.chans) >= maxChannels {
		return 0
	}
	i := uint16(len(s.chans))
	s.chans = append(s.chans, channel{name: name})
	s.byName[name] = i
	return i
}

func (s *Service) add(k kind, a, b mathx.Vec2, r float32, st *debugdraw.Style) *prim {
	if !s.cfg.Enabled {
		return nil
	}
	if st == nil {
		st = &s.def
	}
	ci := s.channel(st.Channel)
	c := &s.chans[ci]
	if len(s.prims) == cap(s.prims) || c.count >= int(s.cfg.MaxPerChannel) {
		c.dropped++
		s.dropped++
		return nil
	}
	col := st.Color
	if col == (mathx.Color{}) {
		col = s.def.Color
	}
	s.next++
	s.prims = append(s.prims, prim{
		kind: k, space: st.Space, layer: st.Layer, fade: st.Fade, life: st.Life.Kind, dom: st.Life.Domain,
		ch: ci, id: s.next, a: a, b: b, r: r, thick: st.Thickness, left: st.Life.N, total: st.Life.N,
		color: col, owner: st.Owner, src: st.Source,
	})
	c.count++
	return &s.prims[len(s.prims)-1]
}

func id(p *prim) debugdraw.Handle {
	if p == nil {
		return 0
	}
	return p.id
}

func (s *Service) Line(a, b mathx.Vec2, st *debugdraw.Style) debugdraw.Handle {
	return id(s.add(kLine, a, b, 0, st))
}

func (s *Service) Arrow(from, to mathx.Vec2, st *debugdraw.Style) debugdraw.Handle {
	return id(s.add(kArrow, from, to, 0, st))
}

func (s *Service) Ray(origin, dir mathx.Vec2, st *debugdraw.Style) debugdraw.Handle {
	return id(s.add(kArrow, origin, origin.Add(dir), 0, st))
}

func (s *Service) Circle(center mathx.Vec2, r float32, st *debugdraw.Style) debugdraw.Handle {
	return id(s.add(kCircle, center, center, r, st))
}

func (s *Service) Rect(r mathx.Rect, st *debugdraw.Style) debugdraw.Handle {
	return id(s.add(kRect, mathx.Vec2{X: r.X, Y: r.Y}, mathx.Vec2{X: r.X + r.W, Y: r.Y + r.H}, 0, st))
}

func (s *Service) AABB(lo, hi mathx.Vec2, st *debugdraw.Style) debugdraw.Handle {
	return id(s.add(kRect, lo, hi, 0, st))
}

func (s *Service) Cross(p mathx.Vec2, size float32, st *debugdraw.Style) debugdraw.Handle {
	return id(s.add(kCross, p, p, size, st))
}

func (s *Service) Point(p mathx.Vec2, st *debugdraw.Style) debugdraw.Handle {
	return id(s.add(kPoint, p, p, 0, st))
}

func (s *Service) Text(p mathx.Vec2, t string, st *debugdraw.Style) debugdraw.Handle {
	pr := s.add(kText, p, p, 0, st)
	if pr == nil {
		return 0
	}
	pr.text = t
	return pr.id
}

func (s *Service) Polygon(pts []mathx.Vec2, closed bool, st *debugdraw.Style) debugdraw.Handle {
	if !s.cfg.Enabled || len(pts) < 2 {
		return 0
	}
	if len(s.pts)+len(pts) > cap(s.pts) {
		s.dropped++
		return 0
	}
	p := s.add(kPoly, mathx.Vec2{}, mathx.Vec2{}, 0, st)
	if p == nil {
		return 0
	}
	p.pts, p.n, p.closed = int32(len(s.pts)), int32(len(pts)), closed
	s.pts = append(s.pts, pts...)
	return p.id
}

func (s *Service) kill(p *prim) {
	if !p.gone {
		p.gone = true
		s.chans[p.ch].count--
	}
}

func (s *Service) Clear(h debugdraw.Handle) {
	for i := range s.prims {
		if s.prims[i].id == h {
			s.kill(&s.prims[i])
			return
		}
	}
}

func (s *Service) ClearChannel(prefix string) {
	for i := range s.prims {
		if strings.HasPrefix(s.chans[s.prims[i].ch].name, prefix) {
			s.kill(&s.prims[i])
		}
	}
}

func (s *Service) visible(ci uint16) bool {
	switch s.chans[ci].state {
	case 1:
		return true
	case -1:
		return false
	}
	name := s.chans[ci].name
	for _, p := range s.hidden {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}

func (s *Service) Wants(name string) bool {
	return s.cfg.Enabled && s.visible(s.channel(name))
}

func (s *Service) Hide(name string, hidden bool) {
	c := &s.chans[s.channel(name)]
	c.state = 1
	if hidden {
		c.state = -1
	}
}

func (s *Service) HideByDefault(prefix string) {
	for _, p := range s.hidden {
		if p == prefix {
			return
		}
	}
	s.hidden = append(s.hidden, prefix)
}

func (p *prim) alive(paused bool, game, real float64) bool {
	if p.gone {
		return false
	}
	switch p.life {
	case debugdraw.OneFrame:
		return paused
	case debugdraw.Frames:
		if paused {
			return true
		}
		p.left--
		return p.left > 0
	case debugdraw.Seconds:
		d := game
		if p.dom == debugdraw.Unscaled {
			d = real
		}
		p.left -= float32(d)
		return p.left > 0
	}
	return true
}

func (s *Service) Advance(paused bool, game, real float64) {
	w := 0
	s.spare = s.spare[:0]
	for i := range s.prims {
		p := s.prims[i]
		gone := p.gone
		if !p.alive(paused, game, real) {
			if !gone {
				s.chans[p.ch].count--
			}
			continue
		}
		if p.kind == kPoly {
			at := len(s.spare)
			s.spare = append(s.spare, s.pts[p.pts:p.pts+p.n]...)
			p.pts = int32(at)
		}
		s.prims[w] = p
		w++
	}
	clear(s.prims[w:])
	s.prims = s.prims[:w]
	s.pts, s.spare = s.spare, s.pts[:0]
}

func (s *Service) Live() int {
	n := 0
	for i := range s.prims {
		if !s.prims[i].gone {
			n++
		}
	}
	return n
}

func (s *Service) Dropped() uint64 {
	return s.dropped
}

func (s *Service) EachChannel(fn func(name string, count int, dropped uint64)) {
	for _, c := range s.chans {
		fn(c.name, c.count, c.dropped)
	}
}

func (s *Service) SetPen(owner vm.InstanceID, ch string, ttl float32, unscaled bool) {
	if s.pens == nil {
		s.pens = map[vm.InstanceID]pen{}
	}
	var life debugdraw.Lifetime
	switch {
	case ttl < 0:
		life = debugdraw.Forever()
	case ttl > 0 && unscaled:
		life = debugdraw.ForUnscaled(ttl)
	case ttl > 0:
		life = debugdraw.ForSeconds(ttl)
	}
	s.pens[owner] = pen{ch: ch, life: life}
}

func (s *Service) scriptStyle(owner vm.InstanceID, node scene.NodeHandle, col mathx.Color) *debugdraw.Style {
	p := s.pens[owner]
	if p.ch == "" {
		p.ch = scriptChannel
	}
	s.tmp = debugdraw.Style{Color: col, Life: p.life, Channel: p.ch, Owner: node, Source: scriptChannel}
	return &s.tmp
}

func (p *prim) factor() float32 {
	if p.total <= 0 || (p.life != debugdraw.Seconds && p.life != debugdraw.Frames) {
		return 1
	}
	return mathx.Clamp(p.left/p.total, 0, 1)
}

func (p *prim) tint() (mathx.Color, float32) {
	f := p.factor()
	c, k := p.color, float32(1)
	if p.fade != debugdraw.FadeNone {
		c.A *= f
	}
	if p.fade == debugdraw.FadeAlphaScale {
		k = f
	}
	return c, k
}

func (s *Service) Lines(d *rdebug.Draw, px, scale float32, screen bool) {
	if scale <= 0 {
		scale = 1
	}
	for i := range s.prims {
		p := &s.prims[i]
		if p.gone || p.kind == kText || (p.space == debugdraw.Screen && !screen) || !s.visible(p.ch) {
			continue
		}
		c, k := p.tint()
		phys, unit := px, px*scale
		switch {
		case p.space == debugdraw.Screen:
			d.Space(rdebug.Screen)
			phys, unit = 1/scale, 1
		case p.layer == debugdraw.Overlay:
			d.Space(rdebug.Overlay)
		default:
			d.Space(rdebug.World)
		}
		n, step := 1, phys
		if p.thick > 1 && p.kind <= kRect {
			w, limit := p.thick*scale, 8
			if p.kind == kCircle {
				w, step, limit = w*math.Sqrt2, phys/math.Sqrt2, 12
			}
			n = min(int(w+0.5), limit)
		}
		for t := range n {
			s.shape(d, p, c, k, unit, (float32(t)-float32(n-1)/2)*step)
		}
	}
	d.Space(rdebug.World)
}

func (s *Service) shape(d *rdebug.Draw, p *prim, c mathx.Color, k, unit, off float32) {
	switch p.kind {
	case kLine, kArrow:
		v := p.b.Sub(p.a).Scale(k)
		l := v.Len()
		if l == 0 {
			return
		}
		u := v.Scale(1 / l)
		o := mathx.Vec2{Y: off}
		if mathx.Abs(u.X) < mathx.Abs(u.Y) {
			o = mathx.Vec2{X: off}
		}
		a, b := p.a.Add(o), p.a.Add(v).Add(o)
		d.Line(a, b, c)
		if p.kind == kArrow {
			h := min(10*unit, l*0.4)
			d.Line(b, b.Sub(u.Rotated(0.45).Scale(h)), c)
			d.Line(b, b.Sub(u.Rotated(-0.45).Scale(h)), c)
		}
	case kCircle:
		d.Circle(p.a, max(p.r*k+off, 0), 32, c)
	case kRect:
		e := mathx.Vec2{X: off, Y: off}
		d.AABB(mathx.AABB2{Min: p.a.Sub(e), Max: p.b.Add(e)}, c)
	case kPoly:
		pts := s.pts[p.pts : p.pts+p.n]
		for i := 0; i+1 < len(pts); i++ {
			d.Line(pts[i], pts[i+1], c)
		}
		if p.closed && len(pts) > 2 {
			d.Line(pts[len(pts)-1], pts[0], c)
		}
	case kCross:
		d.Cross(p.a, p.r*k, c)
	case kPoint:
		d.Cross(p.a, 6*unit*k, c)
	}
}

func (s *Service) Texts(f *render.ViewFrame, font *text.Font, px float32) {
	if font == nil {
		return
	}
	v := f.View()
	for i := range s.prims {
		p := &s.prims[i]
		if p.gone || p.kind != kText || p.text == "" || !s.visible(p.ch) {
			continue
		}
		c, k := p.tint()
		pos := p.a
		if p.space == debugdraw.Screen {
			pos = v.ScreenToWorld(p.a.Scale(v.PixelScale))
		}
		f.TextFont(font, p.text, pos, px*v.PixelScale*k, c, math.MaxInt16, 0)
	}
}
