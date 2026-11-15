package mix

import (
	"errors"
	"math"
	"slices"
	"sync/atomic"
)

const (
	DefaultVoices = 64
	DefaultBlock  = 512
	MaxBlock      = 4096
	MinPitch      = 0.25
	MaxPitch      = 4
	MaxRateRatio  = 4
	queueSize     = 1024
	gainRamp      = 0.010
	busRamp       = 0.010
)

type BusDesc struct {
	ID     string
	Parent string
}

type Config struct {
	SampleRate int
	Channels   int
	Voices     int
	Buses      []BusDesc
	Block      int
}

type VoiceID struct {
	Index uint32
	Gen   uint32
}

func (v VoiceID) Valid() bool {
	return v.Gen != 0
}

type PlayParams struct {
	Memory   *Memory
	Stream   *Stream
	Bus      int
	Gain     float32
	Pan      float32
	Pitch    float32
	Priority int32
	Loop     bool
	FadeIn   float32
}

const (
	LayerUser uint8 = iota
	LayerSession
	LayerWorld
)

type handle struct {
	gen     atomic.Uint32
	pos     atomic.Uint32
	gain    atomic.Uint32
	started atomic.Bool
	voice   int32
}

type bus struct {
	id      string
	parent  int
	depth   int
	user    float32
	session float32
	world   float32
	mute    bool
	solo    bool
	audible bool
	gain    float32
	target  float32
	delta   float32
	ramp    int
	live    bool
	acc     []float32
	peak    atomic.Uint32
	rms     atomic.Uint32
	level   atomic.Uint32
}

type Mixer struct {
	cfg     Config
	rate    float64
	srate   atomic.Int32
	buses   []bus
	order   []int
	byID    map[string]int
	master  int
	voices  []voice
	freeV   []int32
	handles []handle
	hfree   *spsc[uint32]
	spare   []uint32
	cmds    CommandQueue
	events  EventQueue
	paused  bool
	clock   uint64
	ages    uint64
	applied uint64
	active  atomic.Int32
	refused atomic.Uint64
	// Remote handle releases are independent of the bounded, lossy public event queue.
	onRelease func(VoiceID)
	remote    bool
}

func New(cfg Config) (*Mixer, error) {
	switch {
	case cfg.SampleRate <= 0:
		return nil, errors.New("mix: the sample rate must be positive")
	case cfg.Channels != 0 && cfg.Channels != 2:
		return nil, errors.New("mix: the output must be stereo")
	}
	cfg.Channels = 2
	if cfg.Voices <= 0 {
		cfg.Voices = DefaultVoices
	}
	if cfg.Block <= 0 {
		cfg.Block = DefaultBlock
	}
	cfg.Block = min(cfg.Block, MaxBlock)
	if len(cfg.Buses) == 0 {
		cfg.Buses = []BusDesc{{ID: "master"}}
	}
	m := &Mixer{cfg: cfg, rate: float64(cfg.SampleRate)}
	m.srate.Store(int32(cfg.SampleRate))
	if err := m.initBuses(cfg.Buses); err != nil {
		return nil, err
	}
	m.voices = make([]voice, cfg.Voices)
	m.freeV = make([]int32, 0, cfg.Voices)
	for i := cfg.Voices - 1; i >= 0; i-- {
		m.freeV = append(m.freeV, int32(i))
	}
	n := 1
	for n < cfg.Voices*4 {
		n <<= 1
	}
	m.handles = make([]handle, n)
	m.hfree = newSPSC[uint32](n)
	for i := range m.handles {
		m.handles[i].gen.Store(1)
		m.handles[i].voice = -1
		m.hfree.push(uint32(i))
	}
	m.spare = make([]uint32, 0, n)
	m.cmds.q = newSPSC[Command](queueSize)
	m.cmds.wake = make(chan struct{}, 1)
	m.events.q = newSPSC[Event](queueSize)
	return m, nil
}

