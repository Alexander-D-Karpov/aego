package input

import (
	"math"
	"slices"
	"strconv"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine/recorder"
	raw "aego/platform/input"
)

const (
	PressPoint     = 0.5
	DeviceNone     = -1
	DeviceKeyboard = 0
	devAll         = uint8(1)<<(raw.MaxPads+1) - 1
	evPress        = uint8(1)
	evRelease      = uint8(2)
)

type Clock struct {
	Real float64
	Dt   float64
	Game float64
	Next uint64
}

type Files interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
}

type state struct {
	value, fValue              mathx.Vec2
	injVal                     mathx.Vec2
	down, pressed, released    bool
	fDown, fPressed, fReleased bool
	consumed, fConsumed        bool
	active, release, rawPrev   bool
	latch, bufLive, inj        bool
	pend                       [2]uint8
	npend                      uint8
	blockedBy, via, why        int8
	visit                      uint64
	heldFor, pressAt, bufAt    float64
}

func (st *state) push(e uint8) {
	if st.npend < 2 {
		st.pend[st.npend] = e
		st.npend++
		return
	}
	st.pend[1] = e
}

type injection struct {
	tick   uint64
	player int
	action int
	value  mathx.Vec2
}

type Info struct {
	Name    string
	Kind    Kind
	Active  bool
	Down    bool
	Value   mathx.Vec2
	Context string
	Blocked string
	Why     string
	Glyph   string
}

type ContextInfo struct {
	Name     string
	Priority int
	Consume  bool
}

type Service struct {
	base      Config
	cfg       Config
	acts      map[string]int
	ctxs      map[string]int
	ctxActs   [][]int
	players   int
	states    []state
	stack     []int
	order     []int
	devs      [MaxPlayers]uint8
	device    [MaxPlayers]int8
	padsOn    [raw.MaxPads]bool
	changed   bool
	frame     uint64
	fixed     bool
	mask      mask
	in        *raw.State
	clock     Clock
	inj       []injection
	rec       *Recording
	play      *replay
	rebind    rebind
	keys      UserKeys
	look      look
	unsub     func()
	events    *recorder.Recorder
	detail    [MaxPlayers][2]string
	listeners []func(player, device int)
}

func NewService(c Config, keys UserKeys) *Service {
	s := &Service{base: c, keys: keys}
	for p := range s.device {
		s.device[p] = DeviceNone
	}
	if keys.Store != nil {
		s.unsub = keys.Store.Subscribe(0, s.settingChanged)
	}
	s.build()
	if s.players == 1 {
		s.devs[0] = devAll
	} else {
		s.devs[0] = 1
	}
	s.SetContexts(c.Stack...)
	return s
}

func (s *Service) SetRecorder(r *recorder.Recorder) {
	s.events = r
}

func (s *Service) build() {
	c := s.base
	c.Actions = make([]Action, len(s.base.Actions))
	for i, a := range s.base.Actions {
		a.Bindings = slices.Clone(a.Bindings)
		c.Actions[i] = a
	}
	s.cfg = c
	s.look = look{sens: 1}
	if st := s.keys.Store; st != nil {
		if s.keys.Deadzone != 0 {
			s.cfg.Deadzone = float32(st.Float(s.keys.Deadzone))
		}
		if s.keys.Sensitivity != 0 {
			s.look.sens = float32(st.Float(s.keys.Sensitivity))
		}
		s.look.invX, s.look.invY = st.Bool(s.keys.InvertX), st.Bool(s.keys.InvertY)
		for i := range s.cfg.Actions {
			s.userBindings(&s.cfg.Actions[i])
		}
	}
	s.acts = make(map[string]int, len(c.Actions))
	for i, a := range c.Actions {
		s.acts[a.Name] = i
	}
	s.ctxs = make(map[string]int, len(c.Contexts))
	s.ctxActs = make([][]int, len(c.Contexts))
	for i, x := range c.Contexts {
		s.ctxs[x.Name] = i
		for _, name := range x.Actions {
			if ai, ok := s.acts[name]; ok {
				s.ctxActs[i] = append(s.ctxActs[i], ai)
			}
		}
	}
	s.players = max(c.Players, 1)
	if n := s.players * len(c.Actions); len(s.states) != n {
		s.states = make([]state, n)
		for i := range s.states {
			s.states[i].pressAt = math.Inf(-1)
		}
	}
	for p := 0; p < s.players; p++ {
		prefix := ""
		if s.players > 1 {
			prefix = "p" + strconv.Itoa(p) + " "
		}
		s.detail[p] = [2]string{prefix + "pressed", prefix + "released"}
	}
}

