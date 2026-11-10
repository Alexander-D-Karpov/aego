package main

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"

	"aego/audio/mix"
	"aego/audio/workerproto"
)

func call(t *testing.T, s *service, op int32, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.invoke(op, raw)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func initService(t *testing.T, budget int64) *service {
	t.Helper()
	s := &service{}
	call(t, s, opInit, request{Rate: 48000, Frames: 512, Voices: 4, Budget: budget})
	return s
}

func TestServiceMemoryVoiceAndRelease(t *testing.T) {
	s := initService(t, 1<<20)
	call(t, s, opBeginMemory, request{Source: 7, Rate: 48000, Channels: 1, Samples: 128})
	if len(s.memory) != 128 {
		t.Fatal("staged PCM has the wrong length")
	}
	for i := range s.memory {
		s.memory[i] = .25
	}
	call(t, s, opCommitMemory, request{Source: 7})
	p := commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1, Source: 7,
		GainBits: math.Float32bits(1), PitchBits: math.Float32bits(1)}
	call(t, s, opCommand, request{Command: p})
	if _, err := s.invoke(opDrop, []byte(`{"source":"7"}`)); err == nil {
		t.Fatal("dropped a source while its voice was queued")
	}
	out, err := s.render(64)
	if err != nil || len(out) != 128 || out[0] == 0 {
		t.Fatalf("render: len=%d err=%v", len(out), err)
	}
	var first wireBatch
	if err := json.Unmarshal(call(t, s, opDrain, request{}), &first); err != nil {
		t.Fatal(err)
	}
	if first.Applied != 1 || first.Active != 1 || len(first.Voices) != 1 {
		t.Fatalf("first batch %+v", first)
	}
	if _, err := s.render(128); err != nil {
		t.Fatal(err)
	}
	var last wireBatch
	if err := json.Unmarshal(call(t, s, opDrain, request{}), &last); err != nil {
		t.Fatal(err)
	}
	if len(last.Released) != 1 || last.Released[0].Gen != 1 || last.Released[0].Index != 0 {
		t.Fatalf("release batch %+v", last)
	}
	call(t, s, opDrop, request{Source: 7})
}

func TestMemorySenderTransfersToServiceInBoundedChunks(t *testing.T) {
	s := initService(t, 1<<20)
	pcm := make([]float32, 129)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i) * .05))
	}
	const sourceID uint64 = 1<<53 + 9
	sender, err := workerproto.NewMemorySender(sourceID, &mix.Memory{PCM: pcm, Rate: 48000, Channels: 1}, 32)
	if err != nil {
		t.Fatal(err)
	}
	chunks := 0
	for {
		step, ok, err := sender.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if step.Chunk {
			if s.memoryID != sourceID || len(step.PCM) > 32 || step.Offset+len(step.PCM) > len(s.memory) {
				t.Fatalf("invalid staged chunk %+v", step)
			}
			copy(s.memory[step.Offset:], step.PCM)
			chunks++
		} else {
			call(t, s, step.Op, step.Request)
		}
		if err := sender.Ack(step.Token); err != nil {
			t.Fatal(err)
		}
	}
	if chunks != 5 || s.memoryID != 0 {
		t.Fatalf("transfer chunks = %d, staged source = %d", chunks, s.memoryID)
	}
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1,
		Source: sourceID, GainBits: math.Float32bits(1), PitchBits: math.Float32bits(1)}})
	got, err := s.render(64)
	if err != nil || len(got) != 128 || got[2] == 0 {
		t.Fatalf("transferred PCM did not play: len=%d, error=%v", len(got), err)
	}
}

