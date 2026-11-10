package mix

import (
	"math"
	"math/rand"
	"testing"
)

func remotePair(t *testing.T, cfg Config) (*Mixer, *Mixer, *RemoteClient, *RemoteMixer) {
	t.Helper()
	local, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewRemoteClient(local)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := NewRemoteMixer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return local, reference, client, remote
}

func transferRemote(t *testing.T, c *RemoteClient, r *RemoteMixer) {
	t.Helper()
	for {
		command, ok, err := c.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return
		}
		if err := r.Accept(command); err != nil {
			t.Fatal(err)
		}
	}
}

func compareRemoteBlock(t *testing.T, local, reference *Mixer, c *RemoteClient, r *RemoteMixer, frames int) {
	t.Helper()
	transferRemote(t, c, r)
	want, got := make([]float32, frames*2), make([]float32, frames*2)
	reference.Mix(want)
	if err := r.Mix(got); err != nil {
		t.Fatal(err)
	}
	for i := range want {
		if math.Float32bits(want[i]) != math.Float32bits(got[i]) {
			t.Fatalf("sample %d: remote %v, reference %v", i, got[i], want[i])
		}
	}
	batch, err := r.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(batch); err != nil {
		t.Fatal(err)
	}
	for {
		a, aok := local.Events().Pop()
		b, bok := reference.Events().Pop()
		if aok != bok || a != b {
			t.Fatalf("remote event %v/%v, reference %v/%v", a, aok, b, bok)
		}
		if !aok {
			break
		}
	}
	if local.Active() != reference.Active() {
		t.Fatalf("remote active %d, reference %d", local.Active(), reference.Active())
	}
}

func TestRemoteMixerMatchesCommandsAndVoiceState(t *testing.T) {
	cfg := Config{SampleRate: rate, Channels: 2, Voices: 2, Block: 64}
	local, reference, client, remote := remotePair(t, cfg)
	src := sine(440, rate, 1, .5)
	play := func(priority int32) VoiceID {
		p := PlayParams{Memory: src, Gain: .7, Pan: -.2, Pitch: 1.1, Priority: priority, Loop: true}
		a, b := local.Play(p), reference.Play(p)
		if a != b || !a.Valid() {
			t.Fatalf("IDs remote=%v reference=%v", a, b)
		}
		return a
	}
	a := play(1)
	compareRemoteBlock(t, local, reference, client, remote, 128)
	if local.Position(a) != reference.Position(a) || local.VoiceGain(a) != reference.VoiceGain(a) {
		t.Fatalf("voice state remote=(%v,%v), reference=(%v,%v)", local.Position(a), local.VoiceGain(a), reference.Position(a), reference.VoiceGain(a))
	}
	local.SetBusGain(0, .4)
	reference.SetBusGain(0, .4)
	local.SetPan(a, .8)
	reference.SetPan(a, .8)
	local.SetGain(a, .2, .01)
	reference.SetGain(a, .2, .01)
	compareRemoteBlock(t, local, reference, client, remote, 256)
	if local.BusGain(0) != reference.BusGain(0) {
		t.Fatalf("bus gain remote=%v reference=%v", local.BusGain(0), reference.BusGain(0))
	}
	b := play(1)
	compareRemoteBlock(t, local, reference, client, remote, 64)
	_ = play(1) // The real mixer steals one voice; remote IDs and events must follow it.
	compareRemoteBlock(t, local, reference, client, remote, 64)
	if local.Live(a) != reference.Live(a) || local.Live(b) != reference.Live(b) {
		t.Fatal("voice liveness diverged after stealing")
	}
	local.PauseAll()
	reference.PauseAll()
	compareRemoteBlock(t, local, reference, client, remote, 64)
	local.ResumeAll()
	reference.ResumeAll()
	compareRemoteBlock(t, local, reference, client, remote, 64)
}

