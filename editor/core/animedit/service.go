package animedit

import (
	"math"

	"aego/anim"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/propref"
	"aego/schema"
)

type Mode uint8

const (
	Normal Mode = iota
	Preview
	Record
)

type Mark struct {
	Name string
	T    float32
}

type binding struct {
	node  scene.NodeHandle
	ref   propref.Ref
	track int
	pose  bool
	orig  anim.Value
}

type Service struct {
	AutoKey bool
	doc     *document.SceneDocument
	files   Files
	notify  func(string)
	scene   *scene.Scene
	props   *propref.Resolver
	mode    Mode
	rel     string
	adoc    *mutate.AnimDocument
	clip    *anim.Clip
	target  ids.NodeID
	time    float32
	speed   float32
	playing bool
	applied bool
	binds   []binding
	skipped []string
	posed   map[scene.NodeHandle]scene.Transform2D
	next    map[scene.NodeHandle]scene.Transform2D
	samp    anim.Sampler
	marks   []Mark
	cross   []anim.Crossing
	board   *[]CopiedKey
	flash   []flash
	hold    bool
}

func New(doc *document.SceneDocument, files Files, notify func(string)) *Service {
	return &Service{
		AutoKey: true, doc: doc, files: files, notify: notify, speed: 1,
		posed: map[scene.NodeHandle]scene.Transform2D{}, next: map[scene.NodeHandle]scene.Transform2D{},
	}
}

func (s *Service) Mode() Mode         { return s.mode }
func (s *Service) Time() float32      { return s.time }
func (s *Service) Clip() *anim.Clip   { return s.clip }
func (s *Service) ClipPath() string   { return s.rel }
func (s *Service) Target() ids.NodeID { return s.target }
func (s *Service) Playing() bool      { return s.playing }
func (s *Service) Skipped() []string  { return s.skipped }
func (s *Service) SetSpeed(v float32) { s.speed = v }
func (s *Service) Pause()             { s.playing = false }

func (s *Service) say(msg string) {
	if s.notify != nil {
		s.notify(msg)
	}
}

func decodeClip(rel string, data []byte) (*mutate.AnimDocument, *anim.Clip, error) {
	d, err := mutate.LoadAnim(data, rel)
	if err != nil {
		return nil, nil, err
	}
	c, err := d.Clip()
	return d, c, err
}

func (s *Service) SetClip(rel string) error {
	data, err := s.files.Read(rel)
	if err != nil {
		return err
	}
	d, c, err := decodeClip(rel, data)
	if err != nil {
		return err
	}
	s.restore()
	s.rel, s.adoc, s.clip, s.time = rel, d, c, 0
	s.apply()
	return nil
}

func (s *Service) loaded(rel string, data []byte) {
	if rel != s.rel {
		return
	}
	d, c, err := decodeClip(rel, data)
	if err != nil {
		s.say(err.Error())
		return
	}
	s.restore()
	s.adoc, s.clip = d, c
	s.apply()
}

func (s *Service) Source() *mutate.AnimDocument {
	return s.adoc
}

func (s *Service) View() View {
	return BuildView(s.clip, s.adoc)
}

func (s *Service) SetTarget(id ids.NodeID) {
	if id == s.target {
		return
	}
	s.restore()
	s.target = id
	s.apply()
}

func (s *Service) SetMode(m Mode) {
	if m == s.mode {
		return
	}
	h := s.doc.History
	if m == Normal {
		s.restore()
		s.mode, s.playing = Normal, false
		h.SetGuard(nil)
		h.SetFilter(nil)
		return
	}
	s.mode = m
	h.SetGuard(s)
	if m == Record {
		h.SetFilter(s.record)
	} else {
		h.SetFilter(nil)
	}
	s.apply()
}

func (s *Service) Close() {
	s.SetMode(Normal)
}

func (s *Service) Before() {
	s.restore()
}

func (s *Service) After() {
	s.apply()
}

func (s *Service) SetTime(t float32) {
	if s.clip != nil {
		t = mathx.Clamp(t, 0, s.clip.Length)
	}
	s.time = t
	if s.applied {
		s.sample()
	}
}

func (s *Service) Play() {
	if s.mode == Normal {
		s.SetMode(Preview)
	}
	s.playing = s.clip != nil
}

func (s *Service) TakeMarks(dst []Mark) []Mark {
	dst = append(dst[:0], s.marks...)
	s.marks = s.marks[:0]
	return dst
}

func (s *Service) Tick(dt float32) {
	if s.mode == Normal {
		return
	}
	if s.applied && s.scene != s.doc.Scene() {
		s.applied = false
		clear(s.posed)
		s.apply()
	}
	n := 0
	for _, f := range s.flash {
		if f.life -= dt; f.life > 0 {
			s.flash[n] = f
			n++
		}
	}
	s.flash = s.flash[:n]
	if !s.playing || s.clip == nil || s.clip.Length <= 0 {
		return
	}
	c := s.clip
	t0 := s.time
	t1 := t0 + dt*s.speed
	s.cross = anim.Crossings(c, c.Loop, float64(t0), float64(t1), false, s.cross)
	for _, x := range s.cross {
		if e := &c.Events[x.Event]; e.Mode == anim.EventPreview {
			s.marks = append(s.marks, Mark{Name: e.Name, T: e.T})
			s.flash = append(s.flash, flash{t: e.T, life: flashLife})
		}
	}
	switch {
	case c.Loop == anim.LoopOnce && (t1 >= c.Length || t1 <= 0):
		t1 = mathx.Clamp(t1, 0, c.Length)
		s.playing = false
	case c.Loop == anim.LoopPingPong:
		t1 = wrap(t1, 2*c.Length)
	case c.Loop == anim.LoopRepeat:
		t1 = wrap(t1, c.Length)
	}
	s.time = t1
	s.sample()
}

