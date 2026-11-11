package mix

import (
	"math"
	"testing"
)

func sourcePacket(t *testing.T, c *RemoteClient, id uint64) RemoteSourceCommand {
	t.Helper()
	p, ok, err := c.Next()
	if err != nil || !ok {
		t.Fatalf("next remote command: %v %v", ok, err)
	}
	p.Command.Memory, p.Command.Stream = nil, nil
	return RemoteSourceCommand{RemoteCommand: p, Source: id}
}

func TestRemoteSourcesMemoryOwnershipAndVoiceLifetime(t *testing.T) {
	cfg := Config{SampleRate: rate, Channels: 2, Voices: 4, Block: 64}
	clientMixer, _ := New(cfg)
	client, err := NewRemoteClient(clientMixer)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewRemoteSources(cfg, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]float32, 128)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i) * .03))
	}
	source := &Memory{PCM: pcm, Rate: rate, Channels: 1}
	if err := worker.RegisterMemory(7, source); err != nil {
		t.Fatal(err)
	}
	if err := worker.RegisterMemory(7, source); err == nil {
		t.Fatal("duplicate source replaced the registered source")
	}
	// Registration owns its copy; replacing or changing the caller's buffer
	// cannot mutate audio already sent to a separate worker.
	clear(pcm)
	id := clientMixer.Play(PlayParams{Memory: source, Gain: 1})
	if !id.Valid() {
		t.Fatal("play refused")
	}
	if err := worker.Accept(sourcePacket(t, client, 7)); err != nil {
		t.Fatal(err)
	}
	if err := worker.Drop(7); err == nil {
		t.Fatal("dropped a source still used by an accepted voice")
	}
	first := make([]float32, 128)
	if err := worker.Mix(first); err != nil {
		t.Fatal(err)
	}
	var sum float64
	for _, x := range first {
		sum += float64(x * x)
	}
	if sum == 0 {
		t.Fatal("registered PCM was aliased to the caller's modified buffer")
	}
	batch, err := worker.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	if err := worker.Mix(make([]float32, 256)); err != nil {
		t.Fatal(err)
	}
	batch, err = worker.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	if clientMixer.Live(id) {
		t.Fatal("finished voice was not released")
	}
	if err := worker.Drop(7); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteSourcesBudgetAndInvalidCommands(t *testing.T) {
	w, err := NewRemoteSources(Config{SampleRate: rate}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.RegisterMemory(0, &Memory{}); err == nil {
		t.Fatal("zero source ID accepted")
	}
	if err := w.RegisterMemory(1, &Memory{PCM: make([]float32, 257)}); err == nil {
		t.Fatal("memory budget exceeded")
	}
	if err := w.RegisterStream(2, rate, 2, 48000); err == nil {
		t.Fatal("stream exceeded the total budget")
	}
	if err := w.Accept(RemoteSourceCommand{RemoteCommand: RemoteCommand{Sequence: 1,
		Command: Command{Kind: CmdPlay, Voice: VoiceID{Gen: 1}, Memory: &Memory{}}}, Source: 1}); err == nil {
		t.Fatal("Go pointer accepted across the source protocol")
	}
	if err := w.Drop(99); err == nil {
		t.Fatal("missing source reported as dropped")
	}
}

func TestRemoteSourcesInvalidPlayIsRejectedLikeNative(t *testing.T) {
	for _, params := range []PlayParams{
		{Gain: 1},
		{Memory: dc(.2, 64), Stream: NewStream(rate, 1, 64), Gain: 1},
	} {
		cfg := Config{SampleRate: rate, Voices: 4}
		local, _ := New(cfg)
		reference, _ := New(cfg)
		client, err := NewRemoteClient(local)
		if err != nil {
			t.Fatal(err)
		}
		worker, err := NewRemoteSources(cfg, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		a, b := local.Play(params), reference.Play(params)
		if a != b || !a.Valid() {
			t.Fatalf("voice IDs = %v, %v", a, b)
		}
		// Zero represents an invalid source combination. The worker must let
		// the actual mixer reject it and release the same handle, rather than
		// treating a legal producer command as a broken transport.
		if err := worker.Accept(sourcePacket(t, client, 0)); err != nil {
			t.Fatal(err)
		}
		reference.Mix(make([]float32, 128))
		if err := worker.Mix(make([]float32, 128)); err != nil {
			t.Fatal(err)
		}
		batch, err := worker.Drain()
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Apply(batch); err != nil {
			t.Fatal(err)
		}
		got, ok := local.Events().Pop()
		want, wantOK := reference.Events().Pop()
		if got != want || ok != wantOK || !ok || got.Kind != EvRejected || local.Live(a) {
			t.Fatalf("remote event %v/%v, native %v/%v, live=%v", got, ok, want, wantOK, local.Live(a))
		}
	}
}

func TestRemoteSourcesStagedMemoryRequiresCommit(t *testing.T) {
	cfg := Config{SampleRate: rate}
	local, _ := New(cfg)
	client, err := NewRemoteClient(local)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewRemoteSources(cfg, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := w.BeginMemory(5, Memory{Rate: rate, Channels: 1}, 128)
	if err != nil {
		t.Fatal(err)
	}
	for i := range pcm {
		pcm[i] = .25
	}
	id := local.Play(PlayParams{Memory: &Memory{PCM: make([]float32, 128), Rate: rate, Channels: 1}, Gain: 1})
	if !id.Valid() {
		t.Fatal("play refused")
	}
	packet := sourcePacket(t, client, 5)
	if err := w.Accept(packet); err == nil {
		t.Fatal("played an uncommitted source")
	}
	if err := w.CommitMemory(5); err != nil {
		t.Fatal(err)
	}
	if err := w.CommitMemory(5); err == nil {
		t.Fatal("committed source twice")
	}
	if err := w.Accept(packet); err != nil {
		t.Fatal(err)
	}
	out := make([]float32, 128)
	if err := w.Mix(out); err != nil {
		t.Fatal(err)
	}
	if out[0] == 0 {
		t.Fatal("committed source did not render")
	}
	if _, err := w.BeginMemory(6, Memory{Rate: rate, Channels: 1}, 129); err == nil {
		t.Fatal("staged memory exceeded the remaining budget")
	}
}

func TestRemoteSourcesStreamPCMAndEOF(t *testing.T) {
	cfg := Config{SampleRate: rate, Channels: 2, Voices: 4, Block: 64}
	local, _ := New(cfg)
	reference, _ := New(cfg)
	client, err := NewRemoteClient(local)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewRemoteSources(cfg, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RegisterStream(3, rate, 2, 2048); err != nil {
		t.Fatal(err)
	}
	pcm := make([]float32, 2048)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i)*.03) * .3)
	}
	refStream := NewStream(rate, 2, 2048)
	if n := refStream.Ring.Write(pcm); n != len(pcm)/2 {
		t.Fatalf("reference wrote %d frames", n)
	}
	refStream.SetEOF()
	if n, err := worker.WriteStream(3, pcm); err != nil || n != len(pcm)/2 {
		t.Fatalf("worker wrote %d frames: %v", n, err)
	}
	if err := worker.EndStream(3); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.WriteStream(3, pcm); err == nil {
		t.Fatal("stream accepted PCM after EOF")
	}
	clientStream := NewStream(rate, 2, 2048)
	id := local.Play(PlayParams{Stream: clientStream, Gain: .7})
	if want := reference.Play(PlayParams{Stream: refStream, Gain: .7}); want != id || !id.Valid() {
		t.Fatalf("stream ID %v vs %v", id, want)
	}
	if err := worker.Accept(sourcePacket(t, client, 3)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		want, got := make([]float32, 128), make([]float32, 128)
		reference.Mix(want)
		if err := worker.Mix(got); err != nil {
			t.Fatal(err)
		}
		for j := range want {
			if math.Float32bits(want[j]) != math.Float32bits(got[j]) {
				t.Fatalf("block %d sample %d: %v vs %v", i, j, got[j], want[j])
			}
		}
		batch, err := worker.Drain()
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Apply(batch); err != nil {
			t.Fatal(err)
		}
	}
	if local.Live(id) || reference.Live(id) {
		t.Fatal("EOF left the stream voice live")
	}
	if err := worker.Drop(3); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteSourcesReuseStreamKeepsUnconsumedPCM(t *testing.T) {
	cfg := Config{SampleRate: rate, Voices: 4, Block: 64}
	local, _ := New(cfg)
	reference, _ := New(cfg)
	client, err := NewRemoteClient(local)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewRemoteSources(cfg, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	stream, refStream := NewStream(rate, 1, 8), NewStream(rate, 1, 8)
	pcm := []float32{.1, .2, .3, .4, .5, .6, .7, .8}
	stream.Ring.Write(pcm)
	refStream.Ring.Write(pcm)
	if err := worker.RegisterStream(3, rate, 1, 8); err != nil {
		t.Fatal(err)
	}
	sender, err := NewRemoteStreamSender(stream)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 8)
	n, _, err := sender.Peek(buf)
	if err != nil || n != 8 {
		t.Fatalf("peek = %d, %v", n, err)
	}
	written, err := worker.WriteStream(3, buf[:n])
	if err != nil || written != n {
		t.Fatalf("write = %d, %v", written, err)
	}
	if err := sender.Ack(written); err != nil {
		t.Fatal(err)
	}
	id, wantID := local.Play(PlayParams{Stream: stream, Gain: 1}), reference.Play(PlayParams{Stream: refStream, Gain: 1})
	if id != wantID || !id.Valid() {
		t.Fatalf("voice IDs = %v, %v", id, wantID)
	}
	if err := worker.Accept(sourcePacket(t, client, 3)); err != nil {
		t.Fatal(err)
	}
	got, want := make([]float32, 2), make([]float32, 2)
	reference.Mix(want)
	if err := worker.Mix(got); err != nil {
		t.Fatal(err)
	}
	batch, err := worker.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	local.Kill(id)
	reference.Kill(wantID)
	if err := worker.Accept(sourcePacket(t, client, 0)); err != nil {
		t.Fatal(err)
	}
	reference.Mix(want)
	if err := worker.Mix(got); err != nil {
		t.Fatal(err)
	}
	batch, err = worker.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	status, err := worker.StreamStatus(3)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Reconcile(status.Consumed, status.Buffered); err != nil {
		t.Fatal(err)
	}
	if err := worker.Drop(3); err != nil {
		t.Fatal(err)
	}
	if err := sender.Close(); err != nil {
		t.Fatal(err)
	}
	if stream.Ring.Len() != refStream.Ring.Len() {
		t.Fatalf("stopped stream lost prefetched frames: local %d, native %d", stream.Ring.Len(), refStream.Ring.Len())
	}
}

func TestRemoteSourcesStreamControlRequiresAcknowledgement(t *testing.T) {
	cfg := Config{SampleRate: rate, Channels: 2, Voices: 4, Block: 64}
	local, _ := New(cfg)
	client, err := NewRemoteClient(local)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewRemoteSources(cfg, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RegisterStream(3, rate, 1, 8); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.WriteStream(3, make([]float32, 8)); n != 8 || err != nil {
		t.Fatalf("initial PCM: %d, %v", n, err)
	}
	st := NewStream(rate, 1, 8)
	if id := local.Play(PlayParams{Stream: st, Gain: 1}); !id.Valid() {
		t.Fatal("play refused")
	}
	if err := worker.Accept(sourcePacket(t, client, 3)); err != nil {
		t.Fatal(err)
	}
	if err := worker.Skip(20); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.WriteStream(3, []float32{1}); n != 0 || err != nil {
		t.Fatalf("held worker accepted in-flight PCM: %d, %v", n, err)
	}
	first, err := worker.StreamStatus(3)
	if err != nil || first.Control == 0 || !first.Held || first.Skip == 0 {
		t.Fatalf("missing skip control: %+v, %v", first, err)
	}
	// The worker can resume before the main thread receives the control. The
	// already-reported skip must remain an admission barrier until it is acked.
	if err := worker.Mix(make([]float32, 2)); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.WriteStream(3, []float32{1}); n != 0 || err != nil {
		t.Fatalf("resumed worker accepted unacknowledged PCM: %d, %v", n, err)
	}
	if err := worker.EndStream(3); err == nil {
		t.Fatal("accepted EOF while the skip control was in flight")
	}
	again, err := worker.StreamStatus(3)
	if err != nil || again.Control != first.Control || again.Skip != first.Skip || again.Held != first.Held {
		t.Fatalf("in-flight control changed: %+v, %v", again, err)
	}
	if err := worker.AckStreamControl(3, first.Control+1); err == nil {
		t.Fatal("accepted a control acknowledgement for the wrong generation")
	}
	if err := worker.AckStreamControl(3, first.Control); err != nil {
		t.Fatal(err)
	}
	if err := worker.AckStreamControl(3, first.Control); err == nil {
		t.Fatal("accepted the same acknowledgement twice")
	}
	if n, err := worker.WriteStream(3, []float32{2}); n != 0 || err != nil {
		t.Fatalf("accepted PCM before the resume control: %d, %v", n, err)
	}
	resume, err := worker.StreamStatus(3)
	if err != nil || resume.Control == 0 || resume.Control == first.Control || resume.Held || resume.Skip != 0 {
		t.Fatalf("missing resume control: %+v, %v", resume, err)
	}
	if err := worker.AckStreamControl(3, resume.Control); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.WriteStream(3, []float32{3, 4}); n != 2 || err != nil {
		t.Fatalf("reconciled worker refused PCM: %d, %v", n, err)
	}
	if err := worker.EndStream(3); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteSourcesAccumulatesSkipsBehindInFlightControl(t *testing.T) {
	cfg := Config{SampleRate: rate, Channels: 2, Voices: 4}
	local, _ := New(cfg)
	client, err := NewRemoteClient(local)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewRemoteSources(cfg, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RegisterStream(7, rate, 1, 8); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.WriteStream(7, make([]float32, 8)); n != 8 || err != nil {
		t.Fatalf("initial PCM: %d, %v", n, err)
	}
	if id := local.Play(PlayParams{Stream: NewStream(rate, 1, 8), Gain: 1}); !id.Valid() {
		t.Fatal("play refused")
	}
	if err := worker.Accept(sourcePacket(t, client, 7)); err != nil {
		t.Fatal(err)
	}
	if err := worker.Skip(20); err != nil {
		t.Fatal(err)
	}
	first, err := worker.StreamStatus(7)
	if err != nil || first.Control == 0 || first.Skip != 11 {
		t.Fatalf("first skip = %+v, %v", first, err)
	}
	if err := worker.Skip(10); err != nil {
		t.Fatal(err)
	}
	repeated, err := worker.StreamStatus(7)
	if err != nil || repeated.Control != first.Control || repeated.Skip != first.Skip {
		t.Fatalf("first control changed: %+v, %v", repeated, err)
	}
	if err := worker.AckStreamControl(7, first.Control); err != nil {
		t.Fatal(err)
	}
	second, err := worker.StreamStatus(7)
	if err != nil || second.Control == 0 || second.Control == first.Control || second.Skip != 10 || !second.Held {
		t.Fatalf("second skip was lost: %+v, %v", second, err)
	}
	if err := worker.AckStreamControl(7, second.Control); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteSourcesStreamSkipThenResumeMatchesNative(t *testing.T) {
	cfg := Config{SampleRate: rate, Channels: 2, Voices: 4, Block: 64}
	local, _ := New(cfg)
	reference, _ := New(cfg)
	client, err := NewRemoteClient(local)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewRemoteSources(cfg, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RegisterStream(9, rate, 1, 32); err != nil {
		t.Fatal(err)
	}
	src, ref := NewStream(rate, 1, 128), NewStream(rate, 1, 128)
	pcm := make([]float32, 128)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i) * .03))
	}
	for _, s := range []*Stream{src, ref} {
		if n := s.Ring.Write(pcm); n != len(pcm) {
			t.Fatalf("source wrote %d frames", n)
		}
		s.SetEOF()
	}
	sender, err := NewRemoteStreamSender(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sender.Close(); err != nil {
			t.Errorf("sender close: %v", err)
		}
	}()
	transfer := func() {
		t.Helper()
		buf := make([]float32, 32)
		n, eof, err := sender.Peek(buf)
		if err != nil || eof || n != 32 {
			t.Fatalf("peek: %d frames, eof=%v, %v", n, eof, err)
		}
		written, err := worker.WriteStream(9, buf[:n])
		if err != nil || written != n {
			t.Fatalf("write: %d frames, %v", written, err)
		}
		if err := sender.Ack(written); err != nil {
			t.Fatal(err)
		}
	}
	transfer()
	a := local.Play(PlayParams{Stream: src, Gain: .7})
	b := reference.Play(PlayParams{Stream: ref, Gain: .7})
	if a != b || !a.Valid() {
		t.Fatalf("voice IDs = %v, %v", a, b)
	}
	if err := worker.Accept(sourcePacket(t, client, 9)); err != nil {
		t.Fatal(err)
	}
	reference.Skip(50)
	if err := worker.Skip(50); err != nil {
		t.Fatal(err)
	}
	held, err := worker.StreamStatus(9)
	if err != nil || held.Control == 0 || !held.Held || held.Skip != 17 {
		t.Fatalf("skip status = %+v, %v", held, err)
	}
	if err := sender.Reconcile(held.Consumed, held.Buffered); err != nil {
		t.Fatal(err)
	}
	if err := sender.BeginControl(held.Skip, held.Held); err != nil {
		t.Fatal(err)
	}
	if err := worker.AckStreamControl(9, held.Control); err != nil {
		t.Fatal(err)
	}
	// One output frame releases the worker's hold but cannot advance a
	// starved stream; its resume must reach the sender before more PCM.
	if err := worker.Mix(make([]float32, 2)); err != nil {
		t.Fatal(err)
	}
	resume, err := worker.StreamStatus(9)
	if err != nil || resume.Control == 0 || resume.Held || resume.Skip != 0 {
		t.Fatalf("resume status = %+v, %v", resume, err)
	}
	if err := sender.Reconcile(resume.Consumed, resume.Buffered); err != nil {
		t.Fatal(err)
	}
	if err := sender.BeginControl(resume.Skip, resume.Held); err != nil {
		t.Fatal(err)
	}
	if err := worker.AckStreamControl(9, resume.Control); err != nil {
		t.Fatal(err)
	}
	transfer()
	want, got := make([]float32, 32), make([]float32, 32)
	reference.Mix(want)
	if err := worker.Mix(got); err != nil {
		t.Fatal(err)
	}
	for i := range want {
		if math.Float32bits(want[i]) != math.Float32bits(got[i]) {
			t.Fatalf("resumed sample %d = %v, native = %v", i, got[i], want[i])
		}
	}
}

func TestRemoteSourcesHeldStreamEOFReleasesVoiceWhileMuted(t *testing.T) {
	cfg := Config{SampleRate: rate, Channels: 2, Voices: 4}
	local, _ := New(cfg)
	client, err := NewRemoteClient(local)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewRemoteSources(cfg, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RegisterStream(4, rate, 1, 4); err != nil {
		t.Fatal(err)
	}
	src := NewStream(rate, 1, 8)
	if n := src.Ring.Write([]float32{1, 2, 3, 4, 5, 6, 7, 8}); n != 8 {
		t.Fatal(n)
	}
	src.SetEOF()
	sender, err := NewRemoteStreamSender(src)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4)
	n, eof, err := sender.Peek(buf)
	if err != nil || eof || n != 4 {
		t.Fatalf("initial transfer: %d frames, eof=%v, %v", n, eof, err)
	}
	written, err := worker.WriteStream(4, buf)
	if err != nil || written != n {
		t.Fatalf("worker write: %d, %v", written, err)
	}
	if err := sender.Ack(written); err != nil {
		t.Fatal(err)
	}
	id := local.Play(PlayParams{Stream: src, Gain: 1})
	if !id.Valid() {
		t.Fatal("play refused")
	}
	if err := worker.Accept(sourcePacket(t, client, 4)); err != nil {
		t.Fatal(err)
	}
	if err := worker.Skip(20); err != nil {
		t.Fatal(err)
	}
	control, err := worker.StreamStatus(4)
	if err != nil || control.Control == 0 || !control.Held || control.Skip == 0 {
		t.Fatalf("skip control: %+v, %v", control, err)
	}
	if err := sender.Reconcile(control.Consumed, control.Buffered); err != nil {
		t.Fatal(err)
	}
	if err := sender.BeginControl(control.Skip, control.Held); err != nil {
		t.Fatal(err)
	}
	if err := worker.AckStreamControl(4, control.Control); err != nil {
		t.Fatal(err)
	}
	if n, eof, err := sender.Peek(buf); err != nil || n != 0 || !eof {
		t.Fatalf("held stream EOF: %d frames, eof=%v, %v", n, eof, err)
	}
	if err := worker.EndStream(4); err != nil {
		t.Fatal(err)
	}
	if err := worker.Skip(1); err != nil {
		t.Fatal(err)
	}
	batch, err := worker.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	if local.Live(id) {
		t.Fatal("exhausted stream remained live while muted")
	}
	if ev, ok := local.Events().Pop(); !ok || ev.Kind != EvFinished || ev.Voice != id {
		t.Fatalf("finished event = %+v, %v", ev, ok)
	}
	if err := worker.Drop(4); err != nil {
		t.Fatal(err)
	}
	if err := sender.Close(); err != nil {
		t.Fatal(err)
	}
}
