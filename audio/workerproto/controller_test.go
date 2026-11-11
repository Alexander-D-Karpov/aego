package workerproto

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

	"aego/audio/mix"
)

// The real worker-side source and mixer implementation, without a browser,
// output device, timers or mocked mixer state.
type controllerFixture struct {
	t      *testing.T
	mixer  *mix.Mixer
	worker *mix.RemoteSources
	client *Controller
	staged []float32
	steps  []ControllerStep
}

func newControllerFixture(t *testing.T) *controllerFixture {
	t.Helper()
	cfg := mix.Config{SampleRate: 48000, Voices: 4, Block: 64}
	m, err := mix.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	w, err := mix.NewRemoteSources(cfg, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewController(m)
	if err != nil {
		t.Fatal(err)
	}
	return &controllerFixture{t: t, mixer: m, worker: w, client: c}
}

func (f *controllerFixture) invoke(step ControllerStep) []byte {
	f.t.Helper()
	f.steps = append(f.steps, step)
	r := step.Request
	var result []byte
	var err error
	if step.Chunk {
		if step.Offset < 0 || step.Offset+len(step.PCM) > len(f.staged) {
			f.t.Fatal("memory chunk exceeds worker staging buffer")
		}
		copy(f.staged[step.Offset:], step.PCM)
		return nil
	}
	switch step.Op {
	case BeginMemory:
		f.staged, err = f.worker.BeginMemory(r.Source, mix.Memory{Rate: r.Rate, Channels: r.Channels,
			LoopStart: r.LoopStart, LoopEnd: r.LoopEnd, Loop: r.Loop}, r.Samples)
	case CommitMemory:
		err = f.worker.CommitMemory(r.Source)
		f.staged = nil
	case Stream:
		err = f.worker.RegisterStream(r.Source, r.Rate, r.Channels, r.Frames)
	case StreamStatus:
		var status mix.RemoteStreamStatus
		status, err = f.worker.StreamStatus(r.Source)
		if err == nil {
			result, err = json.Marshal(status)
		}
	case StreamWrite:
		var written int
		written, err = f.worker.WriteStream(r.Source, step.PCM)
		if err == nil {
			result, err = json.Marshal(struct {
				Frames int `json:"frames"`
			}{written})
		}
	case TryStreamEnd:
		var ended bool
		ended, err = f.worker.TryEndStream(r.Source)
		if err == nil {
			result, err = json.Marshal(struct {
				Ended bool `json:"ended"`
			}{ended})
		}
	case StreamControlAck:
		err = f.worker.AckStreamControl(r.Source, r.Control)
	case Command:
		err = f.worker.Accept(r.Command.Mix())
	case Skip:
		err = f.worker.Skip(r.Frames)
	case Drop:
		err = f.worker.Drop(r.Source)
	default:
		f.t.Fatalf("unexpected operation: %+v", step)
	}
	if err != nil {
		f.t.Fatalf("worker operation %+v: %v", step, err)
	}
	return result
}

func TestControllerSkipMatchesLocalMixerAndBoundsCalls(t *testing.T) {
	f := newControllerFixture(t)
	reference, err := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 64})
	if err != nil {
		t.Fatal(err)
	}
	source := &mix.Memory{PCM: make([]float32, 4800), Rate: 48000, Channels: 1, Loop: true}
	voice := f.mixer.Play(mix.PlayParams{Memory: source, Gain: 1, Loop: true})
	refVoice := reference.Play(mix.PlayParams{Memory: source, Gain: 1, Loop: true})
	// The zero skip must drain a queued Play before any output is rendered.
	if err := f.client.QueueSkip(0); err != nil {
		t.Fatal(err)
	}
	f.drive()
	reference.Skip(0)
	batch, err := f.worker.Drain()
	if err != nil || f.client.ApplyBatch(batch) != nil || f.mixer.Active() != reference.Active() {
		t.Fatalf("zero skip did not apply the pending play: %v", err)
	}
	const frames = 2*maxSkipFrames + 37
	start := len(f.steps)
	if err := f.client.QueueSkip(frames); err != nil {
		t.Fatal(err)
	}
	f.drive()
	reference.Skip(frames)
	batch, err = f.worker.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.client.ApplyBatch(batch); err != nil {
		t.Fatal(err)
	}
	var chunks []int
	for _, step := range f.steps[start:] {
		if step.Op == Skip {
			chunks = append(chunks, step.Request.Frames)
		}
	}
	if !reflect.DeepEqual(chunks, []int{maxSkipFrames, maxSkipFrames, 37}) {
		t.Fatalf("skip was not bounded: %v", chunks)
	}
	if !f.mixer.Live(voice) || math.Abs(float64(f.mixer.Position(voice)-reference.Position(refVoice))) > 1e-5 {
		t.Fatalf("remote skip diverged: remote %v local %v", f.mixer.Position(voice), reference.Position(refVoice))
	}
}

