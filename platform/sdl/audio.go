package sdl

/*
#include "aego_sdl.h"
*/
import "C"

import (
	"sync"
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
	"aego/platform"
)

type Audio struct {
	mu   sync.Mutex
	p    *Platform
	s    *C.SDL_AudioStream
	spec platform.AudioSpec
	dev  uint32
}

func (p *Platform) OpenAudio(want platform.AudioSpec) (platform.AudioOutput, error) {
	ch := want.Channels
	if ch <= 0 {
		ch = 2
	}
	frames := want.Frames
	if frames <= 0 {
		frames = platform.DefaultAudioFrames
	}
	var rate, got C.int
	s := C.aego_audio_open(C.int(want.SampleRate), C.int(ch), C.int(frames), &rate, &got)
	if s == nil {
		return nil, errs.New(errs.PlatformInit, "SDL audio output failed", log.F("sdl", lastError()))
	}
	a := &Audio{p: p, s: s, spec: platform.AudioSpec{SampleRate: int(rate), Channels: ch, Frames: int(got)}, dev: uint32(C.aego_audio_device(s))}
	p.audio = a
	return a, nil
}

func (a *Audio) Spec() platform.AudioSpec {
	return a.spec
}

func (a *Audio) Write(s []float32) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := len(s) / a.spec.Channels
	if a.s == nil || n == 0 {
		return 0
	}
	if !C.aego_audio_put(a.s, (*C.float)(unsafe.Pointer(unsafe.SliceData(s))), C.int(n*a.spec.Channels*4)) {
		return 0
	}
	return n
}

func (a *Audio) Queued() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.s == nil {
		return 0
	}
	return int(C.aego_audio_queued(a.s)) / (4 * a.spec.Channels)
}

func (a *Audio) Pause(on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.s != nil {
		C.aego_audio_pause(a.s, boolInt(on))
	}
}

func (a *Audio) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.s == nil {
		return
	}
	C.aego_audio_close(a.s)
	a.s = nil
	if a.p != nil && a.p.audio == a {
		a.p.audio = nil
	}
}

var (
	_ platform.Audioer     = (*Platform)(nil)
	_ platform.AudioOutput = (*Audio)(nil)
)