func (m *Mixer) initBuses(descs []BusDesc) error {
	m.byID = make(map[string]int, len(descs))
	for i, d := range descs {
		if d.ID == "" {
			return errors.New("mix: a bus needs an id")
		}
		if _, dup := m.byID[d.ID]; dup {
			return errors.New("mix: bus " + d.ID + " is declared twice")
		}
		m.byID[d.ID] = i
	}
	m.buses = make([]bus, len(descs))
	m.master = -1
	for i, d := range descs {
		b := &m.buses[i]
		b.id, b.parent, b.user, b.session, b.world, b.gain, b.target, b.audible = d.ID, -1, 1, 1, 1, 1, 1, true
		b.acc = make([]float32, m.cfg.Block*2)
		b.level.Store(math.Float32bits(1))
		if d.Parent == "" {
			if m.master >= 0 {
				return errors.New("mix: buses " + descs[m.master].ID + " and " + d.ID + " both have no parent")
			}
			m.master = i
			continue
		}
		p, ok := m.byID[d.Parent]
		if !ok {
			return errors.New("mix: bus " + d.ID + " has an unknown parent " + d.Parent)
		}
		b.parent = p
	}
	if m.master < 0 {
		return errors.New("mix: there is no master bus without a parent")
	}
	for i := range m.buses {
		depth := 0
		for j := m.buses[i].parent; j >= 0; j = m.buses[j].parent {
			depth++
			if depth > len(m.buses) {
				return errors.New("mix: bus " + descs[i].ID + " is part of a cycle")
			}
		}
		m.buses[i].depth = depth
	}
	m.order = make([]int, len(m.buses))
	for i := range m.order {
		m.order[i] = i
	}
	slices.SortStableFunc(m.order, func(a, b int) int { return m.buses[b].depth - m.buses[a].depth })
	return nil
}

func (m *Mixer) Reserve() VoiceID {
	if n := len(m.spare); n > 0 {
		h := m.spare[n-1]
		m.spare = m.spare[:n-1]
		return VoiceID{Index: h, Gen: m.handles[h].gen.Load()}
	}
	h, ok := m.hfree.pop()
	if !ok {
		m.refused.Add(1)
		return VoiceID{}
	}
	return VoiceID{Index: h, Gen: m.handles[h].gen.Load()}
}

func (m *Mixer) Play(p PlayParams) VoiceID {
	id := m.Reserve()
	if !id.Valid() {
		return id
	}
	pitch := p.Pitch
	if pitch == 0 {
		pitch = 1
	}
	ok := m.cmds.Push(Command{
		Kind: CmdPlay, Voice: id, Bus: int32(p.Bus), Memory: p.Memory, Stream: p.Stream,
		Gain: p.Gain, Pan: p.Pan, Pitch: pitch, Fade: p.FadeIn, Priority: p.Priority, Flag: p.Loop,
	})
	if !ok {
		m.spare = append(m.spare, id.Index)
		return VoiceID{}
	}
	return id
}

func (m *Mixer) Stop(id VoiceID, fade float32) bool {
	return m.cmds.Push(Command{Kind: CmdStop, Voice: id, Fade: fade})
}

// StopPair queues one stop for up to two owned voices, ignores stale IDs and reports enqueueing, not application.
func (m *Mixer) StopPair(first, second VoiceID, fade float32) bool {
	if !first.Valid() && !second.Valid() {
		return false
	}
	return m.cmds.Push(Command{Kind: CmdStopPair, Voice: first, Previous: second, Fade: fade})
}

func (m *Mixer) SetGain(id VoiceID, gain, fade float32) bool {
	return m.cmds.Push(Command{Kind: CmdSetGain, Voice: id, Gain: gain, Fade: fade})
}

func (m *Mixer) SetPan(id VoiceID, pan float32) bool {
	return m.cmds.Push(Command{Kind: CmdSetPan, Voice: id, Pan: pan})
}

func (m *Mixer) SetPitch(id VoiceID, pitch float32) bool {
	return m.cmds.Push(Command{Kind: CmdSetPitch, Voice: id, Pitch: pitch})
}

func (m *Mixer) Pause(id VoiceID) bool {
	return m.cmds.Push(Command{Kind: CmdPause, Voice: id})
}

func (m *Mixer) Resume(id VoiceID) bool {
	return m.cmds.Push(Command{Kind: CmdResume, Voice: id})
}

// SeekMemory queues a memory-voice frame, not a stream seek; stale/stopping voices and out-of-range or loop-exceeding frames are ignored, and device buffers remain.
func (m *Mixer) SeekMemory(id VoiceID, frame int64) bool {
	// Keep requested frames exactly representable as float64, including non-editor commands.
	if !id.Valid() || frame < 0 || frame > 1<<53 {
		return false
	}
	return m.cmds.Push(Command{Kind: CmdSeekMemory, Voice: id, Frame: frame})
}

