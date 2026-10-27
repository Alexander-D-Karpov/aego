package audio

import (
	"slices"

	"aego/asset"
	"aego/audio/mix"
	"aego/contracts/camera2d"
	"aego/contracts/scriptwait"
	"aego/contracts/signals"
	"aego/core/clock"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/core/rng"
	"aego/data"
	"aego/engine"
	"aego/engine/recorder"
	"aego/scene"
	"aego/script/abi"
	"aego/script/vm"
)

const (
	ServiceID     = ID + ".voices"
	SystemReady   = ID + ".ready"
	SystemWorld   = ID + ".world"
	ScriptModule  = "aego/audio"
	KindSound     = ScriptModule + ".Sound"
	sigCompleted  = 0
	OwnerFade     = 0.05
	stealFade     = 0.02
	musicPriority = 100
	epsilon       = 1e-3
	rngSeed       = 0x5eeda0d1
	recPlay       = "audio.play"
	recStop       = "audio.stop"
)

var typeSound = abi.HandleID(KindSound)

type Sound uint64

func soundOf(k uint64) Sound {
	return Sound(k)
}

func (s Sound) key() uint64 {
	return uint64(s)
}

func (s Sound) voice() mix.VoiceID {
	return mix.VoiceID{Index: uint32(s), Gen: uint32(s >> 32)}
}

func soundFor(id mix.VoiceID) Sound {
	return Sound(uint64(id.Gen)<<32 | uint64(id.Index))
}

type Options struct {
	Volume   float32
	Pitch    float32
	Pan      float32
	Bus      Bus
	Loop     bool
	FadeIn   float32
	Priority int64
	Unscaled bool
}

type voiceRec struct {
	id       mix.VoiceID
	owner    scene.NodeHandle
	event    ids.AssetID
	sp       Spatial
	at       mathx.Vec2
	volume   float32
	clipGain float32
	sentG    float32
	sentP    float32
	born     uint64
	cancel   chan struct{}
	source   bool
	spatial  bool
	follow   bool
	game     bool
	held     bool
	user     bool
	stopped  bool
	music    bool
	length   float32
}

type zoneState struct {
	node   scene.NodeHandle
	inside bool
	seen   bool
	seq    uint64
}

type coolEntry struct {
	id    ids.AssetID
	until float64
}

type playReq struct {
	clip    ids.AssetID
	event   ids.AssetID
	owner   scene.NodeHandle
	sp      Spatial
	at      mathx.Vec2
	volume  float32
	pitch   float32
	pan     float32
	fade    float32
	bus     int
	prio    int32
	source  bool
	spatial bool
	follow  bool
	loop    bool
	game    bool
	music   bool
}

type Service struct {
	m       *Module
	scene   *scene.Scene
	world   *engine.World
	hub     signals.Hub
	rng     *rng.Source
	recs    []voiceRec
	zones   []zoneState
	cool    []coolEntry
	pending []scene.NodeHandle
	worldG  [busCount]float32
	fades   [busCount]float32
	music   mix.VoiceID
	listen  mathx.Vec2
	time    float64
	seq     uint64
	born    uint64
	ready   bool
	paused  bool
	all     bool
	lib     *data.Library
}

func NewService(m *Module, s *scene.Scene, w *engine.World) *Service {
	svc := &Service{m: m, scene: s, world: w, rng: rng.New(rngSeed)}
	for b := range svc.worldG {
		svc.worldG[b] = 1
	}
	if w != nil {
		svc.hub, _ = engine.ServiceOf[signals.Hub](w, signals.ServiceID)
	}
	return svc
}

func Of(w *engine.World) *Service {
	if w == nil {
		return nil
	}
	s, _ := engine.ServiceOf[*Service](w, ServiceID)
	return s
}

func (s *Service) mixer() *mix.Mixer {
	if s.m == nil {
		return nil
	}
	return s.m.mixer
}

func (s *Service) replaying() bool {
	return s.world.Replaying()
}

func (s *Service) find(id mix.VoiceID) int {
	if !id.Valid() {
		return -1
	}
	for i := range s.recs {
		if s.recs[i].id == id {
			return i
		}
	}
	return -1
}

func (s *Service) alive(key uint64) bool {
	return s.find(Sound(key).voice()) >= 0
}