func (s *Service) Config() *Config {
	return &s.cfg
}

func (s *Service) Players() int {
	return s.players
}

func (s *Service) Has(name string) bool {
	_, ok := s.acts[name]
	return ok
}

func (s *Service) leave(ci int) {
	if !s.cfg.Contexts[ci].Release {
		return
	}
	n := len(s.cfg.Actions)
	for _, ai := range s.ctxActs[ci] {
		for p := 0; p < s.players; p++ {
			s.states[p*n+ai].release = true
		}
	}
}

func (s *Service) sortStack() {
	s.order = s.order[:0]
	for i := len(s.stack) - 1; i >= 0; i-- {
		s.order = append(s.order, s.stack[i])
	}
	slices.SortStableFunc(s.order, func(a, b int) int {
		return s.cfg.Contexts[b].Priority - s.cfg.Contexts[a].Priority
	})
}

func (s *Service) PushContext(name string) bool {
	ci, ok := s.ctxs[name]
	if !ok {
		return false
	}
	s.stack = append(s.stack, ci)
	s.sortStack()
	return true
}

func (s *Service) PopContext() {
	n := len(s.stack)
	if n == 0 {
		return
	}
	s.leave(s.stack[n-1])
	s.stack = s.stack[:n-1]
	s.sortStack()
}

func (s *Service) SetContext(name string) bool {
	if _, ok := s.ctxs[name]; !ok {
		return false
	}
	s.SetContexts(name)
	return true
}

func (s *Service) SetContexts(names ...string) {
	for _, ci := range s.stack {
		s.leave(ci)
	}
	s.stack = s.stack[:0]
	for _, name := range names {
		if ci, ok := s.ctxs[name]; ok {
			s.stack = append(s.stack, ci)
		}
	}
	s.sortStack()
}

func (s *Service) Contexts() []string {
	out := make([]string, len(s.stack))
	for i, ci := range s.stack {
		out[i] = s.cfg.Contexts[ci].Name
	}
	return out
}

func (s *Service) Frame(in *raw.State, ck Clock) {
	s.in, s.clock = in, ck
	s.frame++
	s.fixed = false
	s.changed = false
	for i := range s.states {
		st := &s.states[i]
		st.pressed, st.released, st.consumed = false, false, false
	}
	if in != nil {
		s.devices(in)
	}
	s.applyInjections(ck.Next)
	switch {
	case s.play != nil:
		s.replayFrame()
	default:
		if in != nil {
			if s.rebind.on {
				s.pollRebind(in)
			}
			s.activity(in)
		}
		for p := 0; p < s.players; p++ {
			s.evalPlayer(p, in)
		}
	}
	s.post()
}

func (s *Service) evalPlayer(p int, in *raw.State) {
	s.mask = mask{}
	r := reader{in: in, kbm: s.devs[p]&1 != 0, pads: s.devs[p] >> 1}
	n := len(s.cfg.Actions)
	base := p * n
	for _, ci := range s.order {
		for _, ai := range s.ctxActs[ci] {
			st := &s.states[base+ai]
			if st.visit == s.frame {
				continue
			}
			st.visit = s.frame
			st.via = int8(ci + 1)
			s.evalAction(st, &s.cfg.Actions[ai], &r)
		}
		if s.cfg.Contexts[ci].Consume {
			for _, ai := range s.ctxActs[ci] {
				s.mask.take(&s.cfg.Actions[ai], r.pads, int8(ci+1))
			}
		}
	}
	for ai := 0; ai < n; ai++ {
		if st := &s.states[base+ai]; st.visit != s.frame && st.active {
			s.deactivate(st)
		}
	}
}