func TestRemoteBackpressureIncludesCommandsInFlight(t *testing.T) {
	local, _, client, remote := remotePair(t, Config{SampleRate: rate})
	for i := 0; i < queueSize; i++ {
		if !local.SetBusGain(0, 1) {
			t.Fatalf("command %d refused early", i)
		}
	}
	transferRemote(t, client, remote)
	if local.SetBusGain(0, 1) {
		t.Fatal("commands in transit bypassed the queue limit")
	}
	if local.Commands().Dropped() != 1 {
		t.Fatal("the refused command was not counted")
	}
	if err := remote.Mix(make([]float32, 2)); err != nil {
		t.Fatal(err)
	}
	batch, err := remote.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	if !local.SetBusGain(0, 1) {
		t.Fatal("processed commands did not release queue capacity")
	}
}

func TestRemoteReleasesRemainOrderedWhenEventsDrop(t *testing.T) {
	local, reference, client, remote := remotePair(t, Config{SampleRate: rate, Voices: 4, Block: 64})
	// Simulate the existing bounded event queue filling. Voice reclamation is
	// still required even when clients cannot receive every event.
	reference.events.q = newSPSC[Event](8)
	remote.m.events.q = newSPSC[Event](8)
	src := dc(.25, rate)
	for i := 0; i < len(local.handles); i++ {
		p := PlayParams{Memory: src, Gain: 1, Loop: true}
		if a, b := local.Play(p), reference.Play(p); a != b || !a.Valid() {
			t.Fatalf("initial voice %d: %v vs %v", i, a, b)
		}
	}
	compareRemoteBlock(t, local, reference, client, remote, 64)
	if local.Events().Dropped() == 0 || local.Events().Dropped() != reference.Events().Dropped() {
		t.Fatalf("dropped events remote=%d reference=%d", local.Events().Dropped(), reference.Events().Dropped())
	}
	for i := 0; i < len(local.handles)-4; i++ {
		p := PlayParams{Memory: src, Gain: 1, Loop: true}
		if a, b := local.Play(p), reference.Play(p); a != b || !a.Valid() {
			t.Fatalf("reused voice %d: remote %v, reference %v", i, a, b)
		}
	}
	compareRemoteBlock(t, local, reference, client, remote, 64)
}

func TestRemoteRetainsReleasesAcrossUndrainedRenders(t *testing.T) {
	local, reference, client, remote := remotePair(t, Config{SampleRate: rate, Voices: 4, Block: 64})
	src := dc(.25, 128)
	ids := make([]VoiceID, 0, 16)
	for i := 0; i < 16; i++ {
		params := PlayParams{Memory: src, Gain: 1}
		a, b := local.Play(params), reference.Play(params)
		if a != b || !a.Valid() {
			t.Fatalf("voice %d = %v, reference %v", i, a, b)
		}
		ids = append(ids, a)
	}
	transferRemote(t, client, remote)
	for i := 0; i < 600; i++ {
		want, got := make([]float32, 128), make([]float32, 128)
		reference.Mix(want)
		if err := remote.Mix(got); err != nil {
			t.Fatal(err)
		}
		for j := range want {
			if math.Float32bits(got[j]) != math.Float32bits(want[j]) {
				t.Fatalf("block %d sample %d = %v, reference %v", i, j, got[j], want[j])
			}
		}
	}
	batch, err := remote.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Released) != len(ids) || batch.Active != 0 {
		t.Fatalf("undrained release ledger = %+v", batch)
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if local.Live(id) || reference.Live(id) {
			t.Fatalf("voice %v leaked while batches were withheld", id)
		}
	}
	for {
		a, aok := local.Events().Pop()
		b, bok := reference.Events().Pop()
		if a != b || aok != bok {
			t.Fatalf("worker event %v/%v, reference %v/%v", a, aok, b, bok)
		}
		if !aok {
			break
		}
	}
}

