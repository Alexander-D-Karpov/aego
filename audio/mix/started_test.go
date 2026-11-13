package mix

import "testing"

func TestStartedReplacementWaitsForInitialPosition(t *testing.T) {
	m := newMixer(t)
	id := m.Reserve()
	stream := NewStream(rate, 1, 64)
	// replaceStream initializes the voice with play before it sets the stream
	// offset and pause state. An owner on another thread must not observe this
	// intermediate state as an applied replacement.
	m.play(&Command{Kind: CmdReplaceStream, Voice: id, Stream: stream, Gain: 1, Pitch: 1, Frame: 1, Paused: true})
	if m.find(id) < 0 || m.Started(id) {
		t.Fatal("replacement was published before its initial position and pause state")
	}
}

func TestStartedDistinguishesQueuedPausedReleasedAndRejected(t *testing.T) {
	m := newMixer(t)
	id := m.Play(PlayParams{Memory: dc(.2, 64), Gain: 1})
	if !id.Valid() || !m.Live(id) || m.Started(id) || m.Started(VoiceID{}) {
		t.Fatal("queued voice was reported as started")
	}
	if !m.Pause(id) {
		t.Fatal("could not queue pause")
	}
	m.Mix(nil)
	if !m.Started(id) {
		t.Fatal("applied paused voice was not reported as started")
	}
	if !m.Kill(id) {
		t.Fatal("could not queue release")
	}
	m.Mix(nil)
	if m.Live(id) || m.Started(id) {
		t.Fatal("released handle remained started")
	}
	bad := m.Play(PlayParams{Memory: &Memory{Rate: 0, Channels: 1, PCM: []float32{1}}, Gain: 1})
	if !bad.Valid() || m.Started(bad) {
		t.Fatal("could not queue rejected voice")
	}
	m.Mix(nil)
	if m.Live(bad) || m.Started(bad) {
		t.Fatal("rejected voice remained started")
	}
}

func TestRemoteStartedRequiresValidatedBatch(t *testing.T) {
	local, _, client, remote := remotePair(t, Config{SampleRate: rate})
	id := local.Play(PlayParams{Memory: dc(.2, 128), Gain: 1})
	if !id.Valid() || local.Started(id) {
		t.Fatal("queued remote voice was reported as started")
	}
	transferRemote(t, client, remote)
	if err := remote.Mix(nil); err != nil {
		t.Fatal(err)
	}
	if local.Started(id) {
		t.Fatal("remote state was visible before a validated batch")
	}
	batch, err := remote.Drain()
	if err != nil {
		t.Fatal(err)
	}
	bad := batch
	bad.Serial++
	if err := client.Apply(bad); err == nil || local.Started(id) {
		t.Fatal("invalid batch published voice state")
	}
	if err := client.Apply(batch); err != nil || !local.Started(id) {
		t.Fatal("validated batch did not publish started voice", err)
	}
	if !local.Kill(id) {
		t.Fatal("could not queue remote release")
	}
	transferRemote(t, client, remote)
	if err := remote.Mix(nil); err != nil {
		t.Fatal(err)
	}
	if !local.Started(id) {
		t.Fatal("unacknowledged release changed producer state")
	}
	batch, err = remote.Drain()
	if err != nil || client.Apply(batch) != nil || local.Started(id) || local.Live(id) {
		t.Fatalf("release batch did not clear state: %+v, %v", batch, err)
	}
}