func TestControllerSkipReleasesFinishedVoice(t *testing.T) {
	f := newControllerFixture(t)
	voice := f.mixer.Play(mix.PlayParams{Memory: &mix.Memory{PCM: make([]float32, 8), Rate: 48000, Channels: 1}, Gain: 1})
	if err := f.client.QueueSkip(16); err != nil {
		t.Fatal(err)
	}
	f.drive()
	batch, err := f.worker.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.client.ApplyBatch(batch); err != nil {
		t.Fatal(err)
	}
	if f.mixer.Live(voice) || !reflect.DeepEqual(batch.Released, []mix.VoiceID{voice}) || len(batch.Events) != 1 || batch.Events[0].Kind != mix.EvFinished {
		t.Fatalf("skip did not publish the finished voice: %+v", batch)
	}
	f.drive()
	if len(f.client.sources) != 0 {
		t.Fatal("skip retained a released source")
	}
}

func TestControllerSkipReconcilesStreamHoldAndFrames(t *testing.T) {
	f := newControllerFixture(t)
	reference, _ := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 64})
	source, refSource := mix.NewStream(48000, 1, 64), mix.NewStream(48000, 1, 64)
	pcm := []float32{.1, .2, .3, .4, .5, .6, .7, .8}
	source.Ring.Write(pcm)
	refSource.Ring.Write(pcm)
	voice := f.mixer.Play(mix.PlayParams{Stream: source, Gain: 1})
	refVoice := reference.Play(mix.PlayParams{Stream: refSource, Gain: 1})
	if err := f.client.QueueSkip(100); err != nil {
		t.Fatal(err)
	}
	f.drive()
	reference.Skip(100)
	batch, err := f.worker.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.client.ApplyBatch(batch); err != nil {
		t.Fatal(err)
	}
	if err := f.client.PollStreams(); err != nil {
		t.Fatal(err)
	}
	f.drive()
	if !source.Held() || source.Held() != refSource.Held() || source.Ring.Len() != refSource.Ring.Len() ||
		source.TakeSkip() != refSource.TakeSkip() || f.mixer.Position(voice) != reference.Position(refVoice) {
		t.Fatal("remote skip did not reconcile stream hold, consumption and position")
	}
}

