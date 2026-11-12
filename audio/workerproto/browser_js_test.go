//go:build js && wasm

package workerproto

import (
	"encoding/json"
	"math"
	"syscall/js"
	"testing"
	"time"

	"aego/audio/mix"
)

func browserFixture(t *testing.T, m *mix.Mixer, opts BrowserOptions) (*Browser, js.Value) {
	t.Helper()
	g := js.Global()
	old := g.Get("AegoAudioWorkerHost")
	t.Cleanup(func() { g.Set("AegoAudioWorkerHost", old); g.Delete("aegoTestWorkerHost") })
	ctor := g.Get("Function").New(`return class {
		constructor(worker, notify) { this.worker=worker;this.notify=notify;this.sent=[];this.acks=[];this.id=0;globalThis.aegoTestWorkerHost=this; }
		start(options) { this.options=options; }
		send(op,input,pcm,offset) { const id=++this.id;this.sent.push({id,op,input,pcm,offset});return id; }
		quiesce() { const id=++this.id;this.sent.push({id,op:'quiesce'});return id; }
		resume() { const id=++this.id;this.sent.push({id,op:'resume'});return id; }
		rebind(output) { this.output=output;return ++this.id; }
		ackBatch(serial) { this.acks.push(serial); }
		emit(value) { this.notify(value); }
		close() { this.closed=true;this.notify=()=>{}; }
	}`).Invoke()
	g.Set("AegoAudioWorkerHost", ctor)
	opts.Worker, opts.Module, opts.Node = g.Get("Object").New(), g.Get("Object").New(), g.Get("Object").New()
	opts.Block, opts.Target = 512, 3072
	opts.Init = Request{Rate: 48000, Frames: 512, Voices: 4, Budget: 1 << 20}
	b, err := NewBrowser(m, opts)
	if err != nil {
		t.Fatal(err)
	}
	return b, g.Get("aegoTestWorkerHost")
}

func browserReply(host js.Value) {
	sent := host.Get("sent")
	last := sent.Index(sent.Length() - 1)
	host.Call("emit", map[string]any{"type": "reply", "id": last.Get("id"), "result": ""})
}

func browserBatch(t *testing.T, host js.Value, batch mix.RemoteBatch) {
	t.Helper()
	raw, err := json.Marshal(BatchFromMix(batch))
	if err != nil {
		t.Fatal(err)
	}
	host.Call("emit", map[string]any{"type": "batch", "batch": string(raw)})
}

func TestBrowserControllerTransfersAndReleasesMemory(t *testing.T) {
	m, err := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	if err != nil {
		t.Fatal(err)
	}
	voice := m.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{.25, .5}, Rate: 48000, Channels: 1}, Gain: 1})
	ready, batches := 0, 0
	b, host := browserFixture(t, m, BrowserOptions{OnReady: func() { ready++ }, OnBatch: func() { batches++ }})
	defer b.Terminate()
	if host.Get("sent").Length() != 0 || ready != 0 {
		t.Fatal("transferred commands before the host became ready")
	}
	host.Call("emit", map[string]any{"type": "ready"})
	if ready != 1 {
		t.Fatal("ready callback was not delivered")
	}
	for i, want := range []any{int(BeginMemory), "memory-chunk", int(CommitMemory), int(Command)} {
		sent := host.Get("sent")
		if sent.Length() != i+1 {
			t.Fatalf("expected one outstanding step at %d, got %d", i, sent.Length())
		}
		step := sent.Index(i)
		if step.Get("op").String() != js.ValueOf(want).String() {
			t.Fatalf("operation %d = %s, want %v", i, step.Get("op").String(), want)
		}
		if i == 1 {
			pcm := step.Get("pcm")
			if pcm.Length() != 2 || pcm.Index(0).Float() != .25 || pcm.Index(1).Float() != .5 {
				t.Fatal("browser transfer changed the source PCM")
			}
		}
		browserReply(host)
	}
	active := mix.RemoteBatch{Serial: 1, Applied: 1, Active: 1, Buses: make([]mix.RemoteBus, m.BusCount()),
		Voices: []mix.RemoteVoice{{ID: voice, Position: 1, Gain: .5}}}
	browserBatch(t, host, active)
	if batches != 1 || !m.Live(voice) || m.Position(voice) != 1 || host.Get("acks").Index(0).String() != "1" {
		t.Fatal("the worker batch was not applied before acknowledgement")
	}
	browserBatch(t, host, mix.RemoteBatch{Serial: 2, Applied: 1, Buses: make([]mix.RemoteBus, m.BusCount()), Released: []mix.VoiceID{voice}})
	if m.Live(voice) || batches != 2 || host.Get("acks").Index(1).String() != "2" {
		t.Fatal("released voice was not applied or acknowledged")
	}
	sent := host.Get("sent")
	if sent.Length() != 5 || sent.Index(4).Get("op").Int() != int(Drop) {
		t.Fatal("released source was not dropped")
	}
	browserReply(host)
	if len(b.controller.sources) != 0 || b.err != nil {
		t.Fatal("source lifetime did not finish", b.err)
	}
}

