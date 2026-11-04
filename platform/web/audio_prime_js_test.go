//go:build js

package web

import (
	"syscall/js"
	"testing"

	"aego/platform"
)

func TestAudioStartupBatchIsPublishedAtomically(t *testing.T) {
	g := js.Global()
	if !g.Get("SharedArrayBuffer").Truthy() || !g.Get("Atomics").Truthy() {
		t.Skip("SharedArrayBuffer is unavailable")
	}
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "messages", true: "shared"}[shared], func(t *testing.T) {
			ctx := g.Get("Object").New()
			ctx.Set("currentTime", 0)
			port := g.Get("Function").New(`return {writes:[], postMessage(samples) {this.writes.push(Array.from(samples))}}`).Invoke()
			a := &Audio{g: g, ctx: ctx, port: port, f32: g.Get("Float32Array"), u8: g.Get("Uint8Array"),
				atomics: g.Get("Atomics"), spec: platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 4},
				state: "running", ready: true, shared: shared}
			if shared {
				buf := g.Get("SharedArrayBuffer").New(ringFrames * 2 * 4)
				a.data = a.u8.New(buf)
				a.index = g.Get("Int32Array").New(g.Get("SharedArrayBuffer").New(16))
			}
			a.BeginAudioPrime(12)
			for i := range 3 {
				samples := make([]float32, 8)
				for j := range samples {
					samples[j] = float32(i + 1)
				}
				if n := a.Write(samples); n != 4 {
					t.Fatalf("write %d: %d frames", i, n)
				}
				if shared && a.atomics.Call("load", a.index, 1).Int() != 0 {
					t.Fatal("partial prime became visible in the shared ring")
				}
				if !shared && port.Get("writes").Length() != 0 {
					t.Fatal("partial prime was posted to the worklet")
				}
			}
			a.EndAudioPrime()
			if a.Queued() != 12 {
				t.Fatalf("committed queue contains %d frames, want 12", a.Queued())
			}
			var samples js.Value
			if shared {
				samples = g.Get("Float32Array").New(a.data.Get("buffer"))
			} else {
				if port.Get("writes").Length() != 1 {
					t.Fatal("startup was not sent in one message")
				}
				samples = port.Get("writes").Index(0)
			}
			for i := range 24 {
				if got, want := samples.Index(i).Float(), float64(i/8+1); got != want {
					t.Fatalf("sample %d: %v, want %v", i, got, want)
				}
			}
			// Only the initial prime is batched; later writes are visible at once.
			a.BeginAudioPrime(4)
			a.Write(make([]float32, 8))
			if a.Queued() != 16 {
				t.Fatal("subsequent playback was held behind another prime")
			}
			a.EndAudioPrime()
		})
	}
}

func TestAudioStartupBatchIsBoundedAndReportsFailedCommit(t *testing.T) {
	g := js.Global()
	ctx := g.Get("Object").New()
	ctx.Set("currentTime", 0)
	port := g.Get("Function").New(`return {writes:[], postMessage(samples) {this.writes.push(Array.from(samples))}}`).Invoke()
	a := &Audio{g: g, ctx: ctx, port: port, f32: g.Get("Float32Array"), u8: g.Get("Uint8Array"),
		spec: platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 4}, state: "running", ready: true}
	a.BeginAudioPrime(0)
	a.EndAudioPrime()
	if a.sentAudio || port.Get("writes").Length() != 0 {
		t.Fatal("empty prime sent audio")
	}
	a.BeginAudioPrime(3)
	if n := a.Write(make([]float32, 8)); n != 3 {
		t.Fatalf("prime accepted %d frames, want its limit of 3", n)
	}
	if n := a.Write(make([]float32, 8)); n != 0 {
		t.Fatalf("prime exceeded its limit: accepted %d frames", n)
	}
	a.closed = true
	a.EndAudioPrime()
	if a.failed == "" || port.Get("writes").Length() != 0 {
		t.Fatal("an interrupted initial commit was not reported")
	}
}