// ReplaceStream queues an unused stream with initialFrame 0 or 1 for a preceding interpolation frame; rejection preserves the old voice, and callers retire both feeders.
func (m *Mixer) ReplaceStream(previous VoiceID, p PlayParams, initialFrame int64, paused bool) VoiceID {
	if !previous.Valid() || p.Stream == nil || p.Memory != nil || initialFrame < 0 || initialFrame > 1 {
		return VoiceID{}
	}
	id := m.Reserve()
	if !id.Valid() {
		return id
	}
	pitch := p.Pitch
	if pitch == 0 {
		pitch = 1
	}
	if !m.cmds.Push(Command{Kind: CmdReplaceStream, Voice: id, Previous: previous, Stream: p.Stream,
		Bus: int32(p.Bus), Gain: p.Gain, Pan: p.Pan, Pitch: pitch, Fade: p.FadeIn, Priority: p.Priority,
		Flag: p.Loop, Frame: initialFrame, Paused: paused}) {
		m.spare = append(m.spare, id.Index)
		return VoiceID{}
	}
	return id
}

func (m *Mixer) SetBusGain(b int, gain float32) bool {
	return m.cmds.Push(Command{Kind: CmdBusGain, Bus: int32(b), Gain: gain})
}

func (m *Mixer) SetBusMute(b int, on bool) bool {
	return m.cmds.Push(Command{Kind: CmdBusMute, Bus: int32(b), Flag: on})
}

func (m *Mixer) SetBusSolo(b int, on bool) bool {
	return m.cmds.Push(Command{Kind: CmdBusSolo, Bus: int32(b), Flag: on})
}

func (m *Mixer) PauseAll() bool {
	return m.cmds.Push(Command{Kind: CmdPauseAll})
}

func (m *Mixer) ResumeAll() bool {
	return m.cmds.Push(Command{Kind: CmdResumeAll})
}

func (m *Mixer) SampleRate() int {
	return int(m.srate.Load())
}

func (m *Mixer) Block() int {
	return m.cfg.Block
}

func (m *Mixer) Bus(id string) (int, bool) {
	i, ok := m.byID[id]
	return i, ok
}

func (m *Mixer) BusCount() int {
	return len(m.buses)
}

func (m *Mixer) Meters(b int) (peak, rms float32) {
	if b < 0 || b >= len(m.buses) {
		return 0, 0
	}
	return math.Float32frombits(m.buses[b].peak.Load()), math.Float32frombits(m.buses[b].rms.Load())
}

func (m *Mixer) BusGain(b int) float32 {
	if b < 0 || b >= len(m.buses) {
		return 0
	}
	return math.Float32frombits(m.buses[b].level.Load())
}

func (m *Mixer) Active() int {
	return int(m.active.Load())
}

func (m *Mixer) Refused() uint64 {
	return m.refused.Load()
}

func (m *Mixer) Commands() *CommandQueue {
	return &m.cmds
}

func (m *Mixer) Events() *EventQueue {
	return &m.events
}

func (m *Mixer) SetSampleRate(rate int) {
	if rate <= 0 {
		return
	}
	m.rate = float64(rate)
	m.cfg.SampleRate = rate
	m.srate.Store(int32(rate))
	for i := range m.voices {
		v := &m.voices[i]
		switch {
		case v.state == voiceFree:
			continue
		case v.mem != nil:
			v.ratio = float64(v.mem.Rate) / m.rate
		default:
			v.ratio = float64(v.st.Rate) / m.rate
		}
		v.step = float64(v.pitch) * v.ratio
	}
}

func (m *Mixer) SetBusLayer(b int, layer uint8, gain, fade float32) bool {
	return m.cmds.Push(Command{Kind: CmdBusLayer, Bus: int32(b), Layer: layer, Gain: gain, Fade: fade})
}

func (m *Mixer) Kill(id VoiceID) bool {
	return m.cmds.Push(Command{Kind: CmdKill, Voice: id})
}

func (m *Mixer) Live(id VoiceID) bool {
	return id.Gen != 0 && int(id.Index) < len(m.handles) && m.handles[id.Index].gen.Load() == id.Gen
}

// Started reports an applied, unreleased voice; queued voices are Live first, and remote voices may finish before a validated batch observes them.
func (m *Mixer) Started(id VoiceID) bool {
	if id.Gen == 0 || int(id.Index) >= len(m.handles) {
		return false
	}
	h := &m.handles[id.Index]
	return h.gen.Load() == id.Gen && h.started.Load() && h.gen.Load() == id.Gen
}

func (m *Mixer) Position(id VoiceID) float32 {
	if !m.Live(id) {
		return 0
	}
	return math.Float32frombits(m.handles[id.Index].pos.Load())
}

func (m *Mixer) VoiceGain(id VoiceID) float32 {
	if !m.Live(id) {
		return 0
	}
	return math.Float32frombits(m.handles[id.Index].gain.Load())
}
