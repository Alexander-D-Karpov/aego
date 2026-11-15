package audio

import (
	"errors"
	"slices"
	"strconv"

	"aego/asset"
	_ "aego/audio/codec/vorbis"
	_ "aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/audio/pump"
	"aego/core/ids"
	"aego/core/log"
	"aego/data"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/settings"
	"aego/platform"
	"aego/platform/headless"
	"aego/schema"
	"aego/script/bind"
)

const (
	ID            = "audio"
	MixerID       = ID + ".mixer"
	KeyBufferSize = ID + ".bufferSize"
	KeySampleRate = ID + ".sampleRate"
	KeyEnabled    = ID + ".enabled"
	SystemUpdate  = ID + ".update"
	MasterBus     = "master"
	Voices        = 64
	Block         = 512
	DefaultFrames = 512
)

var (
	Buses       = []string{"master", "music", "sfx", "ui", "ambience", "voice"}
	titles      = []string{"Master", "Music", "Effects", "Interface", "Ambience", "Voice"}
	BufferSizes = []int{256, 512, 1024, 2048}
)

func VolumeKey(bus string) string {
	return ID + ".bus." + bus + ".volume"
}

func MuteKey(bus string) string {
	return ID + ".bus." + bus + ".mute"
}

func BusTitle(i int) string {
	if i < 0 || i >= len(titles) {
		return ""
	}
	return titles[i]
}

func BusDescs() []mix.BusDesc {
	out := make([]mix.BusDesc, len(Buses))
	for i, b := range Buses {
		out[i] = mix.BusDesc{ID: b}
		if i > 0 {
			out[i].Parent = MasterBus
		}
	}
	return out
}

type PlayOptions struct {
	Gain     float32
	Pan      float32
	Pitch    float32
	Priority int32
	Loop     bool
	FadeIn   float32
}

type EventCounts struct {
	Finished uint64
	Stolen   uint64
	Rejected uint64
	Starved  uint64
}

type Module struct {
	st              *settings.Store
	vol             []settings.Key
	mute            []settings.Key
	bufKey          settings.Key
	rateKey         settings.Key
	enabled         settings.Key
	eng             *engine.Engine
	log             *log.ChannelLogger
	mixer           *mix.Mixer
	pump            *pump.Pump
	worker          workerBackend
	out             platform.AudioOutput
	spec            platform.AudioSpec
	requested       platform.AudioSpec
	manual          bool
	paced           bool
	minBlocks       int
	frameDt         float64
	keyRate         int
	key             string
	previewKeyRate  int
	previewKey      string
	listeners       []func(mix.Event)
	counts          EventCounts
	started         module.Signal0
	finished        module.Signal1[bool]
	worlds          []*Service
	session         [busCount]float32
	smute           [busCount]bool
	clips           map[clipCacheKey]*Clip
	warned          map[ids.AssetID]bool
	preview         mix.VoiceID
	previewCancel   chan struct{}
	previewPaused   bool
	previewEpoch    uint64
	previewFrames   int64
	previewRate     int
	previewArtifact *asset.AudioArtifact
	previewDigest   [32]byte
	previewGuard    bool
	previewGain     float32
	previewLoop     bool
	previewClose    func()
	previewDone     <-chan struct{}
	previewTime     *streamSeekTimeline
	streamSeek      previewStreamSeek
	solo            [busCount]bool
	previewID       ids.AssetID
}

func New() *Module {
	m := &Module{}
	for i := range m.session {
		m.session[i] = 1
	}
	return m
}

func (*Module) IconDir() string {
	return "modules/audio/res/icons/src"
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true, DisplayName: "Audio", DocsOrder: 130, DocsIcon: "cat.audio",
		Codes: []string{CodeFormat, CodeLoop, CodeNoClip, CodeNoListener, CodeListeners, CodeEvent}}
}

func choice(name, label string, v int) settings.Choice {
	return settings.Choice{Name: name, Label: label, Value: schema.Enum(int64(v))}
}

