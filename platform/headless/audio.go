package headless

import (
	"sync"

	"aego/platform"
)

type Audio struct {
	mu        sync.Mutex
	spec      platform.AudioSpec
	frameTime float64
	now       func() float64
	last      float64
	started   bool
	carry     float64
	queued    int
	written   int64
	consumed  int64
	paused    bool
	stalled   bool
	closed    bool
	sink      func([]float32)
}

func NewAudio(spec platform.AudioSpec, frameTime float64, now func() float64) *Audio {
	return &Audio{spec: spec.WithDefaults(), frameTime: frameTime, now: now}
}

func (p *Platform) OpenAudio(want platform.AudioSpec) (platform.AudioOutput, error) {
	if p.audio != nil {
		p.audio.Close()
	}
	a := NewAudio(want, p.opts.FrameTime, p.clock.Now)
	p.audio = a
	return a, nil
}

func (a *Audio) Spec() platform.AudioSpec {
	return a.spec
}

func (a *Audio) Write(s []float32) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return 0
	}
	n := len(s) / a.spec.Channels
	a.queued += n
	a.written += int64(n)
	if a.sink != nil {
		a.sink(s[:n*a.spec.Channels])
	}
	return n
}

func (a *Audio) Queued() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.frameTime <= 0 && a.now != nil {
		a.tickClock()
	}
	return a.queued
}

func (a *Audio) Pause(on bool) {
	a.mu.Lock()
	a.paused = on
	a.mu.Unlock()
}

func (a *Audio) Close() {
	a.mu.Lock()
	a.closed = true
	a.queued = 0
	a.mu.Unlock()
}

func (a *Audio) Written() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.written
}

func (a *Audio) Consumed() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.consumed
}

func (a *Audio) SetSink(fn func([]float32)) {
	a.mu.Lock()
	a.sink = fn
	a.mu.Unlock()
}

func (a *Audio) Stall(on bool) {
	a.mu.Lock()
	a.stalled = on
	a.mu.Unlock()
}

func (a *Audio) advance() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.frameTime > 0 {
		a.consume(a.frameTime)
	} else if a.now != nil {
		a.tickClock()
	}
}

func (a *Audio) tickClock() {
	t := a.now()
	if !a.started {
		a.started, a.last = true, t
		return
	}
	a.consume(t - a.last)
	a.last = t
}

func (a *Audio) consume(dt float64) {
	if a.paused || a.stalled || a.closed || dt <= 0 {
		return
	}
	a.carry += dt * float64(a.spec.SampleRate)
	n := int(a.carry)
	a.carry -= float64(n)
	n = min(n, a.queued)
	a.queued -= n
	a.consumed += int64(n)
}

var (
	_ platform.Audioer     = (*Platform)(nil)
	_ platform.AudioOutput = (*Audio)(nil)
)
