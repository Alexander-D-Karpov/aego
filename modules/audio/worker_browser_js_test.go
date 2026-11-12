//go:build js && wasm

package audio

import (
	"encoding/json"
	"errors"
	"syscall/js"
	"testing"
	"time"

	"aego/audio/mix"
	"aego/audio/pump"
	"aego/audio/workerproto"
	"aego/platform"
)

type testBrowserOutput struct {
	node                      js.Value
	spec                      platform.AudioSpec
	ready, reserved, attached bool
	silent, gated, closed     bool
	quiet                     bool
	quietToken                uint64
	failure                   string
	counters                  platform.AudioCounters
	state                     func()
	refill                    func() int
}

func (o *testBrowserOutput) Spec() platform.AudioSpec { return o.spec }
func (o *testBrowserOutput) Write(p []float32) int    { return len(p) / 2 }
func (o *testBrowserOutput) Queued() int              { return 0 }
func (o *testBrowserOutput) Close()                   { o.closed = true }
func (o *testBrowserOutput) Pause(on bool)            { o.setSilent(on) }
func (o *testBrowserOutput) FrameDriven() bool        { return true }
func (o *testBrowserOutput) SetRefill(fn func() int)  { o.refill = fn }
func (o *testBrowserOutput) ReserveWorkerOutput() bool {
	if o.closed || o.reserved {
		return false
	}
	o.reserved = true
	return true
}
func (o *testBrowserOutput) WorkerOutput() (js.Value, js.Value, js.Value, int, int, bool) {
	if !o.reserved || o.attached || !o.ready || o.closed {
		return js.Value{}, js.Value{}, js.Value{}, 0, 0, false
	}
	return o.node, js.Undefined(), js.Undefined(), 512, 3072, true
}
func (o *testBrowserOutput) CommitWorkerOutput() bool {
	if !o.reserved || o.attached {
		return false
	}
	o.attached = true
	return true
}
func (o *testBrowserOutput) CancelWorkerOutput() bool {
	if !o.reserved || o.attached {
		return false
	}
	o.reserved, o.gated, o.state = false, false, nil
	return true
}
func (o *testBrowserOutput) WorkerSilent() bool                     { return o.silent }
func (o *testBrowserOutput) WorkerError() string                    { return o.failure }
func (o *testBrowserOutput) WorkerQuiet(token uint64) bool          { return o.quiet && o.quietToken == token }
func (o *testBrowserOutput) WorkerCounters() platform.AudioCounters { return o.counters }
func (o *testBrowserOutput) SetWorkerBaseline(c platform.AudioCounters) bool {
	if !o.reserved || o.attached || o.closed {
		return false
	}
	o.counters = c
	return true
}
func (o *testBrowserOutput) SelectWorkerOutput() bool { return o.reserved && !o.closed }
func (o *testBrowserOutput) FailWorkerOutput(message string) {
	o.failure, o.gated = message, true
}
func (o *testBrowserOutput) SetWorkerStateListener(fn func()) bool {
	if !o.reserved || o.closed {
		return false
	}
	o.state = fn
	return true
}
func (o *testBrowserOutput) SetWorkerGate(on bool) bool {
	if !o.reserved || o.closed {
		return false
	}
	o.gated = on
	return true
}
func (o *testBrowserOutput) setSilent(on bool) {
	o.silent = on
	if o.state != nil {
		o.state()
	}
}
func (o *testBrowserOutput) setQuiet(token uint64) {
	o.quiet, o.quietToken = true, token
	if o.state != nil {
		o.state()
	}
}