func (s *Service) deactivate(st *state) {
	if st.release && st.down {
		st.released = true
	}
	st.active, st.release, st.down, st.rawPrev, st.latch = false, false, false, false, false
	st.value = mathx.Vec2{}
	st.heldFor = 0
	st.via, st.blockedBy, st.why = 0, 0, 0
}

func (s *Service) deadzone(b *Binding, a *Action) float32 {
	switch {
	case b.Deadzone > 0:
		return b.Deadzone
	case a.Deadzone > 0:
		return a.Deadzone
	}
	return s.cfg.Deadzone
}

func deadzone1(x, dz float32) float32 {
	ax := mathx.Abs(x)
	if ax <= dz {
		return 0
	}
	v := min((ax-dz)/(1-dz), 1)
	if x < 0 {
		return -v
	}
	return v
}

func (s *Service) binding(b *Binding, a *Action, r *reader) (v mathx.Vec2, down, pressed, released bool) {
	if !b.Mod.IsZero() && !r.read(b.Mod).down {
		return
	}
	switch b.Shape {
	case Single:
		c := b.Parts[0]
		if c.Device == DevStick {
			v = r.stick(c.Code)
			break
		}
		x := r.read(c)
		v.X, down, pressed, released = x.v, x.down, x.pressed, x.released
	case Pair:
		n, p := r.read(b.Parts[0]), r.read(b.Parts[1])
		v.X = p.v - n.v
		down, pressed, released = n.down || p.down, n.pressed || p.pressed, n.released || p.released
	case Quad:
		u, d, l, rt := r.read(b.Parts[0]), r.read(b.Parts[1]), r.read(b.Parts[2]), r.read(b.Parts[3])
		v = mathx.Vec2{X: rt.v - l.v, Y: d.v - u.v}
		down = u.down || d.down || l.down || rt.down
		pressed = u.pressed || d.pressed || l.pressed || rt.pressed
		released = u.released || d.released || l.released || rt.released
	}
	k := b.Scale
	if k == 0 {
		k = 1
	}
	if b.Invert {
		k = -k
	}
	v = v.Scale(k)
	if b.analog() {
		dz := s.deadzone(b, a)
		if b.Parts[0].Device == DevStick {
			if l := v.Len(); l <= dz {
				v = mathx.Vec2{}
			} else {
				v = v.Scale(min((l-dz)/(1-dz), 1) / l)
			}
			down = v.Len() >= PressPoint
		} else {
			v.X = deadzone1(v.X, dz)
			down = mathx.Abs(v.X) >= PressPoint
		}
	}
	return
}

func (s *Service) evalAction(st *state, a *Action, r *reader) {
	st.release = false
	st.blockedBy, st.why = 0, 0
	var v mathx.Vec2
	rawDown, tap, fresh := false, false, false
	switch {
	case st.inj:
		v = st.injVal
		rawDown, fresh = v.Len() >= PressPoint, true
	case a.Kind == Pointer:
		if r.in != nil {
			v = r.in.MousePos()
		}
	case a.Kind == PointerDelta:
		if r.in != nil {
			k := a.Sensitivity
			if k == 0 {
				k = 1
			}
			v = r.in.MouseDelta().Scale(k * s.look.sens)
			if s.look.invX {
				v.X = -v.X
			}
			if s.look.invY {
				v.Y = -v.Y
			}
		}
	default:
		for i := range a.Bindings {
			b := &a.Bindings[i]
			if by := s.mask.blocked(b, r.pads); by != 0 {
				st.blockedBy, st.why = by, int8(i)
				continue
			}
			bv, d, p, rl := s.binding(b, a, r)
			if a.Kind == Button {
				v.X = max(v.X, mathx.Abs(bv.X))
			} else {
				v = v.Add(bv)
			}
			rawDown = rawDown || d
			tap = tap || (p && rl)
			fresh = fresh || p
		}
	}
	switch a.Kind {
	case Button:
		rawDown = rawDown || v.X >= PressPoint
	case Axis1D:
		v.X = mathx.Clamp(v.X, -1, 1)
		rawDown = mathx.Abs(v.X) >= PressPoint
	case Axis2D:
		if l := v.Len(); l > 1 {
			v = v.Scale(1 / l)
		}
		rawDown = v.Len() >= PressPoint
	}
	if a.Smoothing > 0 && a.Kind != Button && a.Kind != Pointer {
		t := float32(1 - math.Exp(-s.clock.Dt/float64(a.Smoothing)))
		v = st.value.Add(v.Sub(st.value).Scale(t))
	}
	tap = tap && !rawDown
	if !st.active {
		st.rawPrev = rawDown && !fresh
	}
	rp := !st.rawPrev && (rawDown || tap)
	rr := !rawDown && (st.rawPrev || tap)
	st.active = true
	st.rawPrev = rawDown
	down, pressed, released := s.interact(st, a, rawDown, rp, rr)
	st.down, st.value = down, v
	st.pressed = st.pressed || pressed
	st.released = st.released || released
}

