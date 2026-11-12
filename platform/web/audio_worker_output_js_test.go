//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"

	"aego/platform"
)

func TestWebAudioWorkerOutputReservation(t *testing.T) {
	g := js.Global()
	for _, shared := range []bool{false, true} {
		a := &Audio{g: g, shared: shared, target: 3, spec: platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512}}
		if _, _, _, _, _, ok := a.WorkerOutput(); ok || !a.ReserveWorkerOutput() || a.ReserveWorkerOutput() {
			t.Fatal("untouched output was not reserved exactly once")
		}
		if _, _, _, _, _, ok := a.WorkerOutput(); ok {
			t.Fatal("returned an output before its worklet was ready")
		}
		if n := a.Write([]float32{1, 1}); n != 0 || a.sentAudio {
			t.Fatal("local PCM entered the reserved output")
		}
		a.ready, a.node = true, g.Get("Object").New()
		var dataBuffer, indexBuffer js.Value
		if shared {
			sab := g.Get("SharedArrayBuffer")
			if sab.Type() != js.TypeFunction {
				t.Fatal("test requires SharedArrayBuffer")
			}
			dataBuffer, indexBuffer = sab.New(ringFrames*2*4), sab.New(16)
			a.data = g.Get("Uint8Array").New(dataBuffer)
			a.index = g.Get("Int32Array").New(indexBuffer)
		}
		node, data, index, block, target, ok := a.WorkerOutput()
		wantTarget := 3072 // The existing message transport keeps six blocks.
		if shared {
			wantTarget = 1536
		}
		if !ok || !node.Equal(a.node) || block != 512 || target != wantTarget {
			t.Fatalf("unexpected worker endpoint: shared=%v block=%d target=%d ready=%v", shared, block, target, ok)
		}
		if shared && (!data.Equal(dataBuffer) || !index.Equal(indexBuffer)) {
			t.Fatal("shared endpoint did not return the original buffers")
		}
		if !shared && (data.Type() != js.TypeUndefined || index.Type() != js.TypeUndefined) {
			t.Fatal("message endpoint returned shared buffers")
		}
		if !a.CancelWorkerOutput() || a.CancelWorkerOutput() || a.workerReserved {
			t.Fatal("unattached output could not be released")
		}
		if !a.ReserveWorkerOutput() || !a.CommitWorkerOutput() || a.CommitWorkerOutput() || a.CancelWorkerOutput() {
			t.Fatal("attachment was not permanent")
		}
		if n := a.Write([]float32{1, 1}); n != 0 {
			t.Fatal("local PCM entered the attached worker output")
		}
	}
}

func TestWebAudioWorkerOutputRejectsExistingProducer(t *testing.T) {
	for _, a := range []*Audio{
		{closed: true},
		{failed: "worklet load failed"},
		{sentAudio: true},
		{priming: true},
		{refill: func() int { return 0 }},
	} {
		if a.ReserveWorkerOutput() {
			t.Fatal("claimed an output with a previous or active local producer")
		}
	}
	if (*Audio)(nil).ReserveWorkerOutput() {
		t.Fatal("claimed a nil output")
	}
}

func TestWebAudioWorkerOutputRejectsLocalRefill(t *testing.T) {
	a := &Audio{spec: platform.AudioSpec{Channels: 2, Frames: 512}}
	if !a.ReserveWorkerOutput() {
		t.Fatal("could not reserve untouched output")
	}
	a.SetRefill(func() int { t.Error("reserved output invoked local refill"); return 1 })
	if a.refill != nil || a.failed == "" {
		t.Fatal("reserved output accepted a local refill callback")
	}
	a.BeginAudioPrime(512)
	if a.priming {
		t.Fatal("reserved output began a local PCM prime")
	}
	if !a.CancelWorkerOutput() {
		t.Fatal("unattached failed claim could not be cancelled")
	}
}

func TestWebAudioWorkerReservationConfiguresWorkletRefills(t *testing.T) {
	g := js.Global()
	port := g.Get("Function").New(`return {messages:[],postMessage(m){this.messages.push(m)}}`).Invoke()
	a := &Audio{port: port, spec: platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512}}
	if a.refillThreshold() != 0 || !a.ReserveWorkerOutput() {
		t.Fatal("output did not start without a refill producer")
	}
	if a.refillThreshold() != 1152 || port.Get("messages").Length() != 1 ||
		port.Get("messages").Index(0).Get("refillAt").Int() != 1152 {
		t.Fatal("worker reservation did not configure the existing refill threshold")
	}
	if !a.CancelWorkerOutput() || a.refillThreshold() != 0 || port.Get("messages").Length() != 2 ||
		port.Get("messages").Index(1).Get("refillAt").Int() != 0 {
		t.Fatal("cancelling the reservation left worker refill requests enabled")
	}
	loading := &Audio{spec: a.spec}
	if !loading.ReserveWorkerOutput() || loading.refillThreshold() != 1152 {
		t.Fatal("worklet loading after reservation would not receive its refill threshold")
	}
}