// This exercises the actual producer client, common wire encoding and WASI
// service together; it is not a browser transport or a device test.
func TestServiceWireConnectsProducerToMixer(t *testing.T) {
	s := initService(t, 1<<20)
	cfg := mix.Config{SampleRate: 48000, Channels: 2, Voices: 4, Block: mix.DefaultBlock}
	producer, err := mix.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := mix.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mix.NewRemoteClient(producer)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]float32, 128)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i) * .08))
	}
	const source uint64 = 1<<53 + 17
	call(t, s, opBeginMemory, request{Source: source, Rate: 48000, Channels: 1, Samples: len(pcm)})
	copy(s.memory, pcm)
	call(t, s, opCommitMemory, request{Source: source})
	params := mix.PlayParams{Memory: &mix.Memory{PCM: pcm, Rate: 48000, Channels: 1}, Gain: .7, Pitch: 1}
	id, wantID := producer.Play(params), reference.Play(params)
	if id != wantID || !id.Valid() || !producer.SetGain(id, .3, 0) || !reference.SetGain(wantID, .3, 0) {
		t.Fatalf("cannot prepare matching producer voices: %v, %v", id, wantID)
	}
	for {
		command, ok, err := client.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		packet := mix.RemoteSourceCommand{RemoteCommand: command}
		if command.Command.Kind == mix.CmdPlay {
			packet.Source = source
		}
		call(t, s, opCommand, request{Command: workerproto.CommandFromMix(packet)})
	}
	for _, frames := range []int{64, 128} {
		want := make([]float32, frames*2)
		reference.Mix(want)
		got, err := s.render(frames)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
				t.Fatalf("frame batch %d sample %d: worker %v, reference %v", frames, i, got[i], want[i])
			}
		}
		batch, err := workerproto.DecodeBatch(call(t, s, opDrain, request{}))
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Apply(batch.Mix()); err != nil {
			t.Fatal(err)
		}
		if producer.Live(id) != reference.Live(wantID) || producer.Active() != reference.Active() ||
			producer.Position(id) != reference.Position(wantID) || producer.VoiceGain(id) != reference.VoiceGain(wantID) {
			t.Fatalf("producer state differs from reference after %d frames", frames)
		}
		for {
			a, aok := producer.Events().Pop()
			b, bok := reference.Events().Pop()
			if a != b || aok != bok {
				t.Fatalf("worker event %v/%v, reference %v/%v", a, aok, b, bok)
			}
			if !aok {
				break
			}
		}
	}
	call(t, s, opDrop, request{Source: source})
}

func TestServiceSampleRateChangeKeepsVoiceAndPCM(t *testing.T) {
	s := initService(t, 2<<20)
	call(t, s, opBeginMemory, request{Source: 11, Rate: 48000, Channels: 1, Samples: 256})
	pcm := make([]float32, len(s.memory))
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i) * .03))
	}
	copy(s.memory, pcm)
	call(t, s, opCommitMemory, request{Source: 11})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1, Source: 11,
		GainBits: math.Float32bits(.7), PitchBits: math.Float32bits(1), Flag: true}})
	ref, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Voices: 4, Block: mix.DefaultBlock})
	if err != nil {
		t.Fatal(err)
	}
	ref.Play(mix.PlayParams{Memory: &mix.Memory{PCM: pcm, Rate: 48000, Channels: 1}, Gain: .7, Pitch: 1, Loop: true})
	compare := func() {
		t.Helper()
		want := make([]float32, 128)
		ref.Mix(want)
		got, err := s.render(len(want) / 2)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
				t.Fatalf("sample %d = %v, native = %v", i, got[i], want[i])
			}
		}
	}
	compare()
	call(t, s, opSetSampleRate, request{Rate: 44100})
	ref.SetSampleRate(44100)
	compare()
	if _, err := s.invoke(opSetSampleRate, []byte(`{"rate":0}`)); err == nil {
		t.Fatal("accepted an invalid output rate")
	}
	compare()
}

func TestServiceStreamFlowAndEOF(t *testing.T) {
	s := initService(t, 2<<20)
	call(t, s, opStream, request{Source: 3, Rate: 48000, Channels: 2, Frames: 64})
	buffer, err := s.streamBuffer(128)
	if err != nil {
		t.Fatal(err)
	}
	for i := range buffer {
		buffer[i] = .2
	}
	var written struct {
		Frames int `json:"frames"`
	}
	if err := json.Unmarshal(call(t, s, opStreamWrite, request{Source: 3, Samples: 128}), &written); err != nil {
		t.Fatal(err)
	}
	if written.Frames != 64 {
		t.Fatalf("wrote %d frames", written.Frames)
	}
	call(t, s, opStreamEnd, request{Source: 3})
	if _, err := s.invoke(opStreamWrite, []byte(`{"source":"3","samples":128}`)); err == nil {
		t.Fatal("accepted stream data after EOF")
	}
	if _, err := s.streamBuffer(maxStreamSamples + 1); err == nil {
		t.Fatal("unbounded streaming transfer")
	}
}

func TestServiceStreamControlAcknowledgement(t *testing.T) {
	s := initService(t, 2<<20)
	call(t, s, opStream, request{Source: 3, Rate: 48000, Channels: 1, Frames: 8})
	buffer, err := s.streamBuffer(8)
	if err != nil {
		t.Fatal(err)
	}
	for i := range buffer {
		buffer[i] = float32(i)
	}
	call(t, s, opStreamWrite, request{Source: 3, Samples: 8})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1,
		Source: 3, GainBits: math.Float32bits(1), PitchBits: math.Float32bits(1)}})
	call(t, s, opSkip, request{Frames: 20})
	var status mix.RemoteStreamStatus
	if err := json.Unmarshal(call(t, s, opStreamStatus, request{Source: 3}), &status); err != nil {
		t.Fatal(err)
	}
	if status.Control == 0 || status.Skip == 0 || !status.Held {
		t.Fatalf("skip status = %+v", status)
	}
	call(t, s, opStreamControlAck, request{Source: 3, Control: status.Control})
	if _, err := s.invoke(opStreamControlAck, []byte(`{"source":"3","control":1}`)); err == nil {
		t.Fatal("accepted a duplicate acknowledgement")
	}
}