func translation(m mathx.Mat3) mathx.Vec2 {
	return mathx.Vec2{X: m[6], Y: m[7]}
}

func (s *Service) posOf(owner scene.NodeHandle, at mathx.Vec2, follow bool) mathx.Vec2 {
	if follow && !owner.IsZero() && s.scene.IsAlive(owner) {
		return translation(s.scene.RenderWorld(owner))
	}
	return at
}

type VoiceInfo struct {
	Position mathx.Vec2
	Gain     float32
	Spatial  bool
	Source   bool
	Owner    scene.NodeHandle
}

func (s *Service) Scene() *scene.Scene {
	return s.scene
}

func (s *Service) Listener() mathx.Vec2 {
	return s.listen
}

func (s *Service) EachVoice(fn func(VoiceInfo)) {
	for i := range s.recs {
		r := &s.recs[i]
		if r.stopped {
			continue
		}
		fn(VoiceInfo{Position: s.posOf(r.owner, r.at, r.follow), Gain: r.sentG, Spatial: r.spatial, Source: r.source, Owner: r.owner})
	}
}

func (s *Service) SourceLength(h scene.NodeHandle) float32 {
	for i := range s.recs {
		if r := &s.recs[i]; r.source && r.owner == h && !r.stopped {
			return r.length
		}
	}
	return 0
}

func (s *Service) resolver() asset.Resolver {
	if s.scene == nil {
		return nil
	}
	return s.scene.Resolver()
}

func (s *Service) record(name string, owner scene.NodeHandle) {
	if s.world == nil || s.world.Recorder == nil {
		return
	}
	var id ids.NodeID
	if !owner.IsZero() && s.scene != nil && s.scene.IsAlive(owner) {
		id = s.scene.ID(owner)
	}
	s.world.Recorder.Add(recorder.Audio, id, ids.NodeID{}, name, "")
}

func (s *Service) start(r *playReq) Sound {
	mx := s.mixer()
	if mx == nil || r.clip.IsZero() {
		return 0
	}
	s.record(recPlay, r.owner)
	if s.replaying() {
		return 0
	}
	c, err := s.m.clip(s.resolver(), r.clip)
	if err != nil {
		s.m.warnAsset(r.clip, err)
		return 0
	}
	bus := r.bus
	if bus < 0 || bus >= busCount {
		bus = c.Bus
	}
	vol := r.volume * c.Gain
	gain, pan := vol, r.pan
	if r.spatial {
		g, p := r.sp.At(s.listen, s.posOf(r.owner, r.at, r.follow))
		gain, pan = vol*g, p
	}
	p := mix.PlayParams{Bus: bus, Gain: gain, Pan: pan, Pitch: r.pitch, Priority: r.prio, Loop: r.loop, FadeIn: r.fade}
	var cancel chan struct{}
	if c.Mem != nil {
		p.Memory = c.Mem
	} else {
		st, cn, err := c.stream(r.loop)
		if err != nil {
			s.m.warnAsset(r.clip, err)
			return 0
		}
		p.Stream, cancel = st, cn
	}
	id := mx.Play(p)
	if !id.Valid() {
		if cancel != nil {
			close(cancel)
		}
		return 0
	}
	s.born++
	rec := voiceRec{
		id: id, owner: r.owner, event: r.event, sp: r.sp, at: r.at, volume: vol, clipGain: c.Gain,
		sentG: gain, sentP: pan, born: s.born, cancel: cancel, source: r.source, spatial: r.spatial,
		follow: r.follow, game: r.game, music: r.music, length: c.Seconds,
	}
	if s.wantPause(&rec) {
		rec.held = true
		mx.Pause(id)
	}
	s.recs = append(s.recs, rec)
	return soundFor(id)
}

func (s *Service) request(clip ids.AssetID, o Options, owner scene.NodeHandle) playReq {
	vol, pitch := o.Volume, o.Pitch
	if vol == 0 {
		vol = 1
	}
	if pitch == 0 {
		pitch = 1
	}
	bus := int(o.Bus)
	if o.Bus == BusMaster {
		bus = -1
	}
	return playReq{
		clip: clip, owner: owner, volume: vol, pitch: pitch, pan: o.Pan, bus: bus, loop: o.Loop,
		fade: o.FadeIn, prio: int32(min(max(o.Priority, 0), 255)), game: !o.Unscaled,
	}
}