func (s *Service) interact(st *state, a *Action, rawDown, rp, rr bool) (bool, bool, bool) {
	now := s.clock.Real
	t := float64(a.Interaction.Time)
	switch a.Interaction.Trigger {
	case Release:
		return false, rr, rr
	case Hold:
		if t <= 0 {
			t = 0.4
		}
		if rawDown {
			st.heldFor += s.clock.Dt
		} else {
			st.heldFor = 0
		}
		held := rawDown && st.heldFor >= t
		return held, held && !st.down, st.down && !held
	case Tap:
		if t <= 0 {
			t = 0.2
		}
		if rp {
			st.pressAt = now
		}
		ok := rr && now-st.pressAt <= t
		return false, ok, ok
	case DoubleTap:
		if t <= 0 {
			t = 0.3
		}
		ok := false
		if rp {
			if now-st.pressAt <= t {
				ok, st.pressAt = true, math.Inf(-1)
			} else {
				st.pressAt = now
			}
		}
		return false, ok, ok
	case Toggle:
		if rp {
			st.latch = !st.latch
			return st.latch, st.latch, !st.latch
		}
		return st.latch, false, false
	}
	return rawDown, rp, rr
}

func (s *Service) post() {
	n := len(s.cfg.Actions)
	for i := range s.states {
		st := &s.states[i]
		if st.pressed {
			st.push(evPress)
			st.bufAt, st.bufLive = s.clock.Game, true
			s.note(i, n, 0)
		}
		if st.released {
			st.push(evRelease)
			s.note(i, n, 1)
		}
	}
	if s.rec != nil {
		s.rec.capture(s)
	}
}

func (s *Service) note(i, n, k int) {
	if s.events.Wants(recorder.Input) {
		s.events.Add(recorder.Input, ids.NodeID{}, ids.NodeID{}, s.cfg.Actions[i%n].Name, s.detail[i/n][k])
	}
}

func (s *Service) Tick() {
	s.fixed = true
	for i := range s.states {
		st := &s.states[i]
		st.fPressed, st.fReleased, st.fConsumed = false, false, false
		if st.npend > 0 {
			e := st.pend[0]
			st.pend[0] = st.pend[1]
			st.npend--
			st.fPressed, st.fReleased = e == evPress, e == evRelease
		}
		st.fDown = st.fPressed || (st.down && !st.fReleased)
		st.fValue = st.value
	}
}

func (s *Service) EndTick() {
	s.fixed = false
}

func (s *Service) slot(p int, name string) (*state, *Action) {
	ai, ok := s.acts[name]
	if !ok || p < 0 || p >= s.players {
		return nil, nil
	}
	return &s.states[p*len(s.cfg.Actions)+ai], &s.cfg.Actions[ai]
}

func (s *Service) PressedP(p int, name string) bool {
	st, _ := s.slot(p, name)
	switch {
	case st == nil:
		return false
	case s.fixed:
		return st.fPressed && !st.fConsumed
	}
	return st.pressed && !st.consumed
}

func (s *Service) ReleasedP(p int, name string) bool {
	st, _ := s.slot(p, name)
	switch {
	case st == nil:
		return false
	case s.fixed:
		return st.fReleased
	}
	return st.released
}