func TestServiceStreamEOFDefersAcrossAStaleStatus(t *testing.T) {
	s := initService(t, 2<<20)
	call(t, s, opStream, request{Source: 3, Rate: 48000, Channels: 1, Frames: 8})
	buffer, err := s.streamBuffer(8)
	if err != nil {
		t.Fatal(err)
	}
	for i := range buffer {
		buffer[i] = .2
	}
	call(t, s, opStreamWrite, request{Source: 3, Samples: 8})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1,
		Source: 3, GainBits: math.Float32bits(1), PitchBits: math.Float32bits(1)}})
	var before mix.RemoteStreamStatus
	if err := json.Unmarshal(call(t, s, opStreamStatus, request{Source: 3}), &before); err != nil {
		t.Fatal(err)
	}
	if before.Control != 0 || before.EOF {
		t.Fatalf("unexpected initial status %+v", before)
	}
	// The worker can skip after status was returned but before the sender's
	// ordered EOF request arrives. It must defer EOF, not break transport.
	call(t, s, opSkip, request{Frames: 20})
	var reply struct {
		Ended bool `json:"ended"`
	}
	if err := json.Unmarshal(call(t, s, opTryStreamEnd, request{Source: 3}), &reply); err != nil || reply.Ended {
		t.Fatalf("stale EOF = %+v, %v", reply, err)
	}
	var control mix.RemoteStreamStatus
	if err := json.Unmarshal(call(t, s, opStreamStatus, request{Source: 3}), &control); err != nil {
		t.Fatal(err)
	}
	if control.Control == 0 || control.Skip == 0 || control.EOF {
		t.Fatalf("control or EOF lost: %+v", control)
	}
	call(t, s, opStreamControlAck, request{Source: 3, Control: control.Control})
	if err := json.Unmarshal(call(t, s, opTryStreamEnd, request{Source: 3}), &reply); err != nil || !reply.Ended {
		t.Fatalf("reconciled EOF = %+v, %v", reply, err)
	}
	if _, err := s.invoke(opTryStreamEnd, []byte(`{"source":"3"}`)); err == nil {
		t.Fatal("accepted a duplicate EOF")
	}
}