func (s *Service) Play(clip ids.AssetID, o Options, owner scene.NodeHandle) Sound {
	r := s.request(clip, o, owner)
	return s.start(&r)
}

func (s *Service) PlayAt(clip ids.AssetID, pos mathx.Vec2, o Options, owner scene.NodeHandle) Sound {
	r := s.request(clip, o, owner)
	r.spatial, r.at, r.sp = true, pos, defaultSpatial
	return s.start(&r)
}

func (s *Service) PlayEvent(ev ids.AssetID, owner scene.NodeHandle) Sound {
	return s.playEvent(ev, owner, mathx.Vec2{}, false, false)
}

func (s *Service) PlayEventAt(ev ids.AssetID, pos mathx.Vec2, owner scene.NodeHandle) Sound {
	return s.playEvent(ev, owner, pos, true, false)
}

func (s *Service) library() (*data.Library, error) {
	if s.lib == nil {
		src, ok := s.scene.Resolver().(asset.DataSource)
		if !ok {
			return nil, data.ErrNoSource
		}
		s.lib = data.NewLibrary(s.scene.Registry(), src)
	}
	return s.lib, nil
}

func (s *Service) event(id ids.AssetID) (*Event, error) {
	l, err := s.library()
	if err != nil {
		return nil, err
	}
	return data.Get[Event](l, id)
}

func (s *Service) playEvent(id ids.AssetID, owner scene.NodeHandle, at mathx.Vec2, positioned, source bool) Sound {
	ev, err := s.event(id)
	if err != nil {
		s.m.warnAsset(id, err)
		return 0
	}
	return s.playResolved(id, ev, owner, at, positioned, source)
}

func (s *Service) Trigger(key ids.AssetID, ev *Event, owner scene.NodeHandle) Sound {
	if ev == nil {
		return 0
	}
	return s.playResolved(key, ev, owner, mathx.Vec2{}, false, false)
}

func (s *Service) TriggerAt(key ids.AssetID, ev *Event, pos mathx.Vec2, owner scene.NodeHandle) Sound {
	if ev == nil {
		return 0
	}
	return s.playResolved(key, ev, owner, pos, true, false)
}

func (s *Service) playResolved(id ids.AssetID, ev *Event, owner scene.NodeHandle, at mathx.Vec2, positioned, source bool) Sound {
	if !s.allow(id, ev) {
		return 0
	}
	clip, ok := s.variant(ev)
	if !ok {
		return 0
	}
	vol := ev.Volume * (1 - ev.VolumeRand*s.rng.Float32())
	pitch := ev.Pitch * (1 + ev.PitchRand*(2*s.rng.Float32()-1))
	r := playReq{
		clip: clip, event: id, owner: owner, at: at, volume: vol, pitch: pitch, bus: int(ev.Bus), loop: ev.Loop,
		prio: ev.Priority, source: source, game: ev.PauseWith == PauseGame, spatial: ev.Spatial, follow: !positioned,
		sp: Spatial{Min: ev.MinDistance, Max: ev.MaxDistance, Width: ev.PanWidth, Rolloff: ev.Rolloff},
	}
	return s.start(&r)
}

func (s *Service) variant(ev *Event) (ids.AssetID, bool) {
	n := 0
	for _, v := range ev.Variants {
		if !v.IsZero() {
			n++
		}
	}
	if n == 0 {
		return ids.AssetID{}, false
	}
	k := min(int(s.rng.Float32()*float32(n)), n-1)
	for _, v := range ev.Variants {
		if v.IsZero() {
			continue
		}
		if k == 0 {
			return v.ID, true
		}
		k--
	}
	return ids.AssetID{}, false
}

func (s *Service) coolIndex(id ids.AssetID) int {
	for i := range s.cool {
		if s.cool[i].id == id {
			return i
		}
	}
	return -1
}