func (m *Module) Register(r *module.Registry) error {
	both := settings.User | settings.Project
	r.Setting(settings.Desc{
		ID: KeyEnabled, Kind: schema.KindBool, Default: schema.Bool(true), Title: "Sound",
		Doc:      "Turns all sound off without closing the audio output.",
		Category: settings.CategoryAudio, Order: -10, Flags: settings.User, Apply: m.enabledApply,
	})
	for i, b := range Buses {
		apply := m.busApply(i)
		r.Setting(settings.Desc{
			ID: VolumeKey(b), Kind: schema.KindFloat64, Default: schema.Float64(1), Title: titles[i] + " Volume",
			Category: settings.CategoryAudio, Order: i * 10, Flags: both, Min: 0, Max: 1, HasMin: true, HasMax: true, Step: 0.05,
			Apply: apply,
		})
		r.Setting(settings.Desc{
			ID: MuteKey(b), Kind: schema.KindBool, Title: "Mute " + titles[i],
			Category: settings.CategoryAudio, Order: i*10 + 1, Flags: both, Apply: apply,
		})
	}
	sizes := make([]settings.Choice, len(BufferSizes))
	for i, n := range BufferSizes {
		sizes[i] = choice("frames"+strconv.Itoa(n), strconv.Itoa(n), n)
	}
	r.Setting(settings.Desc{
		ID: KeyBufferSize, Kind: schema.KindEnum, Default: schema.Enum(DefaultFrames), Title: "Audio Buffer", Unit: "frames",
		Doc:      "Frames per audio block. Smaller blocks lower the latency, larger ones survive slow frames.",
		Category: settings.CategoryAudio, Order: 100, Flags: settings.User, Choices: sizes, Apply: m.deviceApply,
	})
	r.Setting(settings.Desc{
		ID: KeySampleRate, Kind: schema.KindEnum, Default: schema.Enum(0), Title: "Sample Rate", Unit: "Hz",
		Doc:      "Auto uses the rate of the output device.",
		Category: settings.CategoryAudio, Order: 110, Flags: settings.User,
		Choices: []settings.Choice{choice("auto", "Auto", 0), choice("hz44100", "44100", 44100), choice("hz48000", "48000", 48000)},
		Apply:   m.deviceApply,
	})
	if err := m.registerTypes(r); err != nil {
		return err
	}
	r.Validator(string(asset.ArtAudio), checkClip)
	registerDev(r)
	registerDocs(r)
	r.System(engine.SystemDesc{ID: SystemUpdate, Phase: engine.PhaseFrameBegin, Scope: engine.ScopeFrame, System: updateSystem{m: m}})
	r.System(engine.SystemDesc{ID: SystemReady, Phase: engine.PhasePreUpdate, Scope: engine.ScopePlay, System: readySystem{}})
	r.System(engine.SystemDesc{ID: SystemWorld, Phase: engine.PhasePostUpdate, Scope: engine.ScopePlay, Pause: engine.PauseAlways, System: worldSystem{}})
	if b := r.Bindings(); b != nil {
		if err := errors.Join(
			bind.AssetRefType[ClipRef](b, string(asset.ArtAudio)),
			bind.AssetRefType[data.Ref[Event]](b, bind.DataPrefix+"AudioEvent"),
		); err != nil {
			return err
		}
		installScript(r.ScriptModule(ScriptModule), m)
		if err := bindEvent(b); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) busApply(i int) func(schema.Value) error {
	return func(schema.Value) error {
		m.syncBus(i)
		return nil
	}
}

func (m *Module) syncBus(i int) {
	if m.mixer == nil || m.st == nil || i >= len(m.vol) {
		return
	}
	m.mixer.SetBusGain(i, float32(m.st.Float(m.vol[i])))
	m.mixer.SetBusMute(i, m.st.Bool(m.mute[i]))
}

func (m *Module) deviceApply(schema.Value) error {
	if m.mixer == nil {
		return nil
	}
	if want := m.wanted(); want != m.requested {
		return m.Reopen(want)
	}
	return nil
}

func (m *Module) wanted() platform.AudioSpec {
	s := platform.AudioSpec{Channels: 2, Frames: DefaultFrames}
	if m.st == nil {
		return s
	}
	if m.bufKey != 0 {
		if n := int(m.st.Int(m.bufKey)); n > 0 {
			s.Frames = n
		}
	}
	if m.rateKey != 0 {
		s.SampleRate = int(m.st.Int(m.rateKey))
	}
	return s
}

func (m *Module) Init(ctx *module.InitContext) error {
	m.st = ctx.Settings
	if m.st != nil {
		m.vol, m.mute = m.vol[:0], m.mute[:0]
		for _, b := range Buses {
			m.vol = append(m.vol, m.st.Key(VolumeKey(b)))
			m.mute = append(m.mute, m.st.Key(MuteKey(b)))
		}
		m.bufKey, m.rateKey = m.st.Key(KeyBufferSize), m.st.Key(KeySampleRate)
		m.enabled = m.st.Key(KeyEnabled)
	}
	if ctx.Engine == nil {
		return nil
	}
	ctx.Engine.SetService(MixerID, m)
	m.eng = ctx.Engine
	lg := ctx.Log
	if lg == nil {
		lg = ctx.Engine.Log()
	}
	m.log = lg.With(log.Engine)
	want := m.wanted()
	out := m.open(want)
	spec := out.Spec()
	mx, err := mix.New(mix.Config{SampleRate: spec.SampleRate, Channels: 2, Voices: Voices, Buses: BusDescs(), Block: Block})
	if err != nil {
		out.Close()
		return err
	}
	m.mixer, m.out, m.spec, m.requested = mx, out, spec, want
	m.drive(out, spec)
	m.pump = pump.New(mx, out, pumpConfig(out, spec))
	for i := range Buses {
		m.syncBus(i)
	}
	m.syncEnabled()
	m.worker, err = m.newWorkerBackend()
	if err != nil {
		m.pump.Close()
		m.pump, m.mixer, m.out = nil, nil, nil
		return err
	}
	if m.worker == nil {
		m.startPump()
	}
	m.log.Info("audio output", log.F("rate", spec.SampleRate), log.F("frames", spec.Frames), log.F("channels", spec.Channels))
	return nil
}

func (m *Module) open(want platform.AudioSpec) platform.AudioOutput {
	if a, ok := m.eng.Platform().(platform.Audioer); ok {
		out, err := a.OpenAudio(want)
		if err == nil {
			return out
		}
		m.log.Warn("the audio device is not available, sound is muted", log.F("error", err.Error()))
	}
	return headless.NewAudio(want, 0, m.eng.Platform().Now)
}

const maxBlocks = 12

type frameDriven interface {
	FrameDriven() bool
}

// drive fills the mixer from the frame loop for a frame-driven output or a headless engine on the frame-clocked output; any other output gets the pump goroutine.
func (m *Module) drive(out platform.AudioOutput, spec platform.AudioSpec) {
	f, ok := out.(frameDriven)
	m.paced = ok && f.FrameDriven()
	_, clocked := out.(*headless.Audio)
	m.manual = m.paced || (m.eng.Config().Headless && clocked)
	m.minBlocks = pumpConfig(out, spec).Target
	m.frameDt = 0
}

func (m *Module) startPump() {
	if !m.manual {
		m.pump.Start()
		return
	}
	if out, ok := m.out.(interface{ SetRefill(func() int) }); ok {
		out.SetRefill(m.pump.Fill)
	}
}

func pumpConfig(out platform.AudioOutput, spec platform.AudioSpec) pump.Config {
	target := 3
	if t, ok := out.(interface{ TargetBlocks() int }); ok {
		target = t.TargetBlocks()
	}
	return pump.Config{SampleRate: spec.SampleRate, Channels: spec.Channels, Block: spec.Frames, Target: target}
}

func (m *Module) Reopen(want platform.AudioSpec) error {
	if m.mixer == nil {
		return nil
	}
	m.bumpPreviewEpoch()
	if m.worker != nil {
		return m.worker.Reopen(want)
	}
	m.pump.Stop()
	m.out.Close()
	out := m.open(want)
	spec := out.Spec()
	if spec.SampleRate != m.mixer.SampleRate() {
		m.mixer.SetSampleRate(spec.SampleRate)
	}
	m.out, m.spec, m.requested = out, spec, want
	m.drive(out, spec)
	m.pump.SetOutput(out, pumpConfig(out, spec))
	m.startPump()
	m.log.Info("audio output reopened", log.F("rate", spec.SampleRate), log.F("frames", spec.Frames))
	return nil
}

func (m *Module) Shutdown() {
	m.endPreview()
	for _, s := range m.worlds {
		s.drop(true, 0)
	}
	m.worlds = nil
	if m.worker != nil {
		m.worker.Close()
		m.worker = nil
	}
	if m.pump != nil {
		m.pump.Close()
		m.pump = nil
	}
	m.mixer, m.out = nil, nil
}

type updateSystem struct {
	m *Module
}

func (s updateSystem) Run(ctx *engine.Context) {
	s.m.update(ctx.Unscaled)
}

func (m *Module) pace(dt float64) {
	if dt <= 0 || m.spec.SampleRate <= 0 || m.spec.Frames <= 0 {
		return
	}
	block := float64(m.spec.Frames) / float64(m.spec.SampleRate)
	// A late frame needs additional audio before its next render begins.
	// Raise the reserve immediately and release it on subsequent healthy frames.
	dt = min(dt, float64(maxBlocks)*block)
	if dt > m.frameDt {
		m.frameDt = dt
	} else {
		m.frameDt += (dt - m.frameDt) * .1
	}
	n := int(1.5*m.frameDt/block+0.999) + 1
	m.pump.SetTarget(min(max(n, m.minBlocks), maxBlocks))
}

func (m *Module) update(dt float64) {
	if m.mixer == nil {
		return
	}
	if m.worker != nil {
		m.worker.Tick()
	} else if m.manual {
		if m.paced {
			m.pace(dt)
		}
		m.pump.Fill()
	}
	m.pollStreamSeek()
	q := m.mixer.Events()
	for {
		ev, ok := q.Pop()
		if !ok {
			return
		}
		switch ev.Kind {
		case mix.EvFinished:
			m.counts.Finished++
		case mix.EvStolen:
			m.counts.Stolen++
		case mix.EvRejected:
			m.counts.Rejected++
		case mix.EvStarved:
			m.counts.Starved++
		}
		if ev.Voice == m.preview && ev.Kind != mix.EvStarved {
			m.reconcileStreamSeek()
			if m.streamSeek.pending != nil {
				if !m.mixer.Live(m.preview) {
					m.closePreviewFeeder()
				}
			} else if ev.Voice == m.preview {
				m.endPreview()
			}
		}
		for _, s := range m.worlds {
			if s.onEvent(ev) {
				break
			}
		}
		for _, fn := range m.listeners {
			fn(ev)
		}
	}
}

func (m *Module) Play(src *mix.Memory, bus string, o PlayOptions) mix.VoiceID {
	return m.play(src, nil, bus, o)
}

func (m *Module) PlayStream(st *mix.Stream, bus string, o PlayOptions) mix.VoiceID {
	return m.play(nil, st, bus, o)
}

func (m *Module) play(mem *mix.Memory, st *mix.Stream, bus string, o PlayOptions) mix.VoiceID {
	if m.mixer == nil {
		return mix.VoiceID{}
	}
	if bus == "" {
		bus = "sfx"
	}
	bi := slices.Index(Buses, bus)
	if bi < 0 {
		return mix.VoiceID{}
	}
	gain := o.Gain
	if gain == 0 {
		gain = 1
	}
	return m.mixer.Play(mix.PlayParams{
		Memory: mem, Stream: st, Bus: bi, Gain: gain, Pan: o.Pan, Pitch: o.Pitch,
		Priority: o.Priority, Loop: o.Loop, FadeIn: o.FadeIn,
	})
}

func (m *Module) Stop(id mix.VoiceID, fade float32) bool {
	return m.mixer != nil && m.mixer.Stop(id, fade)
}

func (m *Module) OnEvent(fn func(mix.Event)) {
	m.listeners = append(m.listeners, fn)
}

func (m *Module) Mixer() *mix.Mixer {
	return m.mixer
}

func (m *Module) Spec() platform.AudioSpec {
	return m.spec
}

func (m *Module) Counts() EventCounts {
	return m.counts
}

func (m *Module) Stats() pump.Stats {
	if m.worker != nil {
		return m.worker.Stats()
	}
	if m.pump == nil {
		return pump.Stats{}
	}
	return m.pump.Stats()
}

func (m *Module) Volume(bus string) float32 {
	i := slices.Index(Buses, bus)
	if i < 0 || m.st == nil || len(m.vol) == 0 {
		return 1
	}
	if m.st.Bool(m.mute[0]) || m.st.Bool(m.mute[i]) {
		return 0
	}
	v := m.st.Float(m.vol[i])
	if i > 0 {
		v *= m.st.Float(m.vol[0])
	}
	return float32(v)
}

func ModuleOf(e *engine.Engine) *Module {
	m, _ := engine.Service[*Module](e, MixerID)
	return m
}
