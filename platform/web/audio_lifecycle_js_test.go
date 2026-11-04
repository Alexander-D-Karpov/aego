//go:build js

package web

import (
	"syscall/js"
	"testing"
	"time"

	"aego/platform"
)

func TestWebAudioCloseDuringWorkletLoad(t *testing.T) {
	g := js.Global()
	oldCtor, oldCtx := g.Get("AudioContext"), g.Get(sharedContextGlobal)
	t.Cleanup(func() { g.Set("AudioContext", oldCtor); g.Set(sharedContextGlobal, oldCtx) })
	g.Set("AudioContext", g.Get("Object"))
	for _, resolved := range []bool{true, false} {
		ctx := g.Get("Function").New(`return {
			state:"suspended",sampleRate:48000,
			addEventListener(){},removeEventListener(){},
			audioWorklet:{ addModule() { return new Promise((resolve,reject)=>{this.resolve=resolve;this.reject=reject}) } }
		}`).Invoke()
		g.Set(sharedContextGlobal, ctx)
		a, err := OpenAudio(platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512})
		if err != nil {
			t.Fatal(err)
		}
		a.SetRefill(func() int { t.Error("closed output requested PCM"); return 0 })
		if !a.loading || len(a.funcs) == 0 {
			t.Fatal("test did not reach pending addModule")
		}
		a.Close()
		if !a.closed || a.refill != nil || !a.loading || len(a.funcs) == 0 {
			t.Fatal("close did not detach safely from pending load")
		}
		pending := ctx.Get("audioWorklet")
		if resolved {
			pending.Call("resolve")
		} else {
			pending.Call("reject", g.Get("Error").New("cancelled"))
		}
		deadline := time.Now().Add(time.Second)
		for a.loading && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if a.loading || len(a.funcs) != 0 || currentAudio != nil || a.ready {
			t.Fatal("closed load retained callbacks or attached a node")
		}
		if ctx.Get(workletLoaded).Truthy() != resolved {
			t.Fatal("incorrect cached worklet registration")
		}
		a.Close()
	}
}

func TestWebAudioRefillsOnlyThePhysicallyRunningOutput(t *testing.T) {
	a := &Audio{g: js.Global(), pageCtx: true, state: "running", ready: true}
	b := &Audio{g: js.Global(), pageCtx: true, state: "running", ready: true}
	n, m := 0, 0
	a.SetRefill(func() int { n++; return 0 })
	b.SetRefill(func() int { m++; return 0 })
	defer b.Close()
	report := js.ValueOf(map[string]any{"time": 1, "consumed": 128, "underruns": 1, "sum": 32, "count": 256})
	a.hidden = true // suspension requested; physical completion has not happened
	a.report(report)
	if n != 1 || m != 0 || a.underruns != 1 {
		t.Fatal("starved the pending suspension, reset its counters or addressed the wrong device")
	}
	a.state = "suspended"
	a.report(report)
	a.muted = true
	a.state = "running"
	a.report(report)
	if n != 1 {
		t.Fatal("fed silent/suspended output")
	}
	a.fn(func(js.Value, []js.Value) any { return nil })
	a.Close()
	a.report(report)
	b.report(report)
	if n != 1 || m != 1 || len(a.funcs) != 0 || a.refill != nil {
		t.Fatal("closed callback retained or routed to another output")
	}
}

func TestWebAudioTargetUsesContextLatency(t *testing.T) {
	g := js.Global()
	if !g.Get("SharedArrayBuffer").Truthy() || !g.Get("Atomics").Truthy() {
		t.Skip("SharedArrayBuffer is unavailable")
	}
	oldCtor, oldCtx := g.Get("AudioContext"), g.Get(sharedContextGlobal)
	oldIsolated, oldBlocks := g.Get("crossOriginIsolated"), g.Get("aegoAudioBlocks")
	t.Cleanup(func() {
		g.Set("AudioContext", oldCtor)
		g.Set(sharedContextGlobal, oldCtx)
		g.Set("crossOriginIsolated", oldIsolated)
		g.Set("aegoAudioBlocks", oldBlocks)
	})
	g.Set("AudioContext", g.Get("Object"))
	for _, tt := range []struct {
		name     string
		shared   bool
		latency  any
		override int
		want     int
	}{
		{"shared context latency", true, 2048.0 / 48000, 0, 5},
		{"shared missing latency", true, js.Undefined(), 0, 3},
		{"shared ignores message override", true, 2048.0 / 48000, 7, 5},
		{"message context latency", false, 2048.0 / 48000, 0, 6},
		{"message missing latency", false, js.Undefined(), 0, 6},
		{"message diagnostic override below default", false, 2048.0 / 48000, 4, 4},
		{"message diagnostic override above default", false, 2048.0 / 48000, 8, 8},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := g.Get("Function").New(`return {
				state:"suspended",sampleRate:48000,
				addEventListener(){},removeEventListener(){},
				audioWorklet:{ addModule() { return new Promise(resolve=>{this.resolve=resolve}) } }
			}`).Invoke()
			ctx.Set("baseLatency", tt.latency)
			g.Set(sharedContextGlobal, ctx)
			g.Set("crossOriginIsolated", tt.shared)
			g.Set("aegoAudioBlocks", tt.override)
			a, err := OpenAudio(platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512})
			if err != nil {
				t.Fatal(err)
			}
			if a.shared != tt.shared || a.TargetBlocks() != tt.want {
				t.Errorf("shared %v, target %d; want shared %v, target %d", a.shared, a.TargetBlocks(), tt.shared, tt.want)
			}
			a.Close()
			ctx.Get("audioWorklet").Call("resolve")
			deadline := time.Now().Add(time.Second)
			for a.loading && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if a.loading {
				t.Error("closed worklet load did not settle")
			}
		})
	}
}