func TestWebAudioWorkerReservationFailsClosedOnPortError(t *testing.T) {
	g := js.Global()
	badPort := g.Get("Function").New(`return {postMessage(){throw Error('closed port')}}`).Invoke()
	a := &Audio{port: badPort, spec: platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512}}
	if a.ReserveWorkerOutput() || a.workerReserved || a.failed == "" {
		t.Fatal("a failed threshold update retained an apparently valid claim")
	}
	b := &Audio{spec: a.spec}
	if !b.ReserveWorkerOutput() {
		t.Fatal("could not reserve an output before its worklet loaded")
	}
	b.port = badPort
	if b.CancelWorkerOutput() || !b.workerReserved || b.failed == "" {
		t.Fatal("a failed threshold reset incorrectly returned local ownership")
	}
}

func TestWebAudioWorkerGatePreservesUserMuteAndHoldsUnmute(t *testing.T) {
	g := js.Global()
	ctx := g.Get("Function").New(`return {currentTime:5,suspend(){},resume(){}}`).Invoke()
	param := g.Get("Function").New(`return {value:1,cancelScheduledValues(){},
		setValueAtTime(v){this.value=v},setTargetAtTime(v){this.value=v}}`).Invoke()
	gain := g.Get("Object").New()
	gain.Set("gain", param)
	a := &Audio{g: g, ctx: ctx, gain: gain, state: "running", ready: true}
	if a.SetWorkerStateListener(func() {}) || a.SetWorkerGate(true) || !a.ReserveWorkerOutput() {
		t.Fatal("worker controls ignored reservation ownership")
	}
	var states []bool
	if !a.SetWorkerStateListener(func() {
		states = append(states, a.WorkerSilent())
		if !a.SetWorkerGate(true) {
			t.Error("could not gate output on a lifecycle transition")
		}
	}) {
		t.Fatal("could not observe reserved output")
	}
	a.SetMuted(true)
	if !a.Muted() || param.Get("value").Float() != 0 {
		t.Fatal("mute did not silence the gain")
	}
	a.SetMuted(false)
	if a.Muted() || param.Get("value").Float() != 0 || len(states) != 2 || !states[0] || states[1] {
		t.Fatal("unmute escaped the worker gate or lost the user's preference")
	}
	if !a.SetWorkerGate(false) || param.Get("value").Float() != 1 {
		t.Fatal("acknowledged resume did not restore the user's level")
	}
	a.visibility(true)
	if !a.WorkerSilent() || param.Get("value").Float() != 0 {
		t.Fatal("pending visibility suspension was not immediately gated")
	}
	a.visibility(false)
	if a.WorkerSilent() || param.Get("value").Float() != 0 {
		t.Fatal("visibility resume escaped the gate before the worker was ready")
	}
	if !a.CancelWorkerOutput() || param.Get("value").Float() != 1 || a.workerState != nil {
		t.Fatal("safe startup cancellation did not restore the local output")
	}
}

func TestWebAudioWorkerGateFailureRequestsSuspension(t *testing.T) {
	g := js.Global()
	ctx := g.Get("Function").New(`return {currentTime:0,stopped:false,suspend(){this.stopped=true}}`).Invoke()
	gain := g.Get("Function").New(`return {gain:{cancelScheduledValues(){},setValueAtTime(){throw Error('gate failed')}}}`).Invoke()
	a := &Audio{g: g, ctx: ctx, gain: gain, workerReserved: true}
	if a.SetWorkerGate(true) || a.failed == "" || !ctx.Get("stopped").Bool() {
		t.Fatal("gate failure did not stop the output and report an error")
	}
}

func TestWebAudioWorkerBaselinePreservesCountersAndFinalQuietReport(t *testing.T) {
	g := js.Global()
	a := &Audio{g: g, workerReserved: true, ready: true, state: "running",
		prior: platform.AudioCounters{Underruns: 2, Consumed: 500, Sum: 4, Samples: 16, Suppressed: 32}}
	var signals int
	a.SetWorkerStateListener(func() { signals++ })
	a.report(js.ValueOf(map[string]any{"time": 1, "consumed": 128, "underruns": 1, "sum": 1, "count": 4,
		"quiet": true, "quietToken": 7, "suppressed": 64}))
	if !a.WorkerQuiet(7) || a.WorkerQuiet(6) || signals != 1 {
		t.Fatal("final worklet report did not publish quiet state")
	}
	want := platform.AudioCounters{Underruns: 3, Consumed: 628, Sum: 5, Samples: 20, Suppressed: 96}
	if got := a.WorkerCounters(); got != want {
		t.Fatalf("old output counters = %+v, want %+v", got, want)
	}
	b := &Audio{g: g, workerReserved: true, ready: true, state: "running"}
	if !b.SetWorkerBaseline(want) {
		t.Fatal("replacement rejected valid baseline")
	}
	b.report(js.ValueOf(map[string]any{"time": 2, "consumed": 256, "underruns": 1, "sum": 3, "count": 12}))
	stats := b.Stats()
	if stats.Underruns != 4 || stats.Consumed != 884 || stats.Suppressed != 96 || stats.RMS != .5 {
		t.Fatalf("replacement lost cumulative measurements: %+v", stats)
	}
	if b.SetWorkerBaseline(want) {
		t.Fatal("baseline could be replaced after the new worklet reported audio")
	}
}