func TestRemoteReleaseLedgerCoversConfiguredVoiceCapacity(t *testing.T) {
	const voices = 2050 // More than twice the command queue capacity.
	m, _ := New(Config{SampleRate: rate, Voices: voices})
	client, err := NewRemoteClient(m)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := NewRemoteMixer(Config{SampleRate: rate, Voices: voices})
	if err != nil {
		t.Fatal(err)
	}
	src := dc(.2, 128)
	for left := voices; left > 0; {
		n := min(left, queueSize)
		for i := 0; i < n; i++ {
			if id := m.Play(PlayParams{Memory: src, Gain: 1}); !id.Valid() {
				t.Fatalf("play refused with %d voices left", left-i)
			}
		}
		transferRemote(t, client, remote)
		if err := remote.Mix(make([]float32, 2)); err != nil {
			t.Fatal(err)
		}
		batch, err := remote.Drain()
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Apply(batch); err != nil {
			t.Fatal(err)
		}
		left -= n
	}
	if m.Active() != voices {
		t.Fatalf("active voices = %d, want %d", m.Active(), voices)
	}
	if err := remote.Skip(1024); err != nil {
		t.Fatal(err)
	}
	batch, err := remote.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Released) != voices {
		t.Fatalf("release ledger has %d entries, want %d", len(batch.Released), voices)
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	if m.Active() != 0 {
		t.Fatalf("active voices after skip = %d", m.Active())
	}
}

func TestRemoteRejectsOutOfOrderAndInvalidAcknowledgements(t *testing.T) {
	local, _, client, remote := remotePair(t, Config{SampleRate: rate})
	if !local.SetBusGain(0, 1) {
		t.Fatal("command refused")
	}
	command, ok, err := client.Next()
	if err != nil || !ok {
		t.Fatalf("next: %v %v", ok, err)
	}
	bad := command
	bad.Sequence++
	if err := remote.Accept(bad); err == nil {
		t.Fatal("out-of-order command accepted")
	}
	if err := client.Apply(RemoteBatch{Applied: command.Sequence + 1}); err == nil {
		t.Fatal("unissued acknowledgement accepted")
	}
}

func TestRemoteSkipMatchesLocalAndReleasesVoice(t *testing.T) {
	local, reference, client, remote := remotePair(t, Config{SampleRate: rate})
	p := PlayParams{Memory: dc(.2, 100), Gain: 1}
	id, wantID := local.Play(p), reference.Play(p)
	if id != wantID || !id.Valid() {
		t.Fatalf("IDs %v vs %v", id, wantID)
	}
	transferRemote(t, client, remote)
	reference.Skip(256)
	if err := remote.Skip(256); err != nil {
		t.Fatal(err)
	}
	batch, err := remote.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	a, aok := local.Events().Pop()
	b, bok := reference.Events().Pop()
	if a != b || aok != bok || !aok || local.Live(id) || reference.Live(id) {
		t.Fatalf("skip event remote=%v/%v reference=%v/%v; live=%v/%v", a, aok, b, bok, local.Live(id), reference.Live(id))
	}
}

func TestRemoteSampleRateChangePreservesPlayingVoice(t *testing.T) {
	cfg := Config{SampleRate: rate, Voices: 4, Block: 64}
	local, reference, client, remote := remotePair(t, cfg)
	src := sine(440, 2048, 1, .4)
	p := PlayParams{Memory: src, Gain: .6, Pitch: 1.1, Loop: true}
	a, b := local.Play(p), reference.Play(p)
	if a != b || !a.Valid() {
		t.Fatalf("voice IDs: %v vs %v", a, b)
	}
	compareRemoteBlock(t, local, reference, client, remote, 128)
	local.SetSampleRate(44100)
	reference.SetSampleRate(44100)
	if err := remote.SetSampleRate(44100); err != nil {
		t.Fatal(err)
	}
	compareRemoteBlock(t, local, reference, client, remote, 128)
	if local.SampleRate() != 44100 || local.Position(a) != reference.Position(b) || !local.Live(a) {
		t.Fatalf("rate change lost voice state: local=%v reference=%v", local.Position(a), reference.Position(b))
	}
	if err := remote.SetSampleRate(0); err == nil {
		t.Fatal("invalid remote rate accepted")
	}
}

func TestRemoteBatchValidationIsAtomicAndRejectsReplay(t *testing.T) {
	local, _, client, remote := remotePair(t, Config{SampleRate: rate})
	id := local.Play(PlayParams{Memory: dc(.2, rate), Gain: 1, Loop: true})
	transferRemote(t, client, remote)
	if err := remote.Mix(make([]float32, 128)); err != nil {
		t.Fatal(err)
	}
	batch, err := remote.Drain()
	if err != nil {
		t.Fatal(err)
	}
	bad := batch
	bad.Released = []VoiceID{{Index: id.Index, Gen: id.Gen + 1}}
	if err := client.Apply(bad); err == nil {
		t.Fatal("unknown generation accepted")
	}
	if !local.Live(id) || local.Active() != 0 {
		t.Fatal("invalid batch changed local state")
	}
	if err := client.Apply(batch); err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(batch); err == nil {
		t.Fatal("replayed batch accepted")
	}
	if !local.Live(id) || local.Active() != 1 {
		t.Fatal("valid batch did not preserve the live voice")
	}
}