func (s *Service) HeldP(p int, name string) bool {
	st, _ := s.slot(p, name)
	switch {
	case st == nil:
		return false
	case s.fixed:
		return st.fDown
	}
	return st.down
}

func (s *Service) Axis2P(p int, name string) mathx.Vec2 {
	st, _ := s.slot(p, name)
	switch {
	case st == nil:
		return mathx.Vec2{}
	case s.fixed:
		return st.fValue
	}
	return st.value
}

func (s *Service) AxisP(p int, name string) float32 {
	return s.Axis2P(p, name).X
}

func (s *Service) BufferedP(p int, name string, window float32) bool {
	st, a := s.slot(p, name)
	if st == nil || !st.bufLive {
		return false
	}
	if window <= 0 {
		window = a.Buffer
	}
	return s.clock.Game-st.bufAt <= float64(window)
}

func (s *Service) ConsumeP(p int, name string) {
	st, _ := s.slot(p, name)
	if st == nil {
		return
	}
	if s.fixed {
		st.fConsumed = true
	} else {
		st.consumed = true
	}
	st.bufLive = false
}

func (s *Service) Pressed(name string) bool {
	return s.PressedP(0, name)
}

func (s *Service) Released(name string) bool {
	return s.ReleasedP(0, name)
}

func (s *Service) Held(name string) bool {
	return s.HeldP(0, name)
}

func (s *Service) Axis(name string) float32 {
	return s.AxisP(0, name)
}

func (s *Service) Axis2(name string) mathx.Vec2 {
	return s.Axis2P(0, name)
}

func (s *Service) Buffered(name string, window float32) bool {
	return s.BufferedP(0, name, window)
}

func (s *Service) Consume(name string) {
	s.ConsumeP(0, name)
}

func (s *Service) Pointer() mathx.Vec2 {
	if s.in == nil {
		return mathx.Vec2{}
	}
	return s.in.MousePos()
}

func (s *Service) PointerDelta() mathx.Vec2 {
	if s.in == nil {
		return mathx.Vec2{}
	}
	return s.in.MouseDelta()
}

func (s *Service) GlyphP(p int, name string) string {
	_, a := s.slot(p, name)
	if a == nil || len(a.Bindings) == 0 {
		return ""
	}
	pad := s.device[p] > 0
	for i := range a.Bindings {
		if a.Bindings[i].Pad() == pad {
			return a.Bindings[i].glyph
		}
	}
	return a.Bindings[0].glyph
}

func (s *Service) Glyph(name string) string {
	return s.GlyphP(0, name)
}

func (s *Service) ActiveDevice(p int) int {
	if p < 0 || p >= s.players {
		return DeviceNone
	}
	return int(s.device[p])
}

func (s *Service) DeviceChanged() bool {
	return s.changed
}

func (s *Service) OnDeviceChanged(fn func(player, device int)) {
	s.listeners = append(s.listeners, fn)
}

func (s *Service) Each(p int, fn func(Info)) {
	if p < 0 || p >= s.players {
		return
	}
	n := len(s.cfg.Actions)
	for ai := range s.cfg.Actions {
		a := &s.cfg.Actions[ai]
		st := &s.states[p*n+ai]
		info := Info{Name: a.Name, Kind: a.Kind, Active: st.active, Down: st.down, Value: st.value, Glyph: s.GlyphP(p, a.Name)}
		if st.via > 0 {
			info.Context = s.cfg.Contexts[st.via-1].Name
		}
		if st.blockedBy > 0 {
			info.Blocked = s.cfg.Contexts[st.blockedBy-1].Name
			if int(st.why) < len(a.Bindings) {
				info.Why = a.Bindings[st.why].String()
			}
		}
		fn(info)
	}
}

func (s *Service) Stack() []ContextInfo {
	out := make([]ContextInfo, len(s.order))
	for i, ci := range s.order {
		x := &s.cfg.Contexts[ci]
		out[i] = ContextInfo{Name: x.Name, Priority: x.Priority, Consume: x.Consume}
	}
	return out
}

