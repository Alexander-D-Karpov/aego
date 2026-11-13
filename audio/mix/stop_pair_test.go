package mix

import "testing"

func TestStopPairDuringPendingReplacement(t *testing.T) {
	for _, invalidReplacement := range []bool{false, true} {
		name := "replacement-accepted"
		if invalidReplacement {
			name = "replacement-rejected"
		}
		t.Run(name, func(t *testing.T) {
			m, err := New(Config{SampleRate: rate, Voices: 2})
			if err != nil {
				t.Fatal(err)
			}
			old := m.Play(PlayParams{Memory: dc(.3, 1024), Gain: 1})
			game := m.Play(PlayParams{Memory: dc(.1, 1024), Gain: 1})
			if !old.Valid() || !game.Valid() || !m.Pause(old) {
				t.Fatal("could not prepare voices")
			}
			m.Mix(nil)
			stream := NewStream(rate, 1, 64)
			if invalidReplacement {
				stream.Rate = 0
			}
			next := m.ReplaceStream(old, PlayParams{Stream: stream, Gain: 1}, 0, true)
			if !next.Valid() || !m.StopPair(old, next, 0) {
				t.Fatal("could not queue replacement and stop")
			}
			m.Mix(nil)
			if m.Live(old) || m.Live(next) || !m.Live(game) || !m.Started(game) {
				t.Fatal("stopping the pending pair lost or retained the wrong voice")
			}
		})
	}
}

func TestStopPairRespectsBackpressureAndStaleIDs(t *testing.T) {
	m := newMixer(t)
	old := m.Play(PlayParams{Memory: dc(.3, 4096), Gain: 1})
	game := m.Play(PlayParams{Memory: dc(.1, 4096), Gain: 1})
	m.Mix(nil)
	for i := 0; i < queueSize; i++ {
		if !m.SetBusGain(0, 1) {
			t.Fatal("queue filled too early")
		}
	}
	stale := old
	stale.Gen++
	if m.StopPair(old, stale, 0) || !m.Live(old) || !m.Live(game) {
		t.Fatal("queue-full stop changed the voice state")
	}
	m.Mix(nil)
	if !m.StopPair(old, stale, 0) {
		t.Fatal("could not retry pair stop")
	}
	render(m, 1024)
	if m.Live(old) || !m.Live(game) || m.StopPair(VoiceID{}, VoiceID{}, 0) {
		t.Fatal("stale pair stop changed an unrelated voice")
	}
}

func TestRemoteStopPairMatchesLocalMixer(t *testing.T) {
	local, reference, client, remote := remotePair(t, Config{SampleRate: rate, Voices: 3, Block: 64})
	p := PlayParams{Memory: dc(.1, 4096), Gain: 1}
	a, b, game := local.Play(p), local.Play(p), local.Play(p)
	wa, wb, wantGame := reference.Play(p), reference.Play(p), reference.Play(p)
	if a != wa || b != wb || game != wantGame || !local.Pause(a) || !reference.Pause(wa) {
		t.Fatal("could not prepare matching voices")
	}
	compareRemoteBlock(t, local, reference, client, remote, 16)
	if !local.StopPair(a, b, 0) || !reference.StopPair(wa, wb, 0) {
		t.Fatal("could not queue pair stop")
	}
	compareRemoteBlock(t, local, reference, client, remote, 1024)
	if local.Live(a) || local.Live(b) || !local.Live(game) || !reference.Live(wantGame) {
		t.Fatal("remote pair stop did not match local voice lifetime")
	}
}