func (s *Service) allow(id ids.AssetID, ev *Event) bool {
	if ev.Cooldown > 0 {
		until := s.time + float64(ev.Cooldown)
		switch i := s.coolIndex(id); {
		case i >= 0 && s.time < s.cool[i].until:
			return false
		case i >= 0:
			s.cool[i].until = until
		default:
			s.cool = append(s.cool, coolEntry{id: id, until: until})
		}
	}
	if ev.MaxVoices <= 0 {
		return true
	}
	for {
		n, victim := 0, -1
		mx := s.mixer()
		for i := range s.recs {
			r := &s.recs[i]
			if r.event != id || r.stopped {
				continue
			}
			n++
			switch {
			case victim < 0:
				victim = i
			case ev.Steal == StealQuietest && mx != nil && mx.VoiceGain(r.id) < mx.VoiceGain(s.recs[victim].id):
				victim = i
			case ev.Steal != StealQuietest && r.born < s.recs[victim].born:
				victim = i
			}
		}
		if n < int(ev.MaxVoices) {
			return true
		}
		if ev.Steal == StealNone || victim < 0 {
			return false
		}
		s.stopRec(victim, stealFade)
	}
}

func (s *Service) Music(clip ids.AssetID, fade float32) Sound {
	s.StopMusic(fade)
	r := playReq{clip: clip, volume: 1, pitch: 1, bus: int(BusMusic), loop: true, fade: fade, prio: musicPriority, music: true}
	snd := s.start(&r)
	s.music = snd.voice()
	return snd
}

func (s *Service) StopMusic(fade float32) {
	if i := s.find(s.music); i >= 0 {
		s.stopRec(i, fade)
	}
	s.music = mix.VoiceID{}
}

func (s *Service) stopRec(i int, fade float32) {
	r := &s.recs[i]
	if r.stopped {
		return
	}
	r.stopped = true
	s.record(recStop, r.owner)
	if mx := s.mixer(); mx != nil {
		mx.Stop(r.id, fade)
	}
}

func (s *Service) Stop(h Sound, fade float32) {
	if i := s.find(h.voice()); i >= 0 {
		s.stopRec(i, fade)
	}
}

func (s *Service) Pause(h Sound, on bool) {
	if i := s.find(h.voice()); i >= 0 {
		s.recs[i].user = on
		s.syncPause(&s.recs[i])
	}
}

func (s *Service) SetVolume(h Sound, v float32) {
	i := s.find(h.voice())
	if i < 0 {
		return
	}
	r := &s.recs[i]
	r.volume = max(v, 0) * r.clipGain
	if mx := s.mixer(); mx != nil && !r.spatial && !r.stopped {
		mx.SetGain(r.id, r.volume, 0)
		r.sentG = r.volume
	}
}

func (s *Service) SetPitch(h Sound, p float32) {
	if i := s.find(h.voice()); i >= 0 {
		if mx := s.mixer(); mx != nil {
			mx.SetPitch(s.recs[i].id, p)
		}
	}
}

func (s *Service) SetPan(h Sound, p float32) {
	i := s.find(h.voice())
	if i < 0 || s.recs[i].spatial {
		return
	}
	if mx := s.mixer(); mx != nil {
		mx.SetPan(s.recs[i].id, p)
		s.recs[i].sentP = p
	}
}

func (s *Service) Done(h Sound) bool {
	return s.find(h.voice()) < 0
}

func (s *Service) Playing(h Sound) bool {
	i := s.find(h.voice())
	return i >= 0 && !s.recs[i].stopped && !s.recs[i].held
}

func (s *Service) Position(h Sound) float32 {
	if mx := s.mixer(); mx != nil && s.find(h.voice()) >= 0 {
		return mx.Position(h.voice())
	}
	return 0
}

func (s *Service) PauseAll(on bool) {
	s.all = on
	for i := range s.recs {
		s.syncPause(&s.recs[i])
	}
}

func (s *Service) Voices() int {
	return len(s.recs)
}