func TestBrowserStartupFallbackAndAttachedFailure(t *testing.T) {
	for _, attached := range []bool{false, true} {
		m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
		m.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1, 1, 1}, Rate: 48000, Channels: 1}, Gain: 1})
		var failures int
		var gotAttached bool
		b, host := browserFixture(t, m, BrowserOptions{OnFailure: func(_ error, v bool) { failures++; gotAttached = v }})
		host.Call("emit", map[string]any{"type": "fatal", "error": "injected", "attached": attached})
		if failures != 1 || gotAttached != attached || b.err == nil || !host.Get("closed").Bool() {
			t.Fatal("failure did not stop and report the transport")
		}
		b.Terminate()
		if err := b.CancelUnsent(); (err != nil) != attached {
			t.Fatalf("unsafe fallback decision for attached=%v: %v", attached, err)
		}
		if attached {
			if err := b.AbortAfterTermination(); err != nil {
				t.Fatal(err)
			}
		} else {
			out := make([]float32, 4)
			m.Mix(out)
			if out[0] == 0 && out[2] == 0 {
				t.Fatal("fallback lost the unsent voice")
			}
		}
	}
}

func TestBrowserFailureCannotForgetPriorOutputAttachment(t *testing.T) {
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	var reportedAttached bool
	b, host := browserFixture(t, m, BrowserOptions{OnFailure: func(_ error, attached bool) { reportedAttached = attached }})
	host.Call("emit", map[string]any{"type": "ready"})
	// A malformed or buggy host must not undo known ownership just because no
	// commands have been sent. The output was already attached at readiness.
	host.Call("emit", map[string]any{"type": "fatal", "error": "inconsistent state", "attached": false})
	if !reportedAttached || !b.attached {
		t.Fatal("failure forgot the earlier output attachment")
	}
	b.Terminate()
	if err := b.CancelUnsent(); err == nil {
		t.Fatal("allowed local fallback after an output was attached")
	}
	if err := b.AbortAfterTermination(); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserSynchronousStartupFailureRetainsOwnership(t *testing.T) {
	g := js.Global()
	old := g.Get("AegoAudioWorkerHost")
	t.Cleanup(func() { g.Set("AegoAudioWorkerHost", old); g.Delete("aegoTestWorkerHost") })
	for _, mode := range []string{"fatal", "throw"} {
		ctor := g.Get("Function").New("mode", `return class {
			constructor(worker, notify) { this.notify=notify;globalThis.aegoTestWorkerHost=this; }
			start() { if (mode==='throw') throw Error('start failed'); this.notify({type:'fatal',error:'start failed',attached:false}); }
			close() { this.closed=true; }
		}`).Invoke(mode)
		g.Set("AegoAudioWorkerHost", ctor)
		m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
		m.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1, 1}, Rate: 48000, Channels: 1}, Gain: 1})
		var failures int
		b, err := NewBrowser(m, BrowserOptions{Worker: g.Get("Object").New(), Module: g.Get("Object").New(),
			Node: g.Get("Object").New(), Block: 512, Target: 3072,
			Init: Request{Rate: 48000, Frames: 512, Voices: 4, Budget: 1 << 20},
			OnFailure: func(_ error, attached bool) {
				if attached {
					t.Error("unattached startup reported an attached output")
				}
				failures++
			}})
		if b == nil || err == nil || failures != 1 || !g.Get("aegoTestWorkerHost").Get("closed").Bool() {
			t.Fatalf("%s: startup ownership was lost: browser=%v err=%v failures=%d", mode, b != nil, err, failures)
		}
		b.Terminate()
		if err := b.CancelUnsent(); err != nil {
			t.Fatalf("%s: unsent local ownership not restored: %v", mode, err)
		}
		out := make([]float32, 4)
		m.Mix(out)
		if out[0] == 0 && out[2] == 0 {
			t.Fatalf("%s: queued local play was lost", mode)
		}
	}
}