func TestControllerSkipPreservesCommandTimeBoundaries(t *testing.T) {
	f := newControllerFixture(t)
	reference, err := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 64})
	if err != nil {
		t.Fatal(err)
	}
	source := &mix.Memory{PCM: make([]float32, 256), Rate: 48000, Channels: 1}
	if err := f.client.QueueSkip(10); err != nil {
		t.Fatal(err)
	}
	voice := f.mixer.Play(mix.PlayParams{Memory: source, Gain: 1})
	if !voice.Valid() || f.client.QueueSkip(20) != nil || !f.mixer.Stop(voice, 0) || f.client.QueueSkip(30) != nil {
		t.Fatal("could not prepare the ordered timeline")
	}
	f.drive()
	var timeline []string
	for _, step := range f.steps {
		switch step.Op {
		case Skip:
			timeline = append(timeline, fmt.Sprintf("skip:%d", step.Request.Frames))
		case Command:
			timeline = append(timeline, fmt.Sprintf("command:%d", step.Request.Command.Mix().Command.Kind))
		}
	}
	want := []string{"skip:10", fmt.Sprintf("command:%d", mix.CmdPlay), "skip:20", fmt.Sprintf("command:%d", mix.CmdStop), "skip:30"}
	if !reflect.DeepEqual(timeline, want) {
		t.Fatalf("commands crossed skip boundaries: got %v, want %v", timeline, want)
	}
	reference.Skip(10)
	refVoice := reference.Play(mix.PlayParams{Memory: source, Gain: 1})
	reference.Skip(20)
	reference.Stop(refVoice, 0)
	reference.Skip(30)
	batch, err := f.worker.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.client.ApplyBatch(batch); err != nil {
		t.Fatal(err)
	}
	if f.mixer.Live(voice) != reference.Live(refVoice) || f.mixer.Position(voice) != reference.Position(refVoice) ||
		f.mixer.VoiceGain(voice) != reference.VoiceGain(refVoice) {
		t.Fatalf("ordered timeline diverged from the local mixer: remote %+v, local position=%v gain=%v", batch,
			reference.Position(refVoice), reference.VoiceGain(refVoice))
	}
}

func TestControllerSkipWaitsForAlreadyTransferredCommand(t *testing.T) {
	f := newControllerFixture(t)
	voice := f.mixer.Play(mix.PlayParams{Memory: &mix.Memory{PCM: make([]float32, 256), Rate: 48000, Channels: 1}, Gain: 1})
	step, ok, err := f.client.Next()
	if err != nil || !ok || step.Op != BeginMemory {
		t.Fatalf("expected an in-flight source registration: %+v %v", step, err)
	}
	if err := f.client.QueueSkip(20); err != nil || !f.mixer.Stop(voice, 0) {
		t.Fatalf("could not schedule skip and stop: %v", err)
	}
	if err := f.client.Ack(step.Token, f.invoke(step)); err != nil {
		t.Fatal(err)
	}
	f.drive()
	var timeline []int32
	for _, step := range f.steps {
		if step.Op == Command || step.Op == Skip {
			timeline = append(timeline, step.Op)
		}
	}
	if !reflect.DeepEqual(timeline, []int32{Command, Skip, Command}) {
		t.Fatalf("in-flight command or later stop crossed skip: %v", timeline)
	}
}

func TestControllerSkipLimitsBoundariesAndPreservesRejectedWork(t *testing.T) {
	f := newControllerFixture(t)
	for i := 0; i < maxSkipBoundaries; i++ {
		if !f.mixer.SetBusGain(0, .5) || f.client.QueueSkip(1) != nil {
			t.Fatalf("could not schedule boundary %d", i)
		}
	}
	if !f.mixer.SetBusGain(0, 1) {
		t.Fatal("could not queue the extra command")
	}
	if err := f.client.QueueSkip(1); err == nil {
		t.Fatal("accepted an unbounded number of time boundaries")
	}
	f.drive()
	var skips int
	for _, step := range f.steps {
		if step.Op == Skip {
			skips++
		}
	}
	if skips != maxSkipBoundaries {
		t.Fatalf("lost previously accepted skips: %d", skips)
	}
	if err := f.client.QueueSkip(1); err != nil {
		t.Fatalf("could not retry the rejected skip after draining: %v", err)
	}
	f.drive()
}

func TestControllerSkipCannotFallbackAfterAcknowledgement(t *testing.T) {
	f := newControllerFixture(t)
	if err := f.client.QueueSkip(0); err != nil {
		t.Fatal(err)
	}
	f.drive()
	if err := f.client.CancelUnsent(); err == nil {
		t.Fatal("allowed local fallback after sending a skip")
	}
}

