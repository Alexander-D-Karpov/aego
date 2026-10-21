package audio

import (
	"bytes"
	_ "embed"
	"math"

	"aego/audio/codec"
	_ "aego/audio/codec/vorbis"
	"aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/audio/pump"
	"aego/core/log"
	"aego/core/mathx"
	"aego/demos"
	"aego/engine"
	"aego/gpu"
	"aego/platform"
	"aego/platform/input"
)

const shotRate = 48000

//go:embed music.ogg
var music []byte

func init() {
	demos.Register("audio", func() demos.Demo { return New() })
}

type Demo struct {
	demos.Base
	e        *engine.Engine
	log      *log.ChannelLogger
	mixer    *mix.Mixer
	pump     *pump.Pump
	manual   bool
	shot     *mix.Memory
	stream   *mix.Stream
	cancel   chan struct{}
	done     <-chan error
	music    mix.VoiceID
	musicBus int
	sfxBus   int
	pan      float32
	pitch    float32
	muted    bool
	shots    int
}

func New() *Demo {
	return &Demo{pitch: 1}
}

func (d *Demo) Name() string {
	return "audio"
}

func shotPCM() []float32 {
	out := make([]float32, shotRate/4)
	seed := uint32(2463534242)
	for i := range out {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		noise := float32(seed)/float32(math.MaxUint32)*2 - 1
		t := float64(i) / shotRate
		thump := float32(math.Sin(2*math.Pi*90*t) * math.Exp(-t*30))
		out[i] = 0.8 * float32(math.Exp(-t*18)) * (0.6*noise + 0.6*thump)
	}
	return out
}

func (d *Demo) Init(e *engine.Engine) error {
	d.e = e
	d.log = e.Log().With(log.Game)
	a, ok := e.Platform().(platform.Audioer)
	if !ok {
		d.log.Warn("this platform has no audio output")
		return nil
	}
	out, err := a.OpenAudio(platform.AudioSpec{Channels: 2, Frames: 512})
	if err != nil {
		d.log.Warn("no audio output", log.F("error", err.Error()))
		return nil
	}
	spec := out.Spec()
	m, err := mix.New(mix.Config{SampleRate: spec.SampleRate, Channels: 2, Buses: []mix.BusDesc{
		{ID: "master"}, {ID: "music", Parent: "master"}, {ID: "sfx", Parent: "master"},
	}})
	if err != nil {
		out.Close()
		return err
	}
	d.musicBus, _ = m.Bus("music")
	d.sfxBus, _ = m.Bus("sfx")
	shot, err := codec.Open(bytes.NewReader(wav.Encode(shotPCM(), shotRate, 1, wav.EncodeOptions{Bits: 16})), "shot.wav")
	if err == nil {
		d.shot, err = codec.DecodeAll(shot, spec.SampleRate)
	}
	if err != nil {
		out.Close()
		return err
	}
	dec, err := codec.Open(bytes.NewReader(music), "music.ogg")
	if err != nil {
		out.Close()
		return err
	}
	info := dec.Info()
	d.stream = mix.NewStream(info.SampleRate, info.Channels, info.SampleRate)
	d.cancel = make(chan struct{})
	d.done = codec.Stream(dec, d.stream, codec.StreamOptions{Loop: true}, d.cancel)
	d.mixer = m
	d.pump = pump.New(m, out, pump.Config{SampleRate: spec.SampleRate, Channels: 2, Block: spec.Frames, Target: 3})
	d.manual = e.Config().Headless
	if !d.manual {
		d.pump.Start()
	}
	d.music = m.Play(mix.PlayParams{Stream: d.stream, Bus: d.musicBus, Gain: 0.6, Pitch: 1, FadeIn: 1, Priority: 10})
	d.log.Info("space shoots, left and right pan the music, up and down change its pitch, M mutes it; unplug the headphones to see the sound move",
		log.F("rate", spec.SampleRate), log.F("frames", spec.Frames))
	return nil
}

func (d *Demo) Event(ev platform.Event) {
	switch ev.Kind {
	case platform.EvAudioDeviceAdded, platform.EvAudioDeviceRemoved, platform.EvAudioDeviceFormatChanged:
		d.log.Info(ev.Kind.String(), log.F("device", ev.Audio.ID), log.F("ours", ev.Audio.Default))
	}
}

func (d *Demo) Update(dt float64) {
	in := d.e.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.e.Quit()
	}
	if d.mixer == nil {
		return
	}
	if in.KeyPressed(input.KeySpace) {
		pans := [3]float32{-0.8, 0, 0.8}
		d.mixer.Play(mix.PlayParams{
			Memory: d.shot, Bus: d.sfxBus, Gain: 0.8, Pan: pans[d.shots%3],
			Pitch: 1 + 0.05*float32(d.shots%5), Priority: 1,
		})
		d.shots++
	}
	pan := d.pan
	if in.KeyDown(input.KeyLeft) {
		pan -= float32(dt)
	}
	if in.KeyDown(input.KeyRight) {
		pan += float32(dt)
	}
	if pan = min(max(pan, -1), 1); pan != d.pan {
		d.pan = pan
		d.mixer.SetPan(d.music, pan)
	}
	pitch := d.pitch
	if in.KeyDown(input.KeyUp) {
		pitch *= float32(math.Exp(dt))
	}
	if in.KeyDown(input.KeyDown) {
		pitch /= float32(math.Exp(dt))
	}
	if pitch = min(max(pitch, mix.MinPitch), mix.MaxPitch); pitch != d.pitch {
		d.pitch = pitch
		d.mixer.SetPitch(d.music, pitch)
	}
	if in.KeyPressed(input.KeyM) {
		d.muted = !d.muted
		d.mixer.SetBusMute(d.musicBus, d.muted)
	}
	if d.manual {
		d.pump.Fill()
	}
	for {
		ev, ok := d.mixer.Events().Pop()
		if !ok {
			break
		}
		switch ev.Kind {
		case mix.EvStolen:
			d.log.Debug("voice stolen")
		case mix.EvStarved:
			d.log.Debug("music starved")
		}
	}
}

func (d *Demo) Render(float64) {
	fr := d.e.Frame()
	if fr == nil {
		return
	}
	var peak float32
	if d.mixer != nil {
		peak, _ = d.mixer.Meters(0)
	}
	m := d.e.Metrics()
	p := fr.BeginPass(gpu.PassDesc{
		Color:      gpu.LoadClear,
		ClearColor: mathx.Color{R: 0.04 + 0.6*peak, G: 0.05 + 0.1*(d.pitch-1), B: 0.07 + 0.2*(d.pan+1), A: 1},
		Label:      "audio",
	})
	p.SetViewport(mathx.Rect{W: float32(m.PixelW), H: float32(m.PixelH)})
	p.End()
}

func (d *Demo) Shutdown() {
	if d.cancel != nil {
		close(d.cancel)
		<-d.done
		d.cancel = nil
	}
	if d.pump != nil {
		d.pump.Close()
		d.pump = nil
	}
}