func (s *Service) PlaySource(h scene.NodeHandle) Sound {
	c, ok := scene.Get[AudioSource2D](s.scene, h)
	if !ok {
		return 0
	}
	s.StopSource(h, stealFade)
	var snd Sound
	if !c.Event.IsZero() {
		snd = s.playEvent(c.Event.ID, h, mathx.Vec2{}, false, true)
	} else {
		r := playReq{
			clip: c.Clip.ID, owner: h, source: true, volume: c.Volume, pitch: c.Pitch, pan: c.Pan, bus: int(c.Bus),
			loop: c.Loop, game: c.PauseWith == PauseGame, spatial: c.Spatial, follow: true,
			sp: Spatial{Min: c.MinDistance, Max: c.MaxDistance, Width: c.PanWidth, Rolloff: c.Rolloff},
		}
		snd = s.start(&r)
	}
	if snd == 0 {
		return 0
	}
	c.Playing, c.Position = true, 0
	s.m.started.Emit(s.world, h)
	return snd
}

func (s *Service) StopSource(h scene.NodeHandle, fade float32) {
	for i := range s.recs {
		if r := &s.recs[i]; r.source && r.owner == h {
			s.stopRec(i, fade)
		}
	}
}

func (s *Service) SourcePlaying(h scene.NodeHandle) bool {
	for i := range s.recs {
		if r := &s.recs[i]; r.source && r.owner == h && !r.stopped {
			return true
		}
	}
	return false
}

func (s *Service) wantPause(r *voiceRec) bool {
	return r.user || s.all || (r.game && s.paused)
}

func (s *Service) syncPause(r *voiceRec) {
	want := s.wantPause(r)
	if want == r.held || r.stopped {
		return
	}
	r.held = want
	if mx := s.mixer(); mx != nil {
		if want {
			mx.Pause(r.id)
		} else {
			mx.Resume(r.id)
		}
	}
}

func (s *Service) startOnPlay() {
	s.pending = s.pending[:0]
	scene.Query1[AudioSource2D](s.scene).Each(func(h scene.NodeHandle, c *AudioSource2D) {
		if c.PlayOnStart && s.scene.EffectiveEnabled(h) {
			s.pending = append(s.pending, h)
		}
	})
	for _, h := range s.pending {
		s.PlaySource(h)
	}
}

func (s *Service) listener() mathx.Vec2 {
	found := false
	var pos mathx.Vec2
	scene.Query1[AudioListener2D](s.scene).Each(func(h scene.NodeHandle, l *AudioListener2D) {
		if found || !l.Active || !s.scene.EffectiveEnabled(h) {
			return
		}
		found, pos = true, translation(s.scene.RenderWorld(h))
	})
	if found {
		return pos
	}
	if v, ok := engine.ServiceOf[camera2d.Viewer](s.world, camera2d.ServiceID); ok {
		if p, ok := v.ViewCenter(); ok {
			return p
		}
	}
	return mathx.Vec2{}
}

func (s *Service) zoneFor(h scene.NodeHandle) *zoneState {
	for i := range s.zones {
		if s.zones[i].node == h {
			return &s.zones[i]
		}
	}
	s.zones = append(s.zones, zoneState{node: h})
	return &s.zones[len(s.zones)-1]
}

func (s *Service) updateZones() {
	for i := range s.zones {
		s.zones[i].seen = false
	}
	scene.Query1[AudioZone2D](s.scene).Each(func(h scene.NodeHandle, z *AudioZone2D) {
		in := false
		if s.scene.EffectiveEnabled(h) && z.Area.W > 0 && z.Area.H > 0 {
			if inv, ok := s.scene.World(h).Inverse(); ok {
				p, a := inv.Apply(s.listen), z.Area
				in = p.X >= a.X && p.X <= a.X+a.W && p.Y >= a.Y && p.Y <= a.Y+a.H
			}
		}
		st := s.zoneFor(h)
		st.seen = true
		if in && !st.inside {
			s.seq++
			st.seq = s.seq
		}
		st.inside = in
	})
	n := 0
	for _, z := range s.zones {
		if z.seen {
			s.zones[n] = z
			n++
		}
	}
	s.zones = s.zones[:n]
	s.applyZones()
}