func wrap(t, l float32) float32 {
	return t - l*float32(math.Floor(float64(t/l)))
}

func (s *Service) local() float32 {
	t, _ := s.clip.Local(s.clip.Loop, float64(s.time))
	return t
}

func (s *Service) apply() {
	if s.mode == Normal || s.clip == nil {
		return
	}
	if !s.applied {
		if !s.bind() {
			return
		}
		s.applied = true
	}
	s.sample()
}

func trackName(t *anim.Track) string {
	if t.Target == "" {
		return t.Property
	}
	return t.Target + ": " + t.Property
}

func (s *Service) bind() bool {
	sc := s.doc.Scene()
	s.scene = sc
	s.props = propref.New(sc, nil)
	s.binds, s.skipped = s.binds[:0], s.skipped[:0]
	root := sc.Handle(s.target)
	if root.IsZero() || !sc.IsAlive(root) {
		return false
	}
	s.samp.Reset(s.clip)
	for i := range s.clip.Tracks {
		t := &s.clip.Tracks[i]
		if t.Root {
			continue
		}
		node := root
		if t.Target != "" {
			node = sc.Find(root, t.Target)
		}
		if node.IsZero() || !sc.IsAlive(node) {
			s.skipped = append(s.skipped, trackName(t)+": the node is missing")
			continue
		}
		ref, err := s.props.ResolveAny(node, t.Property)
		if err != nil {
			s.skipped = append(s.skipped, trackName(t)+": "+err.Error())
			continue
		}
		if !fits(t.Kind, &ref) {
			s.skipped = append(s.skipped, trackName(t)+": the kind does not match the property")
			continue
		}
		b := binding{node: node, ref: ref, track: i, pose: ref.Owner() == propref.OwnTransform && !ref.Three()}
		if !b.pose {
			b.orig = s.read(&b)
		}
		s.binds = append(s.binds, b)
	}
	return true
}

func (s *Service) sample() {
	if !s.applied {
		return
	}
	local := s.local()
	clear(s.next)
	for i := range s.binds {
		b := &s.binds[i]
		v := s.samp.Track(s.clip, b.track, local)
		if !b.pose {
			s.write(b, v)
			continue
		}
		if s.hold {
			continue
		}
		tr, ok := s.next[b.node]
		if !ok {
			tr = s.scene.Local(b.node)
		}
		applyPose(&tr, &b.ref, v)
		s.next[b.node] = tr
	}
	for h, tr := range s.next {
		s.scene.SetPresentationLocal(h, tr)
		s.posed[h] = tr
	}
}

func (s *Service) restore() {
	if !s.applied {
		return
	}
	for i := range s.binds {
		if b := &s.binds[i]; !b.pose && s.scene.IsAlive(b.node) {
			s.write(b, b.orig)
		}
	}
	for h, tr := range s.posed {
		if cur, ok := s.scene.PresentationLocal(h); ok && cur == tr {
			s.scene.DropPresentationLocal(h)
		}
		delete(s.posed, h)
	}
	s.applied = false
}

// Driven reports a field the clip shows in Preview; in Record edits become keys, so nothing is driven.
func (s *Service) Driven(node ids.NodeID, t schema.TypeID, f schema.FieldID) bool {
	if !s.applied || s.mode != Preview {
		return false
	}
	h := s.scene.Handle(node)
	td, ok := s.scene.Registry().ByID(t)
	if !ok || h.IsZero() {
		return false
	}
	fd, ok := td.FieldByID(f)
	if !ok {
		return false
	}
	ref, err := s.props.ResolveAny(h, td.FullName+"."+fd.Name)
	if err != nil {
		return false
	}
	for i := range s.binds {
		if s.binds[i].node == h && s.binds[i].ref.Overlaps(&ref) {
			return true
		}
	}
	return false
}

func (s *Service) read(b *binding) anim.Value {
	switch b.ref.Shape() {
	case propref.Discrete:
		i, a, _ := s.props.ReadDiscrete(&b.ref, b.node)
		return anim.Value{I: i, A: a}
	case propref.Int:
		l, _ := s.props.Read(&b.ref, b.node)
		return anim.Int(int64(math.Round(float64(l[0]))))
	}
	l, _ := s.props.Read(&b.ref, b.node)
	return anim.Lanes(l)
}

func (s *Service) write(b *binding, v anim.Value) {
	switch b.ref.Shape() {
	case propref.Discrete:
		s.props.WriteDiscrete(&b.ref, b.node, v.I, v.A)
	case propref.Int:
		s.props.Write(&b.ref, b.node, [4]float32{float32(v.I)})
	default:
		s.props.Write(&b.ref, b.node, v.L)
	}
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

func applyPose(tr *scene.Transform2D, r *propref.Ref, v anim.Value) {
	switch r.Part() {
	case propref.PartPosition:
		setVec(&tr.Position, r.Lane(), v.L)
	case propref.PartRotation:
		tr.Rotation = v.L[0]
	default:
		setVec(&tr.Scale, r.Lane(), v.L)
	}
}

var errNoClip = errs.New(errs.EditorCommand, "open a clip and turn on Preview first")