func TestStreamSenderReconcilesStaleEOFWithService(t *testing.T) {
	service := initService(t, 2<<20)
	source := mix.NewStream(48000, 1, 8)
	if n := source.Ring.Write([]float32{1, 2, 3, 4, 5, 6, 7, 8}); n != 8 {
		t.Fatal(n)
	}
	source.SetEOF()
	sender, err := workerproto.NewStreamSender(3, source, 8)
	if err != nil {
		t.Fatal(err)
	}
	next := func(op int32) workerproto.StreamStep {
		t.Helper()
		step, ok, err := sender.Next()
		if err != nil || !ok || step.Op != op {
			t.Fatalf("step = %+v, %v, %v; want %d", step, ok, err, op)
		}
		return step
	}
	apply := func(step workerproto.StreamStep) []byte {
		t.Helper()
		if step.Op == opStreamWrite {
			buf, err := service.streamBuffer(len(step.PCM))
			if err != nil {
				t.Fatal(err)
			}
			copy(buf, step.PCM)
		}
		result := call(t, service, step.Op, step.Request)
		if err := sender.Ack(step.Token, result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	apply(next(opStream))
	if err := sender.Poll(); err != nil {
		t.Fatal(err)
	}
	apply(next(opStreamStatus))
	apply(next(opStreamWrite))
	if source.Ring.Len() != 0 {
		t.Fatal("worker PCM acknowledgement did not consume the source")
	}
	call(t, service, opCommand, request{Command: commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1,
		Source: 3, GainBits: math.Float32bits(1), PitchBits: math.Float32bits(1)}})
	if err := sender.Poll(); err != nil {
		t.Fatal(err)
	}
	apply(next(opStreamStatus))
	end := next(opTryStreamEnd)
	// Invalidate the earlier status while EOF is in flight.
	call(t, service, opSkip, request{Frames: 20})
	var reply struct {
		Ended bool `json:"ended"`
	}
	if err := json.Unmarshal(apply(end), &reply); err != nil || reply.Ended {
		t.Fatalf("stale end reply = %+v, %v", reply, err)
	}
	if err := sender.Poll(); err != nil {
		t.Fatal(err)
	}
	apply(next(opStreamStatus))
	control := next(opStreamControlAck)
	if control.Request.Control == 0 || !source.Held() {
		t.Fatalf("skip was not applied to the source: %+v", control)
	}
	apply(control)
	if err := sender.Poll(); err != nil {
		t.Fatal(err)
	}
	apply(next(opStreamStatus))
	if err := json.Unmarshal(apply(next(opTryStreamEnd)), &reply); err != nil || !reply.Ended {
		t.Fatalf("reconciled end reply = %+v, %v", reply, err)
	}
	call(t, service, opSkip, request{Frames: 1})
	var batch wireBatch
	if err := json.Unmarshal(call(t, service, opDrain, request{}), &batch); err != nil || len(batch.Released) != 1 {
		t.Fatalf("held EOF did not release the voice: %+v, %v", batch, err)
	}
	call(t, service, opDrop, request{Source: 3})
	if err := sender.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServiceRejectsMalformedRequestsWithoutReplacingState(t *testing.T) {
	s := initService(t, 1<<20)
	for _, raw := range [][]byte{
		[]byte(`{"rate":48000,"unknown":1}`),
		[]byte(`{} {}`),
		[]byte(`null`),
		[]byte(`not-json`),
	} {
		if _, err := s.invoke(opBeginMemory, raw); err == nil {
			t.Fatalf("accepted malformed request %q", raw)
		}
	}
	if _, err := s.invoke(opInit, []byte(`{"rate":48000,"frames":512,"voices":4,"budget":1048576}`)); err == nil {
		t.Fatal("reinitialized a live worker")
	}
	if _, err := s.render(maxOutputFrames + 1); err == nil {
		t.Fatal("accepted oversized render")
	}
	call(t, s, opBeginMemory, request{Source: 9, Rate: 48000, Channels: 1, Samples: 16})
	call(t, s, opDrop, request{Source: 9})
}

func TestServiceBatchPreservesNonFiniteFloatBits(t *testing.T) {
	s := initService(t, 1<<20)
	call(t, s, opBeginMemory, request{Source: 1, Rate: 48000, Channels: 1, Samples: 128})
	for i := range s.memory {
		s.memory[i] = .25
	}
	call(t, s, opCommitMemory, request{Source: 1})
	// JSON cannot encode a NaN as a number; the protocol sends the raw bits.
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1,
		Source: 1, GainBits: 0x7fc01234, PitchBits: math.Float32bits(1)}})
	if _, err := s.render(64); err != nil {
		t.Fatal(err)
	}
	var batch wireBatch
	if err := json.Unmarshal(call(t, s, opDrain, request{}), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Voices) != 1 || !math.IsNaN(float64(math.Float32frombits(batch.Voices[0].GainBits))) {
		t.Fatalf("voice gain did not preserve NaN semantics: %+v", batch.Voices)
	}
}

func TestServiceSourceIDDoesNotLoseUint64Precision(t *testing.T) {
	s := initService(t, 1<<20)
	const id uint64 = 1<<53 + 1
	call(t, s, opBeginMemory, request{Source: id, Rate: 48000, Channels: 1, Samples: 8})
	call(t, s, opCommitMemory, request{Source: id})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1,
		Source: id, GainBits: math.Float32bits(1), PitchBits: math.Float32bits(1)}})
	if _, err := s.render(16); err != nil {
		t.Fatal(err)
	}
	call(t, s, opDrain, request{})
	call(t, s, opDrop, request{Source: id})
}

func TestServiceWireRequiresExactWideIntegers(t *testing.T) {
	// JS numbers cannot represent every uint64. A transport must not round an
	// ID or command sequence before the Go service gets a chance to validate it.
	for _, raw := range [][]byte{
		[]byte(`{"source":9007199254740993}`),
		[]byte(`{"command":{"sequence":9007199254740993}}`),
		[]byte(`{"command":{"source":9007199254740993}}`),
	} {
		if _, err := decodeRequest(raw); err == nil {
			t.Fatalf("accepted an imprecise numeric wire value: %s", raw)
		}
	}
	const value uint64 = 1<<53 + 1
	data, err := json.Marshal(batchWire(mix.RemoteBatch{Serial: value, Applied: value, EventsDropped: value}))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(data, []byte(`"9007199254740993"`)) != 3 {
		t.Fatalf("wide counters were not encoded as exact decimal strings: %s", data)
	}
}