func TestControllerSkipRejectsInvalidAndOverflow(t *testing.T) {
	f := newControllerFixture(t)
	if err := f.client.QueueSkip(-1); err == nil {
		t.Fatal("accepted a negative skip")
	}
	if err := f.client.QueueSkip(math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if err := f.client.QueueSkip(1); err == nil {
		t.Fatal("accepted an overflowing skip queue")
	}
	if err := f.client.CancelUnsent(); err == nil {
		t.Fatal("allowed fallback while skip work was queued")
	}
}

func (f *controllerFixture) drive() {
	f.t.Helper()
	for i := 0; i < 1000; i++ {
		step, ok, err := f.client.Next()
		if err != nil {
			f.t.Fatal(err)
		}
		if !ok {
			return
		}
		result := f.invoke(step)
		if err := f.client.Ack(step.Token, result); err != nil {
			f.t.Fatalf("acknowledging %+v: %v", step, err)
		}
	}
	f.t.Fatal("controller did not become idle")
}

func (f *controllerFixture) mix(frames int) mix.RemoteBatch {
	f.t.Helper()
	if err := f.worker.Mix(make([]float32, frames*2)); err != nil {
		f.t.Fatal(err)
	}
	batch, err := f.worker.Drain()
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.client.ApplyBatch(batch); err != nil {
		f.t.Fatal(err)
	}
	return batch
}

func TestControllerTransfersSharedMemoryAndRetiresAfterAllReleases(t *testing.T) {
	f := newControllerFixture(t)
	pcm := make([]float32, MaxTransferSamples+17)
	for i := range pcm {
		pcm[i] = float32(i%17) / 20
	}
	source := &mix.Memory{PCM: pcm, Rate: 48000, Channels: 1, Loop: true}
	a := f.mixer.Play(mix.PlayParams{Memory: source, Gain: 1})
	b := f.mixer.Play(mix.PlayParams{Memory: source, Gain: 1})
	f.drive()
	var chunks, begins, plays int
	for _, step := range f.steps {
		switch {
		case step.Chunk:
			chunks++
		case step.Op == BeginMemory:
			begins++
		case step.Op == Command:
			plays++
		}
	}
	if chunks != 2 || begins != 1 || plays != 2 || len(f.client.sources) != 1 || f.client.sources[0].refs != 2 {
		t.Fatalf("shared transfer: chunks=%d begins=%d plays=%d sources=%+v", chunks, begins, plays, f.client.sources)
	}
	f.mix(1)
	f.mixer.Kill(a)
	f.drive()
	f.mix(1)
	f.drive()
	if len(f.client.sources) != 1 || f.client.sources[0].refs != 1 {
		t.Fatal("first release dropped a source still used by the second voice")
	}
	f.mixer.Kill(b)
	f.drive()
	f.mix(1)
	f.drive()
	if len(f.client.sources) != 0 || len(f.client.voices) != 0 {
		t.Fatal("fully released memory source was retained")
	}
	// A later play re-registers rather than sending an already dropped ID.
	if !f.mixer.Play(mix.PlayParams{Memory: source, Gain: 1}).Valid() {
		t.Fatal("cannot reserve voice after source retirement")
	}
	f.drive()
	if len(f.client.sources) != 1 || f.client.sources[0].id != 2 {
		t.Fatal("later play reused a retired worker source")
	}
}

func TestControllerStreamEOFConsumptionAndRelease(t *testing.T) {
	f := newControllerFixture(t)
	source := mix.NewStream(48000, 1, 8)
	pcm := []float32{.1, .2, .3, .4, .5, .6, .7, .8}
	if source.Ring.Write(pcm) != len(pcm) {
		t.Fatal("cannot fill test stream")
	}
	source.SetEOF()
	id := f.mixer.Play(mix.PlayParams{Stream: source, Gain: 1})
	f.drive()
	if len(f.client.sources) != 1 || !f.client.sources[0].primed || source.Ring.Len() != len(pcm) {
		t.Fatal("initial stream transfer did not retain prefetched PCM")
	}
	if err := f.client.PollStreams(); err != nil {
		t.Fatal(err)
	}
	f.drive()
	if !f.client.sources[0].streamSend.Ended() {
		t.Fatal("worker did not accept EOF after all PCM")
	}
	f.mix(1)
	if err := f.client.PollStreams(); err != nil {
		t.Fatal(err)
	}
	f.drive()
	if source.Ring.Len() == len(pcm) {
		t.Fatal("EOF stopped worker consumption reconciliation")
	}
	f.mixer.Kill(id)
	f.drive()
	f.mix(1)
	f.drive()
	if len(f.client.sources) != 0 {
		t.Fatal("stopped stream was not reconciled and dropped")
	}
	claim, err := mix.NewRemoteStreamSender(source)
	if err != nil {
		t.Fatal("stream claim was not released", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestControllerRequiresSingleFlightAndFailsClosedOnMalformedBatch(t *testing.T) {
	f := newControllerFixture(t)
	voice := f.mixer.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1}, Rate: 48000, Channels: 1}, Gain: 1})
	step, ok, err := f.client.Next()
	if err != nil || !ok || step.Op != BeginMemory {
		t.Fatalf("first step: %+v, %v, %v", step, ok, err)
	}
	if _, _, err := f.client.Next(); err == nil {
		t.Fatal("two operations were allowed in flight")
	}
	if err := f.client.Ack(step.Token, f.invoke(step)); err != nil {
		t.Fatal(err)
	}
	f.drive()
	before := f.mixer.Live(voice)
	bad := mix.RemoteBatch{Serial: 1, Buses: make([]mix.RemoteBus, 1), Released: []mix.VoiceID{{Index: 123, Gen: 4}}}
	if err := f.client.ApplyBatch(bad); err == nil || f.mixer.Live(voice) != before {
		t.Fatal("a fabricated release changed local voice lifetime")
	}
	if _, _, err := f.client.Next(); err == nil {
		t.Fatal("controller continued after malformed batch")
	}
	if err := f.client.AbortAfterTermination(); err != nil {
		t.Fatal(err)
	}
}