func TestBrowserRejectsMalformedBatchBeforeAcknowledgement(t *testing.T) {
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	var failures int
	b, host := browserFixture(t, m, BrowserOptions{OnFailure: func(error, bool) { failures++ }})
	host.Call("emit", map[string]any{"type": "ready"})
	host.Call("emit", map[string]any{"type": "batch", "batch": `{"serial":"1","unknown":true}`})
	if failures != 1 || b.err == nil || host.Get("acks").Length() != 0 {
		t.Fatal("malformed batch was accepted or acknowledged")
	}
	b.Terminate()
	if err := b.AbortAfterTermination(); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserRequiresValidatedMonotonicWorkerStats(t *testing.T) {
	for _, tt := range []struct {
		name  string
		stats any
	}{
		{"missing", nil},
		{"negative", map[string]any{"blocks": -1, "underruns": 0, "maxMixMS": 0, "totalMixMS": 0}},
		{"noninteger", map[string]any{"blocks": .5, "underruns": 0, "maxMixMS": 0, "totalMixMS": 0}},
		{"nonfinite", map[string]any{"blocks": 1, "underruns": 0, "maxMixMS": math.Inf(1), "totalMixMS": 1}},
		{"inconsistent time", map[string]any{"blocks": 1, "underruns": 0, "maxMixMS": 2, "totalMixMS": 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
			b, host := browserFixture(t, m, BrowserOptions{RequireStats: true})
			defer b.Terminate()
			host.Call("emit", map[string]any{"type": "ready"})
			raw, _ := json.Marshal(BatchFromMix(mix.RemoteBatch{Serial: 1, Buses: make([]mix.RemoteBus, m.BusCount())}))
			host.Call("emit", map[string]any{"type": "batch", "batch": string(raw), "stats": tt.stats})
			if b.err == nil || host.Get("acks").Length() != 0 {
				t.Fatal("accepted or acknowledged a batch with invalid statistics")
			}
		})
	}
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	b, host := browserFixture(t, m, BrowserOptions{RequireStats: true})
	defer b.Terminate()
	host.Call("emit", map[string]any{"type": "ready"})
	raw, _ := json.Marshal(BatchFromMix(mix.RemoteBatch{Serial: 1, Buses: make([]mix.RemoteBus, m.BusCount())}))
	host.Call("emit", map[string]any{"type": "batch", "batch": string(raw), "stats": map[string]any{
		"blocks": 12, "underruns": 1, "maxMixMS": .25, "totalMixMS": .5,
	}})
	stats, ok := b.Stats()
	if !ok || stats.Blocks != 12 || stats.Underruns != 1 || stats.MixMax != 250*time.Microsecond || stats.MixTotal != 500*time.Microsecond || host.Get("acks").Length() != 1 {
		t.Fatalf("worker stats were not retained: %+v available=%v", stats, ok)
	}
	raw, _ = json.Marshal(BatchFromMix(mix.RemoteBatch{Serial: 2, Buses: make([]mix.RemoteBus, m.BusCount())}))
	host.Call("emit", map[string]any{"type": "batch", "batch": string(raw), "stats": map[string]any{
		"blocks": 13, "underruns": 0, "maxMixMS": .25, "totalMixMS": .5,
	}})
	if b.err == nil || host.Get("acks").Length() != 1 {
		t.Fatal("accepted a backwards underrun count")
	}
}

func TestBrowserSendFailureStopsAfterAttachment(t *testing.T) {
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	voice := m.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1}, Rate: 48000, Channels: 1}, Gain: 1})
	var failures int
	var attached bool
	b, host := browserFixture(t, m, BrowserOptions{OnFailure: func(_ error, value bool) { failures++; attached = value }})
	host.Set("send", js.Global().Get("Function").New(`throw Error('injected send failure')`))
	host.Call("emit", map[string]any{"type": "ready"})
	if failures != 1 || !attached || b.err == nil || !host.Get("closed").Bool() || !m.Live(voice) {
		t.Fatal("send failure did not stop the attached transport safely")
	}
	b.Terminate()
	if err := b.CancelUnsent(); err == nil {
		t.Fatal("send failure incorrectly allowed local replay")
	}
	if err := b.AbortAfterTermination(); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserSkipIsOrderedAndBounded(t *testing.T) {
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	var quiesced, resumed int
	b, host := browserFixture(t, m, BrowserOptions{OnQuiesced: func() { quiesced++ }, OnResume: func() { resumed++ }})
	defer b.Terminate()
	if err := b.QueueSkip(1); err == nil {
		t.Fatal("skip was accepted before startup")
	}
	host.Call("emit", map[string]any{"type": "ready"})
	if err := b.QueueSkip(1); err == nil {
		t.Fatal("skip was accepted while output could still play old PCM")
	}
	if err := b.Quiesce(); err != nil || !b.quiescing {
		t.Fatalf("could not quiesce output: %v", err)
	}
	if err := b.QueueSkip(0); err != nil || host.Get("sent").Length() != 1 {
		t.Fatal("skip was not held until quiesce acknowledgement", err)
	}
	browserReply(host)
	if quiesced != 1 || !b.quiesced {
		t.Fatal("quiesce was not acknowledged")
	}
	if err := b.QueueSkip(int64(maxSkipFrames + 2)); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{0, maxSkipFrames, 2} {
		sent := host.Get("sent")
		if sent.Length() != i+2 || sent.Index(i+1).Get("op").Int() != int(Skip) {
			t.Fatalf("skip %d was not serialized", i)
		}
		var request Request
		if err := json.Unmarshal([]byte(sent.Index(i+1).Get("input").String()), &request); err != nil || request.Frames != want {
			t.Fatalf("skip %d frames=%d want=%d error=%v", i, request.Frames, want, err)
		}
		browserReply(host)
	}
	if err := b.QueueSkip(-1); err == nil || b.err != nil {
		t.Fatal("invalid skip was not rejected without damaging the transport")
	}
	if err := b.Resume(); err != nil || !b.resuming {
		t.Fatalf("resume did not start: %v", err)
	}
	if err := b.QueueSkip(1); err == nil {
		t.Fatal("accepted a skip while the output was resuming")
	}
	browserReply(host)
	if resumed != 1 || b.quiesced || b.resuming || b.err != nil {
		t.Fatal("resume did not wait for worklet acknowledgement")
	}
}