func testBrowserWorker(t *testing.T) (*Module, *browserWorker, *testBrowserOutput, js.Value) {
	t.Helper()
	g := js.Global()
	old := g.Get("AegoAudioWorkerHost")
	t.Cleanup(func() { g.Set("AegoAudioWorkerHost", old); g.Delete("aegoBackendHost") })
	ctor := g.Get("Function").New(`return class {
		constructor(worker,notify){this.worker=worker;this.notify=notify;this.sent=[];this.acks=[];this.id=0;globalThis.aegoBackendHost=this;}
		start(options){this.options=options;}
		send(op,input){this.sent.push({id:++this.id,op,input});}
		quiesce(){const id=++this.id;this.sent.push({id,op:'quiesce'});return id;}
		resume(){const id=++this.id;this.sent.push({id,op:'resume'});return id;}
		rebind(output){const id=++this.id;this.output=output;this.sent.push({id,op:'rebind'});return id;}
		ackBatch(serial){this.acks.push(serial);}
		emit(v){this.notify(v);}
		close(){this.closed=true;this.notify=()=>{};}
	}`).Invoke()
	g.Set("AegoAudioWorkerHost", ctor)
	spec := platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512}
	mx, err := mix.New(mix.Config{SampleRate: spec.SampleRate, Channels: 2, Voices: Voices, Block: Block, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	out := &testBrowserOutput{node: g.Get("Object").New(), spec: spec, ready: true}
	m := &Module{mixer: mx, out: out, spec: spec, manual: true}
	m.pump = pump.New(mx, out, pump.Config{SampleRate: 48000, Channels: 2, Block: 512, Target: 6})
	worker := g.Get("Function").New(`return {terminate(){this.terminated=true}}`).Invoke()
	r, err := newBrowserWorker(m, out, g.Get("Object").New(), "/worker.js", func(string) (js.Value, error) { return worker, nil })
	if err != nil || r == nil {
		t.Fatalf("worker setup: %v", err)
	}
	m.worker = r
	t.Cleanup(func() { r.Close() })
	return m, r, out, g.Get("aegoBackendHost")
}

func testBackendReply(host js.Value) {
	sent := host.Get("sent")
	last := sent.Index(sent.Length() - 1)
	host.Call("emit", map[string]any{"type": "reply", "id": last.Get("id"), "result": ""})
}

func TestBrowserWorkerQuietClockFlushAndResume(t *testing.T) {
	_, r, out, host := testBrowserWorker(t)
	clock := time.Unix(1000, 0)
	r.now = func() time.Time { return clock }
	if !out.gated || r.phase != workerStarting {
		t.Fatal("startup did not reserve and gate the output")
	}
	host.Call("emit", map[string]any{"type": "ready"})
	if !out.attached || out.gated || r.phase != workerActive {
		t.Fatal("ready output did not attach")
	}
	out.setSilent(true)
	if !out.gated || r.phase != workerQuiescing || host.Get("sent").Index(0).Get("op").String() != "quiesce" {
		t.Fatal("mute did not gate before starting the flush")
	}
	clock = clock.Add(100 * time.Millisecond)
	out.setSilent(false)
	if !out.gated || r.phase != workerQuiescing {
		t.Fatal("unmute escaped the pending flush")
	}
	testBackendReply(host)
	sent := host.Get("sent")
	if sent.Length() != 2 || sent.Index(1).Get("op").Int() != int(workerproto.Skip) {
		t.Fatal("quiet interval was not sent after the flush")
	}
	var req workerproto.Request
	if err := json.Unmarshal([]byte(sent.Index(1).Get("input").String()), &req); err != nil || req.Frames != 4800 {
		t.Fatalf("quiet interval = %d frames: %v", req.Frames, err)
	}
	testBackendReply(host)
	if r.phase != workerResuming || !out.gated || sent.Index(2).Get("op").String() != "resume" {
		t.Fatal("resume did not wait for the skip or kept the output ungated")
	}
	testBackendReply(host)
	if r.phase != workerActive || out.gated {
		t.Fatal("output did not ungate after resume acknowledgement")
	}
}

func TestBrowserWorkerMuteDuringResumeFlushesAgain(t *testing.T) {
	_, r, out, host := testBrowserWorker(t)
	clock := time.Unix(1000, 0)
	r.now = func() time.Time { return clock }
	host.Call("emit", map[string]any{"type": "ready"})
	out.setSilent(true)
	out.setSilent(false)
	testBackendReply(host) // quiesce
	testBackendReply(host) // zero skip -> resume
	if r.phase != workerResuming {
		t.Fatal("test did not reach pending resume")
	}
	out.setSilent(true)
	clock = clock.Add(10 * time.Millisecond)
	out.setSilent(false)
	if !out.gated || !r.requiesce {
		t.Fatal("second mute was not held behind resume")
	}
	testBackendReply(host) // first resume -> second quiesce
	if r.phase != workerQuiescing || host.Get("sent").Index(3).Get("op").String() != "quiesce" {
		t.Fatal("pending resume did not require another flush")
	}
	testBackendReply(host) // second quiesce -> skip
	sent := host.Get("sent")
	var req workerproto.Request
	if err := json.Unmarshal([]byte(sent.Index(4).Get("input").String()), &req); err != nil || req.Frames != 480 {
		t.Fatalf("second quiet interval = %d frames: %v", req.Frames, err)
	}
	testBackendReply(host) // skip -> second resume
	if r.phase != workerResuming || !out.gated {
		t.Fatal("second resume was not gated")
	}
	testBackendReply(host)
	if r.phase != workerActive || out.gated {
		t.Fatal("second resume did not restore output")
	}
}

func TestBrowserWorkerFallbackOnlyBeforeAttachment(t *testing.T) {
	for _, attached := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "after"}[attached], func(t *testing.T) {
			m, r, out, host := testBrowserWorker(t)
			if attached {
				host.Call("emit", map[string]any{"type": "ready"})
			}
			host.Call("emit", map[string]any{"type": "fatal", "error": "injected", "attached": attached})
			if attached {
				if m.worker != r || r.phase != workerFailed || out.failure == "" || !out.gated || out.refill != nil {
					t.Fatal("attached failure incorrectly enabled local playback")
				}
			} else if m.worker != nil || r.phase != workerClosed || out.reserved || out.refill == nil {
				t.Fatal("unattached failure did not safely restore the pump")
			}
		})
	}
}