func TestControllerWrongAcknowledgementFailsClosed(t *testing.T) {
	f := newControllerFixture(t)
	f.mixer.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1}, Rate: 48000, Channels: 1}, Gain: 1})
	step, ok, err := f.client.Next()
	if err != nil || !ok {
		t.Fatal("cannot start source transfer", err)
	}
	if err := f.client.Ack(step.Token+1, nil); err == nil {
		t.Fatal("accepted the wrong acknowledgement")
	}
	if err := f.client.Ack(step.Token, nil); err == nil {
		t.Fatal("resumed after an out-of-order acknowledgement")
	}
	if _, _, err := f.client.Next(); err == nil {
		t.Fatal("transport remained active after an invalid acknowledgement")
	}
	if err := f.client.AbortAfterTermination(); err != nil {
		t.Fatal(err)
	}
}

func TestControllerRejectsReleaseBeforePlayWasApplied(t *testing.T) {
	f := newControllerFixture(t)
	id := f.mixer.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1}, Rate: 48000, Channels: 1}, Gain: 1})
	step, ok, err := f.client.Next()
	if err != nil || !ok || step.Op != BeginMemory {
		t.Fatalf("source was not staged: %+v, %v, %v", step, ok, err)
	}
	// No Play command has reached the worker: a batch that claims a release
	// despite applied=0 must not free the local handle or its source.
	batch := mix.RemoteBatch{Serial: 1, Applied: 0, Buses: make([]mix.RemoteBus, 1), Released: []mix.VoiceID{id}}
	if err := f.client.ApplyBatch(batch); err == nil || !f.mixer.Live(id) {
		t.Fatal("released a voice whose Play command had not been applied")
	}
	if err := f.client.AbortAfterTermination(); err != nil {
		t.Fatal(err)
	}
}

func TestControllerInvalidPlayKeepsMixerRejection(t *testing.T) {
	f := newControllerFixture(t)
	memory := &mix.Memory{PCM: []float32{1}, Rate: 48000, Channels: 1}
	stream := mix.NewStream(48000, 1, 8)
	id := f.mixer.Play(mix.PlayParams{Memory: memory, Stream: stream, Gain: 1})
	f.drive()
	if len(f.client.sources) != 0 || len(f.steps) != 1 || f.steps[0].Request.Command.Source != 0 {
		t.Fatal("ambiguous source was turned into a valid source")
	}
	batch := f.mix(1)
	if !reflect.DeepEqual(batch.Released, []mix.VoiceID{id}) || len(batch.Events) != 1 || batch.Events[0].Kind != mix.EvRejected {
		t.Fatalf("invalid play did not preserve mixer rejection: %+v", batch)
	}
}