func TestBrowserQuiesceWaitsForRPCAndBlocksQueuedSkipOnRebind(t *testing.T) {
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	m.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1}, Rate: 48000, Channels: 1}, Gain: 1})
	var quiesced int
	b, host := browserFixture(t, m, BrowserOptions{OnQuiesced: func() { quiesced++ }})
	defer b.Terminate()
	host.Call("emit", map[string]any{"type": "ready"})
	if err := b.Quiesce(); err != nil || !b.quiesceReq || b.quiescing {
		t.Fatalf("quiesce did not wait for pending source transfer: %v", err)
	}
	if host.Get("sent").Length() != 1 {
		t.Fatal("quiesce overtook the in-flight request")
	}
	browserReply(host)
	if sent := host.Get("sent"); sent.Length() != 2 || sent.Index(1).Get("op").String() != "quiesce" {
		t.Fatal("source transfer continued before quiescence")
	}
	browserReply(host)
	if quiesced != 1 || !b.quiesced || host.Get("sent").Length() != 3 {
		t.Fatal("quiescence did not resume the source transfer")
	}
	if err := b.QueueSkip(10); err != nil {
		t.Fatal(err)
	}
	output := BrowserOutput{Node: js.Global().Get("Object").New(), Block: 512, Target: 3072, Rate: 48000}
	for i := 0; i < 3; i++ {
		if err := b.Rebind(output); err == nil {
			t.Fatal("rebind overtook source transfer or queued skip")
		}
		browserReply(host)
	}
	if sent := host.Get("sent"); sent.Length() != 6 || sent.Index(5).Get("op").Int() != int(Skip) {
		t.Fatal("skip did not follow the preceding play")
	}
	if err := b.Rebind(output); err == nil {
		t.Fatal("rebind overtook an in-flight skip")
	}
	browserReply(host)
	if err := b.Rebind(output); err != nil {
		t.Fatalf("rebind did not follow the acknowledged skip: %v", err)
	}
	host.Call("emit", map[string]any{"type": "reply", "id": 7, "result": ""})
	if b.quiesced || b.rebinding || b.err != nil {
		t.Fatal("replacement output was not attached")
	}
}

func TestBrowserRequiescesAfterPendingResumeBeforeNextSkip(t *testing.T) {
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	b, host := browserFixture(t, m, BrowserOptions{})
	defer b.Terminate()
	host.Call("emit", map[string]any{"type": "ready"})
	if err := b.Quiesce(); err != nil {
		t.Fatal(err)
	}
	browserReply(host)
	if err := b.Resume(); err != nil {
		t.Fatal(err)
	}
	if err := b.QueueSkip(10); err == nil {
		t.Fatal("accepted a skip during resume without a new quiesce")
	}
	if err := b.Quiesce(); err != nil || !b.quiesceReq {
		t.Fatal("could not request a subsequent quiesce", err)
	}
	if err := b.QueueSkip(10); err != nil || host.Get("sent").Length() != 2 {
		t.Fatal("skip overtook the pending resume", err)
	}
	browserReply(host)
	if sent := host.Get("sent"); sent.Length() != 3 || sent.Index(2).Get("op").String() != "quiesce" {
		t.Fatal("second quiesce did not follow resume")
	}
	browserReply(host)
	if sent := host.Get("sent"); sent.Length() != 4 || sent.Index(3).Get("op").Int() != int(Skip) {
		t.Fatal("skip was not held until the new flush completed")
	}
	browserReply(host)
	if !b.quiesced || !b.Idle() {
		t.Fatal("transport did not become quietly idle")
	}
}