func TestBrowserWorkerKeepsMeasuredStatsAfterFailure(t *testing.T) {
	m, r, out, host := testBrowserWorker(t)
	host.Call("emit", map[string]any{"type": "ready"})
	raw, err := json.Marshal(workerproto.BatchFromMix(mix.RemoteBatch{Serial: 1, Buses: make([]mix.RemoteBus, m.mixer.BusCount())}))
	if err != nil {
		t.Fatal(err)
	}
	host.Call("emit", map[string]any{"type": "batch", "batch": string(raw), "stats": map[string]any{
		"blocks": 10, "underruns": 1, "maxMixMS": .2, "totalMixMS": 1,
	}})
	if got := r.Stats(); got.Blocks != 10 || got.Underruns != 1 || got.MixMax != 200*time.Microsecond || got.MixAvg != 100*time.Microsecond {
		t.Fatalf("missing worker statistics: %+v", got)
	}
	host.Call("emit", map[string]any{"type": "fatal", "error": "injected", "attached": true})
	if got := r.Stats(); got.Blocks != 10 || got.Underruns != 1 || out.failure == "" {
		t.Fatalf("failure lost statistics or output error: %+v", got)
	}
}

func TestBrowserWorkerReopenWaitsForMatchingFinalCounters(t *testing.T) {
	m, r, old, host := testBrowserWorker(t)
	clock := time.Unix(1000, 0)
	r.now = func() time.Time { return clock }
	host.Call("emit", map[string]any{"type": "ready"})
	old.counters = platform.AudioCounters{Underruns: 2, Consumed: 1000, Sum: 4, Samples: 16, Suppressed: 128}
	next := &testBrowserOutput{node: js.Global().Get("Object").New(), ready: true,
		spec: platform.AudioSpec{SampleRate: 44100, Channels: 2, Frames: 512}}
	r.openOutput = func(platform.AudioSpec) platform.AudioOutput { return next }
	want := platform.AudioSpec{SampleRate: 44100, Channels: 2, Frames: 512}
	if err := r.Reopen(want); err != nil || !old.gated || r.phase != workerQuiescing {
		t.Fatalf("replacement did not gate and quiesce old output: %v", err)
	}
	if r.Reopen(want) == nil {
		t.Fatal("accepted an overlapping reopen")
	}
	testBackendReply(host) // Worker flush acknowledged; page report is still absent.
	if old.closed || r.phase != workerQuiet || host.Get("sent").Length() != 1 {
		t.Fatal("rebind overtook the final page-side worklet report")
	}
	old.setQuiet(0) // A report from an earlier flush is not sufficient.
	if old.closed || host.Get("sent").Length() != 1 {
		t.Fatal("stale quiet token authorized rebind")
	}
	old.setQuiet(1)
	if !old.closed || r.phase != workerRebinding || !next.attached || !next.gated || next.counters != old.counters ||
		!host.Get("output").Get("quiet").Bool() {
		t.Fatal("replacement did not inherit counters and start quiet")
	}
	if m.out != next || m.spec.SampleRate != 44100 || m.mixer.SampleRate() != 48000 {
		t.Fatal("local mixer sample rate changed before worker acknowledgement")
	}
	testBackendReply(host) // quiet rebind -> primed resume
	if m.mixer.SampleRate() != 44100 || r.phase != workerResuming || !next.gated {
		t.Fatal("replacement did not preserve the mixer or wait for resume")
	}
	testBackendReply(host)
	if r.phase != workerActive || next.gated || m.requested != want {
		t.Fatal("replacement was not made audible after acknowledgement")
	}
}