func TestRemoteHandleMismatchFailsClosed(t *testing.T) {
	local, _, client, remote := remotePair(t, Config{SampleRate: rate})
	local.Play(PlayParams{Memory: dc(.2, rate), Gain: 1})
	packet, ok, err := client.Next()
	if err != nil || !ok {
		t.Fatalf("next: %v %v", ok, err)
	}
	packet.Command.Voice.Index++
	if err := remote.Accept(packet); err == nil {
		t.Fatal("mismatched handle accepted")
	}
	if err := remote.Mix(make([]float32, 2)); err == nil {
		t.Fatal("broken remote mixer continued rendering")
	}
	if _, err := remote.Drain(); err == nil {
		t.Fatal("broken remote mixer reported valid state")
	}
}

func TestRemoteClientRequiresUnrenderedMixer(t *testing.T) {
	m, err := New(Config{SampleRate: rate})
	if err != nil {
		t.Fatal(err)
	}
	m.Skip(1)
	if _, err := NewRemoteClient(m); err == nil {
		t.Fatal("attached to an already advanced mixer")
	}
	m, err = New(Config{SampleRate: rate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRemoteClient(m); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRemoteClient(m); err == nil {
		t.Fatal("attached two remote consumers")
	}
}

func TestRemoteCancelUnsentRestoresLocalQueueAndVoice(t *testing.T) {
	cfg := Config{SampleRate: rate, Voices: 4, Block: 64}
	local, _ := New(cfg)
	reference, _ := New(cfg)
	src := sine(440, 256, 1, .4)
	params := PlayParams{Memory: src, Gain: .6, Loop: true}
	id, wantID := local.Play(params), reference.Play(params)
	client, err := NewRemoteClient(local)
	if err != nil {
		t.Fatal(err)
	}
	if id != wantID || !local.SetGain(id, .3, 0) || !reference.SetGain(wantID, .3, 0) {
		t.Fatal("could not prepare matching queues")
	}
	if err := client.CancelUnsent(); err != nil {
		t.Fatal(err)
	}
	if err := client.CancelUnsent(); err == nil {
		t.Fatal("cancelled the same client twice")
	}
	if _, _, err := client.Next(); err == nil {
		t.Fatal("cancelled client continued consuming")
	}
	got, want := make([]float32, 128), make([]float32, 128)
	local.Mix(got)
	reference.Mix(want)
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("sample %d = %v, reference %v", i, got[i], want[i])
		}
	}
	if !local.Live(id) || local.Active() != reference.Active() || local.Position(id) != reference.Position(wantID) ||
		local.VoiceGain(id) != reference.VoiceGain(wantID) {
		t.Fatal("cancelling startup lost or changed the queued voice")
	}
	if !local.SetPan(id, .5) {
		t.Fatal("queue remained bound to the cancelled remote client")
	}
}

func TestRemoteCancelUnsentRefusesTransferredState(t *testing.T) {
	for _, afterBatch := range []bool{false, true} {
		m, _ := New(Config{SampleRate: rate})
		client, err := NewRemoteClient(m)
		if err != nil {
			t.Fatal(err)
		}
		if afterBatch {
			remote, err := NewRemoteMixer(Config{SampleRate: rate})
			if err != nil {
				t.Fatal(err)
			}
			batch, err := remote.Drain()
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Apply(batch); err != nil {
				t.Fatal(err)
			}
		} else {
			m.Play(PlayParams{Memory: dc(.2, 64), Gain: 1})
			if _, ok, err := client.Next(); err != nil || !ok {
				t.Fatalf("next command: %v, %v", ok, err)
			}
		}
		if err := client.CancelUnsent(); err == nil {
			t.Fatalf("cancelled after state crossed the transport, afterBatch=%v", afterBatch)
		}
		if _, err := NewRemoteClient(m); err == nil {
			t.Fatal("failed cancellation released the existing client")
		}
	}
}

