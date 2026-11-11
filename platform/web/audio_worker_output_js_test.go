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
