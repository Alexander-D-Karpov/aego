//go:build js && wasm

package main

import (
	"encoding/json"
	"math"
	"syscall/js"

	"aego/audio/mix"
	"aego/audio/pump"
	"aego/platform"
	"aego/platform/web"
)

type audioBench struct {
	out    *web.Audio
	pump   *pump.Pump
	target int
}

type audioReport struct {
	web.AudioStats
	Queued    int    `json:"queued"`
	Target    int    `json:"targetBlocks"`
	Frames    int    `json:"blockFrames"`
	Blocks    uint64 `json:"blocks"`
	Underflow uint64 `json:"pumpUnderruns"`
	MixMaxUs  int64  `json:"mixMaxUs"`
}

var bench audioBench

func init() {
	api := js.Global().Get("Object").New()
	api.Set("start", js.FuncOf(func(js.Value, []js.Value) any { return bench.start() }))
	api.Set("stop", js.FuncOf(func(js.Value, []js.Value) any {
		bench.stop()
		return nil
	}))
	api.Set("stats", js.FuncOf(func(js.Value, []js.Value) any { return bench.stats() }))
	js.Global().Set("aegoAudio", api)
}

func tone(rate int) *mix.Memory {
	pcm := make([]float32, rate*2)
	for f := 0; f < rate; f++ {
		s := 0.5 * float32(math.Sin(2*math.Pi*440*float64(f)/float64(rate)))
		pcm[2*f], pcm[2*f+1] = s, s
	}
	return &mix.Memory{PCM: pcm, Rate: rate, Channels: 2}
}

func (b *audioBench) start() string {
	b.stop()
	out, err := web.OpenAudio(platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512})
	if err != nil {
		return err.Error()
	}
	spec := out.Spec()
	m, err := mix.New(mix.Config{SampleRate: spec.SampleRate, Channels: 2})
	if err != nil {
		out.Close()
		return err.Error()
	}
	m.Play(mix.PlayParams{Memory: tone(spec.SampleRate), Gain: 1, Pitch: 1, Loop: true})
	b.out = out
	b.target = out.TargetBlocks()
	b.pump = pump.New(m, out, pump.Config{SampleRate: spec.SampleRate, Channels: 2, Block: spec.Frames, Target: b.target})
	b.pump.Start()
	return ""
}

func (b *audioBench) stop() {
	if b.pump != nil {
		b.pump.Close()
		b.pump, b.out = nil, nil
		b.target = 0
	}
}

func (b *audioBench) stats() string {
	if b.out == nil {
		return "{}"
	}
	s := b.pump.Stats()
	r := audioReport{AudioStats: b.out.Stats(), Queued: b.out.Queued(), Target: b.target, Frames: b.out.Spec().Frames, Blocks: s.Blocks, Underflow: s.Underruns, MixMaxUs: s.MixMax.Microseconds()}
	data, _ := json.Marshal(r)
	return string(data)
}