func TestControllerInvalidStreamFormatKeepsMixerRejection(t *testing.T) {
	for _, stream := range []*mix.Stream{
		{Rate: 48000, Channels: 0},
		{Rate: 0, Channels: 1},
	} {
		f := newControllerFixture(t)
		id := f.mixer.Play(mix.PlayParams{Stream: stream, Gain: 1})
		f.drive()
		batch := f.mix(1)
		if len(f.client.sources) != 0 || !reflect.DeepEqual(batch.Released, []mix.VoiceID{id}) ||
			len(batch.Events) != 1 || batch.Events[0].Kind != mix.EvRejected {
			t.Fatalf("invalid stream did not preserve mixer rejection: %+v", batch)
		}
	}
}

func TestControllerPollsStreamsFairly(t *testing.T) {
	f := newControllerFixture(t)
	for i := 0; i < 2; i++ {
		stream := mix.NewStream(48000, 1, 32)
		stream.Ring.Write([]float32{.1, .2, .3, .4, .5, .6, .7, .8})
		if !f.mixer.Play(mix.PlayParams{Stream: stream, Gain: 1}).Valid() {
			t.Fatal("cannot reserve stream voice")
		}
	}
	f.drive()
	f.mix(1)
	if err := f.client.PollStreams(); err != nil {
		t.Fatal(err)
	}
	first, ok, err := f.client.Next()
	if err != nil || !ok || first.Op != StreamStatus {
		t.Fatalf("first stream status: %+v, %v, %v", first, ok, err)
	}
	if err := f.client.Ack(first.Token, f.invoke(first)); err != nil {
		t.Fatal(err)
	}
	// A timer can request another cycle while the first stream is in flight.
	// That must not indefinitely starve the other stream's status.
	if err := f.client.PollStreams(); err != nil {
		t.Fatal(err)
	}
	second, ok, err := f.client.Next()
	if err != nil || !ok || second.Op != StreamStatus || second.Request.Source == first.Request.Source {
		t.Fatalf("second stream was starved: first=%+v second=%+v, %v, %v", first, second, ok, err)
	}
	if err := f.client.Ack(second.Token, f.invoke(second)); err != nil {
		t.Fatal(err)
	}
	f.drive()
}

func TestControllerPollsCannotStarveQueuedCommands(t *testing.T) {
	f := newControllerFixture(t)
	var producing *mix.Stream
	for i := 0; i < 2; i++ {
		stream := mix.NewStream(48000, 1, 32)
		stream.Ring.Write([]float32{.1, .2, .3, .4, .5, .6, .7, .8})
		f.mixer.Play(mix.PlayParams{Stream: stream, Gain: 1})
		if i == 1 {
			producing = stream
		}
	}
	f.drive()
	f.mix(1)
	f.mixer.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1}, Rate: 48000, Channels: 1}, Gain: 1})
	for i := 0; i < 20; i++ {
		producing.Ring.Write([]float32{.1})
		if err := f.client.PollStreams(); err != nil {
			t.Fatal(err)
		}
		step, ok, err := f.client.Next()
		if err != nil || !ok {
			t.Fatalf("unexpected idle or failure: %v, %v", ok, err)
		}
		if err := f.client.Ack(step.Token, f.invoke(step)); err != nil {
			t.Fatal(err)
		}
		if step.Op == BeginMemory {
			return
		}
	}
	t.Fatal("frequent stream polls starved a queued play command")
}

