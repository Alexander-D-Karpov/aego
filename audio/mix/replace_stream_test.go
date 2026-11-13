package mix

import (
	"math"
	"testing"
)

func TestReplaceStreamPreservesPausedVoiceAndOtherPlayback(t *testing.T) {
	for _, channels := range []int{1, 2} {
		t.Run(string(rune('0'+channels))+"ch", func(t *testing.T) {
			pcm := make([]float32, 128*channels)
			for i := range pcm {
				pcm[i] = float32(math.Sin(float64(i)*.13)) * .4
			}
			m, ref := newMixer(t), newMixer(t)
			old := m.Play(PlayParams{Memory: dc(.3, 256), Gain: 1})
			game := m.Play(PlayParams{Memory: dc(.1, 256), Gain: 1})
			refVoice := ref.Play(PlayParams{Memory: &Memory{PCM: pcm, Rate: rate / 2, Channels: channels}, Gain: 1})
			refGame := ref.Play(PlayParams{Memory: dc(.1, 256), Gain: 1})
			if !old.Valid() || !game.Valid() || !refVoice.Valid() || !refGame.Valid() || !m.Pause(old) || !ref.Pause(refVoice) {
				t.Fatal("could not prepare paused voice and game voice")
			}
			render(m, 11)
			render(ref, 11)
			stream := NewStream(rate/2, channels, 128)
			if stream.Ring.Write(pcm) != 128 {
				t.Fatal("could not prime the replacement stream")
			}
			replacement := m.ReplaceStream(old, PlayParams{Stream: stream, Gain: 1}, 1, true)
			if !replacement.Valid() || m.Started(replacement) || !m.Started(old) || !ref.SeekMemory(refVoice, 1) {
				t.Fatal("could not queue replacement and reference seek")
			}
			m.Mix(nil)
			ref.Mix(nil)
			if m.Live(old) || m.Started(old) || !m.Live(replacement) || !m.Started(replacement) || !m.Live(game) || m.Position(game) != ref.Position(refGame) {
				t.Fatal("replacement did not preserve voice ownership")
			}
			if pos := m.Position(replacement); pos != ref.Position(refVoice) {
				t.Fatalf("paused position = %v, reference %v", pos, ref.Position(refVoice))
			}
			if !m.Resume(replacement) || !ref.Resume(refVoice) {
				t.Fatal("could not resume replacement")
			}
			got, want := render(m, 32), render(ref, 32)
			for i := range got {
				if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
					t.Fatalf("sample %d = %v, reference %v", i, got[i], want[i])
				}
			}
			if m.Position(game) != ref.Position(refGame) {
				t.Fatal("replacement moved the game voice")
			}
		})
	}
}

func TestReplaceStreamRejectsWithoutStoppingOldVoice(t *testing.T) {
	for _, reason := range []string{"invalid-rate", "already-in-use", "stale-old", "invalid-frame"} {
		t.Run(reason, func(t *testing.T) {
			m := newMixer(t)
			oldStream := NewStream(rate, 1, 64)
			oldStream.Ring.Write(make([]float32, 64))
			old := m.Play(PlayParams{Stream: oldStream, Gain: 1})
			game := m.Play(PlayParams{Memory: dc(.1, 128), Gain: 1})
			m.Mix(nil)
			stream := NewStream(rate, 1, 64)
			previous := old
			frame := int64(0)
			switch reason {
			case "invalid-rate":
				stream.Rate = 0
			case "already-in-use":
				stream = oldStream
			case "stale-old":
				previous.Gen++
			case "invalid-frame":
				frame = 2
			}
			var id VoiceID
			if reason == "invalid-frame" {
				// Exercise consumer validation even for malformed remote commands.
				id = m.Reserve()
				if !m.Commands().Push(Command{Kind: CmdReplaceStream, Voice: id, Previous: previous, Stream: stream, Gain: 1, Frame: frame}) {
					t.Fatal("could not queue malformed command")
				}
			} else {
				id = m.ReplaceStream(previous, PlayParams{Stream: stream, Gain: 1}, frame, false)
			}
			if !id.Valid() {
				t.Fatal("replacement was not queued")
			}
			m.Mix(nil)
			if !m.Live(old) || !m.Started(old) || !m.Live(game) || m.Live(id) || m.Started(id) {
				t.Fatal("rejected replacement changed an existing voice or retained its handle")
			}
			if got := events(m); len(got) != 1 || got[0] != (Event{Kind: EvRejected, Voice: id}) {
				t.Fatalf("replacement events = %+v", got)
			}
		})
	}
}

func TestReplaceStreamRespectsQueueCapacityAndVoiceLimit(t *testing.T) {
	m, err := New(Config{SampleRate: rate, Voices: 2})
	if err != nil {
		t.Fatal(err)
	}
	old := m.Play(PlayParams{Memory: dc(.3, 128), Gain: 1})
	game := m.Play(PlayParams{Memory: dc(.1, 128), Gain: 1})
	m.Mix(nil)
	for i := 0; i < queueSize; i++ {
		if !m.SetBusGain(0, 1) {
			t.Fatal("queue filled too early")
		}
	}
	stream := NewStream(rate, 1, 64)
	stream.Ring.Write(make([]float32, 64))
	if got := m.ReplaceStream(old, PlayParams{Stream: stream, Gain: 1}, 0, false); got.Valid() {
		t.Fatal("replacement bypassed queue backpressure")
	}
	m.Mix(nil)
	newVoice := m.ReplaceStream(old, PlayParams{Stream: stream, Gain: 1}, 0, false)
	if !newVoice.Valid() {
		t.Fatal("could not retry replacement")
	}
	m.Mix(nil)
	if m.Live(old) || !m.Live(newVoice) || !m.Live(game) {
		t.Fatal("replacement stole a game voice at the voice limit")
	}
}

func TestRemoteReplaceStreamMatchesLocalMixer(t *testing.T) {
	cfg := Config{SampleRate: rate, Voices: 2, Block: 64}
	local, reference, client, remote := remotePair(t, cfg)
	params := PlayParams{Memory: dc(.3, 1024), Gain: 1}
	old, wantOld := local.Play(params), reference.Play(params)
	game, wantGame := local.Play(PlayParams{Memory: dc(.1, 1024), Gain: 1}), reference.Play(PlayParams{Memory: dc(.1, 1024), Gain: 1})
	if old != wantOld || game != wantGame {
		t.Fatal("voice IDs differ")
	}
	compareRemoteBlock(t, local, reference, client, remote, 16)
	a, b := NewStream(rate, 1, 128), NewStream(rate, 1, 128)
	pcm := make([]float32, 128)
	for i := range pcm {
		pcm[i] = float32(i) / 512
	}
	a.Ring.Write(pcm)
	b.Ring.Write(pcm)
	id := local.ReplaceStream(old, PlayParams{Stream: a, Gain: 1}, 1, false)
	wantID := reference.ReplaceStream(wantOld, PlayParams{Stream: b, Gain: 1}, 1, false)
	if id != wantID || !id.Valid() {
		t.Fatal("replacement IDs differ")
	}
	compareRemoteBlock(t, local, reference, client, remote, 32)
	if local.Live(old) || !local.Live(game) || !local.Live(id) || local.Position(id) != reference.Position(wantID) {
		t.Fatal("remote replacement diverged from local state")
	}
}