func TestRemoteSeededCommandSequenceMatchesLocal(t *testing.T) {
	local, reference, client, remote := remotePair(t, Config{SampleRate: rate, Voices: 4, Block: 64})
	rng := rand.New(rand.NewSource(426))
	src := sine(440, 128, 1, .2)
	var ids []VoiceID
	for tick := 0; tick < 600; tick++ {
		for n := 0; n < 1+rng.Intn(8); n++ {
			op := rng.Intn(10)
			if op < 3 || len(ids) == 0 {
				p := PlayParams{Memory: src, Gain: .2 + rng.Float32(), Pitch: .5 + rng.Float32(),
					Pan: rng.Float32()*2 - 1, Priority: int32(rng.Intn(3)), Loop: rng.Intn(2) == 0}
				a, b := local.Play(p), reference.Play(p)
				if a != b {
					t.Fatalf("tick %d IDs remote=%v reference=%v", tick, a, b)
				}
				if a.Valid() {
					ids = append(ids, a)
				}
				continue
			}
			id := ids[rng.Intn(len(ids))]
			var a, b bool
			switch op {
			case 3:
				a, b = local.Stop(id, 0), reference.Stop(id, 0)
			case 4:
				a, b = local.Pause(id), reference.Pause(id)
			case 5:
				a, b = local.Resume(id), reference.Resume(id)
			case 6:
				gain := rng.Float32()
				a, b = local.SetGain(id, gain, .01), reference.SetGain(id, gain, .01)
			case 7:
				pan := rng.Float32()*2 - 1
				a, b = local.SetPan(id, pan), reference.SetPan(id, pan)
			case 8:
				pitch := .5 + rng.Float32()
				a, b = local.SetPitch(id, pitch), reference.SetPitch(id, pitch)
			default:
				on := rng.Intn(2) == 0
				a, b = local.SetBusMute(0, on), reference.SetBusMute(0, on)
			}
			if a != b {
				t.Fatalf("tick %d operation %d accepted=%v vs %v", tick, op, a, b)
			}
		}
		compareRemoteBlock(t, local, reference, client, remote, 64)
		for _, id := range ids {
			if local.Live(id) != reference.Live(id) || local.Position(id) != reference.Position(id) || local.VoiceGain(id) != reference.VoiceGain(id) {
				t.Fatalf("tick %d: voice %v diverged", tick, id)
			}
		}
	}
}

func TestRemoteStreamMixerMatchesLocalWithTransferredPCM(t *testing.T) {
	local, reference, client, remote := remotePair(t, Config{SampleRate: rate, Voices: 4, Block: 64})
	clientStream := NewStream(rate, 2, 2048)
	referenceStream := NewStream(rate, 2, 2048)
	workerStream := NewStream(rate, 2, 2048)
	samples := make([]float32, 2048)
	for i := range samples {
		samples[i] = float32(math.Sin(float64(i)*.03) * .3)
	}
	for _, s := range []*Stream{referenceStream, workerStream} {
		if n := s.Ring.Write(samples); n != len(samples)/2 {
			t.Fatalf("stream accepted %d frames", n)
		}
		s.SetEOF()
	}
	a := local.Play(PlayParams{Stream: clientStream, Gain: .7})
	b := reference.Play(PlayParams{Stream: referenceStream, Gain: .7})
	if a != b || !a.Valid() {
		t.Fatalf("stream IDs %v vs %v", a, b)
	}
	packet, ok, err := client.Next()
	if err != nil || !ok || packet.Command.Stream != clientStream {
		t.Fatalf("stream command: %v %v", ok, err)
	}
	// A browser transport owns a separate stream/ring in its worker; Go
	// pointers must never be sent across the worker boundary.
	packet.Command.Stream = workerStream
	if err := remote.Accept(packet); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		compareRemoteBlock(t, local, reference, client, remote, 64)
	}
	if local.Live(a) || reference.Live(a) {
		t.Fatal("finished stream retained its voice")
	}
}