func TestControllerRefillsStreamDuringLargeMemoryTransfer(t *testing.T) {
	f := newControllerFixture(t)
	stream := mix.NewStream(48000, 1, 32)
	stream.Ring.Write([]float32{.1, .2, .3, .4, .5, .6, .7, .8})
	f.mixer.Play(mix.PlayParams{Stream: stream, Gain: 1})
	f.drive()
	f.mix(1)
	stream.Ring.Write([]float32{.9, 1})
	f.mixer.Play(mix.PlayParams{Memory: &mix.Memory{PCM: make([]float32, MaxTransferSamples+1), Rate: 48000, Channels: 1}, Gain: 1})
	begin, ok, err := f.client.Next()
	if err != nil || !ok || begin.Op != BeginMemory {
		t.Fatalf("memory transfer did not start: %+v, %v, %v", begin, ok, err)
	}
	if err := f.client.Ack(begin.Token, f.invoke(begin)); err != nil {
		t.Fatal(err)
	}
	if err := f.client.PollStreams(); err != nil {
		t.Fatal(err)
	}
	status, ok, err := f.client.Next()
	if err != nil || !ok || status.Op != StreamStatus {
		t.Fatalf("memory transfer starved the stream: %+v, %v, %v", status, ok, err)
	}
	if err := f.client.Ack(status.Token, f.invoke(status)); err != nil {
		t.Fatal(err)
	}
	write, ok, err := f.client.Next()
	if err != nil || !ok || write.Op != StreamWrite || len(write.PCM) != 2 {
		t.Fatalf("new stream PCM was not transferred: %+v, %v, %v", write, ok, err)
	}
	if err := f.client.Ack(write.Token, f.invoke(write)); err != nil {
		t.Fatal(err)
	}
	f.drive()
}

func TestControllerCancelsOnlyBeforeAnyCommandIsTaken(t *testing.T) {
	cfg := mix.Config{SampleRate: 48000, Voices: 4, Block: 64}
	local, err := mix.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	src := &mix.Memory{PCM: []float32{.25, .5, .75, 1}, Rate: 48000, Channels: 1}
	id := local.Play(mix.PlayParams{Memory: src, Gain: 1})
	client, err := NewController(local)
	if err != nil || client.CancelUnsent() != nil {
		t.Fatalf("unsent startup did not return local ownership: %v", err)
	}
	out := make([]float32, 8)
	local.Mix(out)
	if !id.Valid() || reflect.DeepEqual(out, make([]float32, len(out))) {
		t.Fatal("unsent fallback lost the queued play command")
	}
	f := newControllerFixture(t)
	f.mixer.Play(mix.PlayParams{Memory: src, Gain: 1})
	if _, ok, err := f.client.Next(); err != nil || !ok {
		t.Fatalf("cannot start transfer: %v", err)
	}
	if err := f.client.CancelUnsent(); err == nil {
		t.Fatal("uncertain transport was incorrectly returned to the local mixer")
	}
}

func TestControllerReconcilesStoppedStreamWhileAnotherVoiceMixes(t *testing.T) {
	f := newControllerFixture(t)
	source := mix.NewStream(48000, 1, 32)
	source.Ring.Write([]float32{.1, .2, .3, .4, .5, .6, .7, .8})
	streamVoice := f.mixer.Play(mix.PlayParams{Stream: source, Gain: 1})
	other := f.mixer.Play(mix.PlayParams{Memory: &mix.Memory{PCM: []float32{1, 1, 1, 1}, Rate: 48000, Channels: 1, Loop: true}, Gain: 1})
	f.drive()
	f.mix(1)
	f.mixer.Kill(streamVoice)
	f.drive()
	f.mix(1)
	step, ok, err := f.client.Next()
	if err != nil || !ok || step.Op != StreamStatus {
		t.Fatalf("missing final stream status: %+v, %v, %v", step, ok, err)
	}
	if err := f.client.Ack(step.Token, f.invoke(step)); err != nil {
		t.Fatal(err)
	}
	// The source's voice is released; rendering a different voice between
	// final status and Drop must not invalidate its consumption accounting.
	f.mix(3)
	f.drive()
	if len(f.client.streams) != 0 || !f.mixer.Live(other) {
		t.Fatal("reconciling a stopped source disturbed an independent voice")
	}
	claim, err := mix.NewRemoteStreamSender(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := claim.Close(); err != nil {
		t.Fatal(err)
	}
}