func TestBrowserIdleCallbackAfterAcknowledgedWorkDoesNotRecurse(t *testing.T) {
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	var b *Browser
	var idle int
	b, host := browserFixture(t, m, BrowserOptions{OnIdle: func() {
		idle++
		if !b.Idle() {
			t.Error("idle callback ran before acknowledgement")
		}
		if err := b.Pump(); err != nil {
			t.Error(err)
		}
	}})
	defer b.Terminate()
	if b.Idle() {
		t.Fatal("transport reported idle before readiness")
	}
	host.Call("emit", map[string]any{"type": "ready"})
	if idle != 1 || !b.Idle() {
		t.Fatal("ready transport did not notify idle exactly once")
	}
	if !m.SetBusGain(0, .5) || b.Idle() {
		t.Fatal("published command did not make the controller busy")
	}
	if err := b.Pump(); err != nil || b.Idle() {
		t.Fatal("in-flight command was reported idle", err)
	}
	browserReply(host)
	if idle != 2 || !b.Idle() {
		t.Fatal("command acknowledgement did not notify idle")
	}
}

func TestBrowserRebindSerializesWithWorkerOperations(t *testing.T) {
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	voice := m.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1}, Rate: 48000, Channels: 1}, Gain: 1})
	var rebound int
	b, host := browserFixture(t, m, BrowserOptions{OnRebind: func() { rebound++ }})
	defer b.Terminate()
	host.Call("emit", map[string]any{"type": "ready"})
	next := js.Global().Get("Object").New()
	output := BrowserOutput{Node: next, Block: 512, Target: 3072, Rate: 44100}
	if err := b.Rebind(output); err == nil {
		t.Fatal("rebind overtook a pending source operation")
	}
	for i := 0; i < 4; i++ {
		browserReply(host)
	}
	if err := b.Rebind(output); err != nil || !b.rebinding {
		t.Fatalf("rebind did not start: %v", err)
	}
	if !host.Get("output").Get("node").Equal(next) || host.Get("output").Get("rate").Int() != 44100 {
		t.Fatal("rebind lost the requested output or sample rate")
	}
	if err := b.Rebind(output); err == nil {
		t.Fatal("started a second rebind while one was pending")
	}
	m.SetGain(voice, .5, 0)
	if err := b.Pump(); err != nil || host.Get("sent").Length() != 4 {
		t.Fatal("command overtook the output rebind", err)
	}
	host.Call("emit", map[string]any{"type": "reply", "id": 5, "result": ""})
	if b.rebinding || rebound != 1 || b.err != nil {
		t.Fatalf("rebind reply was not completed: %v", b.err)
	}
	if sent := host.Get("sent"); sent.Length() != 5 || sent.Index(4).Get("op").Int() != int(Command) {
		t.Fatal("queued command did not resume after rebind")
	}
}

func TestBrowserQuietRebindAllowsSkipBeforeResume(t *testing.T) {
	m, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	var rebound int
	b, host := browserFixture(t, m, BrowserOptions{OnRebind: func() { rebound++ }})
	defer b.Terminate()
	host.Call("emit", map[string]any{"type": "ready"})
	output := BrowserOutput{Node: js.Global().Get("Object").New(), Block: 512, Target: 3072, Rate: 44100, Quiet: true}
	if err := b.Rebind(output); err != nil || !host.Get("output").Get("quiet").Bool() {
		t.Fatalf("quiet rebind did not start: %v", err)
	}
	host.Call("emit", map[string]any{"type": "reply", "id": 1, "result": ""})
	if rebound != 1 || !b.quiesced || b.rebinding {
		t.Fatal("replacement did not remain quiet")
	}
	if err := b.QueueSkip(441); err != nil {
		t.Fatal(err)
	}
	if err := b.Resume(); err == nil {
		t.Fatal("resume overtook the quiet skip")
	}
	browserReply(host)
	if err := b.Resume(); err != nil {
		t.Fatal(err)
	}
	browserReply(host)
	if b.quiesced || b.resuming || b.err != nil {
		t.Fatal("replacement did not resume")
	}
}