func (s *Service) applyZones() {
	mx := s.mixer()
	for b := 0; b < busCount; b++ {
		target := float32(1)
		var best *AudioZone2D
		var bestSeq uint64
		for i := range s.zones {
			z := &s.zones[i]
			if !z.inside {
				continue
			}
			c, ok := scene.Get[AudioZone2D](s.scene, z.node)
			if !ok || int(c.Bus) != b {
				continue
			}
			if best == nil || c.Priority > best.Priority || (c.Priority == best.Priority && z.seq > bestSeq) {
				best, bestSeq = c, z.seq
			}
		}
		if best != nil {
			target, s.fades[b] = best.Gain, best.FadeTime
		}
		if target == s.worldG[b] || mx == nil || s.replaying() {
			continue
		}
		s.worldG[b] = target
		mx.SetBusLayer(b, mix.LayerWorld, target, s.fades[b])
	}
}

func (s *Service) update(ctx *engine.Context) {
	if t := ctx.Time; t != nil {
		s.time += t.Span(clock.Game).Delta
		s.paused = t.Paused()
	}
	s.listen = s.listener()
	s.updateZones()
	mx := s.mixer()
	if mx == nil {
		return
	}
	for i := 0; i < len(s.recs); i++ {
		r := &s.recs[i]
		if r.stopped {
			continue
		}
		if !r.owner.IsZero() && !s.scene.IsAlive(r.owner) {
			s.stopRec(i, OwnerFade)
			continue
		}
		s.syncPause(r)
		if r.spatial {
			g, p := r.sp.At(s.listen, s.posOf(r.owner, r.at, r.follow))
			g *= r.volume
			if mathx.Abs(g-r.sentG) > epsilon {
				mx.SetGain(r.id, g, 0)
				r.sentG = g
			}
			if mathx.Abs(p-r.sentP) > epsilon {
				mx.SetPan(r.id, p)
				r.sentP = p
			}
		}
		if r.source {
			if c, ok := scene.Get[AudioSource2D](s.scene, r.owner); ok {
				c.Playing, c.Position = true, mx.Position(r.id)
			}
		}
	}
}

func (s *Service) complete(key uint64, finished bool) {
	src := signals.Source{Kind: signals.KindHandle, A: key, B: typeSound}
	if s.hub != nil {
		if s.hub.Wants(src) {
			args := s.hub.Args(1)
			args[0] = vm.Bool(finished)
			s.hub.Emit(src, sigCompleted, args)
		}
		s.hub.End(src)
	}
	if w, ok := engine.ServiceOf[scriptwait.Waker](s.world, scriptwait.ServiceID); ok {
		w.Complete(typeSound, key)
	}
}

func (s *Service) finish(r *voiceRec, finished bool) {
	if r.cancel != nil {
		close(r.cancel)
		r.cancel = nil
	}
	if r.music && s.music == r.id {
		s.music = mix.VoiceID{}
	}
	if r.source && s.scene.IsAlive(r.owner) {
		if !s.SourcePlaying(r.owner) {
			if c, ok := scene.Get[AudioSource2D](s.scene, r.owner); ok {
				c.Playing = false
			}
		}
		s.m.finished.Emit(s.world, r.owner, finished)
	}
	s.complete(soundFor(r.id).key(), finished)
}

func (s *Service) onEvent(ev mix.Event) bool {
	i := s.find(ev.Voice)
	if i < 0 {
		return false
	}
	if ev.Kind == mix.EvStarved {
		return true
	}
	r := s.recs[i]
	s.recs = slices.Delete(s.recs, i, i+1)
	s.finish(&r, ev.Kind == mix.EvFinished && !r.stopped)
	return true
}

func (s *Service) drop(kill bool, fade float32) {
	mx := s.mixer()
	recs := s.recs
	s.recs = nil
	for i := range recs {
		r := &recs[i]
		if mx != nil {
			if kill {
				mx.Kill(r.id)
			} else if !r.stopped {
				mx.Stop(r.id, fade)
			}
		}
		s.finish(r, false)
	}
	s.music = mix.VoiceID{}
}

func (s *Service) close() {
	s.drop(false, OwnerFade)
	if mx := s.mixer(); mx != nil {
		for b := range s.worldG {
			if s.worldG[b] != 1 {
				mx.SetBusLayer(b, mix.LayerWorld, 1, OwnerFade)
				s.worldG[b] = 1
			}
		}
	}
}

type readySystem struct{}

func (readySystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil && !s.ready {
		s.ready = true
		s.startOnPlay()
	}
}

type worldSystem struct{}

func (worldSystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.update(ctx)
	}
}