func TestBrowserWorkerReopenDuringMuteKeepsClockAndGate(t *testing.T) {
	m, r, old, host := testBrowserWorker(t)
	clock := time.Unix(1000, 0)
	r.now = func() time.Time { return clock }
	host.Call("emit", map[string]any{"type": "ready"})
	old.setSilent(true)
	clock = clock.Add(100 * time.Millisecond)
	r.Tick()
	testBackendReply(host) // quiesce -> first skip
	old.setQuiet(1)
	testBackendReply(host) // skip
	if r.phase != workerQuiet || !r.clock.started {
		t.Fatal("test did not reach muted output")
	}
	next := &testBrowserOutput{node: js.Global().Get("Object").New(), ready: true, silent: true,
		spec: platform.AudioSpec{SampleRate: 44100, Channels: 2, Frames: 512}}
	r.openOutput = func(platform.AudioSpec) platform.AudioOutput { return next }
	if err := r.Reopen(next.spec); err != nil {
		t.Fatal(err)
	}
	// The old-rate boundary is acknowledged before rebind.
	testBackendReply(host)
	if r.phase != workerRebinding || !next.gated {
		t.Fatal("mute did not keep the replacement gated")
	}
	clock = clock.Add(100 * time.Millisecond)
	testBackendReply(host) // quiet rebind -> new-rate skip
	sent := host.Get("sent")
	var request workerproto.Request
	if err := json.Unmarshal([]byte(sent.Index(sent.Length()-1).Get("input").String()), &request); err != nil || request.Frames != 4410 {
		t.Fatalf("new-rate quiet interval = %d frames: %v", request.Frames, err)
	}
	testBackendReply(host)
	if r.phase != workerQuiet || !next.gated || m.mixer.SampleRate() != 44100 {
		t.Fatal("muted replacement did not remain quiet at the new rate")
	}
	clock = clock.Add(100 * time.Millisecond)
	next.setSilent(false)
	testBackendReply(host) // final skip -> resume
	testBackendReply(host) // resume
	if r.phase != workerActive || next.gated {
		t.Fatal("unmute did not wait for the final skip and resume")
	}
}

func TestBrowserWorkerCreationFailureReturnsLocalOwnership(t *testing.T) {
	g := js.Global()
	mx, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4})
	out := &testBrowserOutput{node: g.Get("Object").New(), spec: platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512}, ready: true}
	m := &Module{mixer: mx, out: out, spec: out.spec, manual: true}
	r, err := newBrowserWorker(m, out, g.Get("Object").New(), "/worker.js", func(string) (js.Value, error) {
		return js.Value{}, errors.New("injected worker creation failure")
	})
	if err != nil || r != nil || out.reserved || out.gated {
		t.Fatalf("creation failure did not safely return ownership: %v", err)
	}
}

func TestBrowserWorkerSelectionRequiresSameOriginPreparedConfig(t *testing.T) {
	// Install the fake host without reusing its already-claimed mixer.
	_, previous, _, _ := testBrowserWorker(t)
	previous.Close()
	g := js.Global()
	oldWorker, oldConfig, oldLocation := g.Get("Worker"), g.Get("aegoAudioWorkerConfig"), g.Get("location")
	t.Cleanup(func() {
		g.Set("Worker", oldWorker)
		g.Set("aegoAudioWorkerConfig", oldConfig)
		g.Set("location", oldLocation)
	})
	g.Set("Worker", g.Get("Function").New(`return class {terminate(){this.terminated=true}}`).Invoke())
	g.Set("location", map[string]any{"href": "https://example.test/demo/index.html", "origin": "https://example.test"})
	makeModule := func() (*Module, *testBrowserOutput) {
		mx, err := mix.New(mix.Config{SampleRate: 48000, Voices: Voices, Buses: BusDescs()})
		if err != nil {
			t.Fatal(err)
		}
		out := &testBrowserOutput{node: g.Get("Object").New(), spec: platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512}, ready: true}
		m := &Module{mixer: mx, out: out, spec: out.spec, manual: true}
		m.pump = pump.New(mx, out, pump.Config{SampleRate: 48000, Channels: 2, Block: 512, Target: 6})
		return m, out
	}
	for _, url := range []string{"https://foreign.test/worker.js", "https://user@example.test/worker.js", "file:///tmp/worker.js"} {
		m, out := makeModule()
		g.Set("aegoAudioWorkerConfig", map[string]any{"module": g.Get("Object").New(), "url": url})
		if selected, err := m.newWorkerBackend(); err != nil || selected != nil || out.reserved {
			t.Fatalf("selected unsafe Worker URL %q: %v", url, err)
		}
	}
	m, out := makeModule()
	g.Set("aegoAudioWorkerConfig", map[string]any{"module": g.Get("Object").New(), "url": "/demo/audio-worker.js?v=abc"})
	selected, err := m.newWorkerBackend()
	if err != nil || selected == nil || !out.reserved || !out.gated {
		t.Fatalf("prepared same-origin Worker was not selected: %v", err)
	}
	selected.Close()
}