func (s *Service) Inject(tick uint64, name string, v float32) error {
	return s.InjectP(0, tick, name, mathx.Vec2{X: v})
}

func (s *Service) InjectVec(tick uint64, name string, v mathx.Vec2) error {
	return s.InjectP(0, tick, name, v)
}

func (s *Service) InjectP(p int, tick uint64, name string, v mathx.Vec2) error {
	ai, ok := s.acts[name]
	if !ok {
		return errs.New(errs.EngineModule, "input: unknown action "+name)
	}
	if p < 0 || p >= s.players {
		return errs.New(errs.EngineModule, "input: player "+strconv.Itoa(p)+" is out of range")
	}
	in := injection{tick: tick, player: p, action: ai, value: v}
	at := len(s.inj)
	for at > 0 && s.inj[at-1].tick > tick {
		at--
	}
	s.inj = slices.Insert(s.inj, at, in)
	return nil
}

func (s *Service) applyInjections(next uint64) {
	k := 0
	n := len(s.cfg.Actions)
	for k < len(s.inj) && s.inj[k].tick <= next {
		x := s.inj[k]
		st := &s.states[x.player*n+x.action]
		st.injVal, st.inj = x.value, x.value != (mathx.Vec2{})
		k++
	}
	if k > 0 {
		s.inj = append(s.inj[:0], s.inj[k:]...)
	}
}

func (s *Service) owner(dev int) int {
	bit := uint8(1) << dev
	for p := 0; p < s.players; p++ {
		if s.devs[p]&bit != 0 {
			return p
		}
	}
	return -1
}

func (s *Service) claim(dev int) int {
	if o := s.owner(dev); o >= 0 {
		return o
	}
	for p := 0; p < s.players; p++ {
		if s.devs[p] == 0 {
			s.devs[p] = uint8(1) << dev
			return p
		}
	}
	return -1
}

func (s *Service) Assign(p, dev int) {
	if p < 0 || p >= s.players || dev < 0 || dev > raw.MaxPads {
		return
	}
	bit := uint8(1) << dev
	for i := range s.devs {
		s.devs[i] &^= bit
	}
	s.devs[p] |= bit
}

func anyKey(in *raw.State) bool {
	for k := raw.Scancode(1); k < raw.ScancodeCount; k++ {
		if in.KeyPressed(k) {
			return true
		}
	}
	for b := 0; b < int(raw.ButtonCount); b++ {
		if in.ButtonPressed(raw.Button(b)) {
			return true
		}
	}
	return false
}

func padBusy(in *raw.State, i int) bool {
	for b := raw.PadButton(0); b < raw.PadButtonCount; b++ {
		if in.PadPressed(i, b) {
			return true
		}
	}
	for a := raw.PadAxis(0); a < raw.PadAxisCount; a++ {
		if mathx.Abs(in.PadAxisValue(i, a)) > 0.6 {
			return true
		}
	}
	return false
}

func (s *Service) activity(in *raw.State) {
	if anyKey(in) {
		if p := s.claim(DeviceKeyboard); p >= 0 {
			s.device[p] = DeviceKeyboard
		}
	}
	for i := 0; i < raw.MaxPads; i++ {
		if in.PadConnected(i) && padBusy(in, i) {
			if p := s.claim(i + 1); p >= 0 {
				s.device[p] = int8(i + 1)
			}
		}
	}
}

func (s *Service) devices(in *raw.State) {
	for i := 0; i < raw.MaxPads; i++ {
		on := in.PadConnected(i)
		if on == s.padsOn[i] {
			continue
		}
		s.padsOn[i] = on
		s.changed = true
		dev := i + 1
		p := s.owner(dev)
		detail := "pad connected"
		if !on {
			detail = "pad removed"
			if s.players > 1 && p >= 0 {
				s.devs[p] &^= uint8(1) << dev
			}
			for q := range s.device {
				if int(s.device[q]) == dev {
					s.device[q] = DeviceNone
				}
			}
		}
		s.events.Add(recorder.Input, ids.NodeID{}, ids.NodeID{}, "device "+strconv.Itoa(dev), detail)
		for _, fn := range s.listeners {
			fn(p, dev)
		}
	}
}
