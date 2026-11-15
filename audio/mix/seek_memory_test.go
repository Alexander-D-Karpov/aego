package mix

import (
	"math"
	"testing"
)

func TestSeekMemoryRendersRequestedFrameWithoutMovingOtherVoices(t *testing.T) {
	pcm := make([]float32, 256)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i)*.13)) * .4
	}
	src := &Memory{PCM: pcm, Rate: rate, Channels: 1}
	game := dc(.1, 256)
	m, reference := newMixer(t), newMixer(t)
	preview := m.Play(PlayParams{Memory: src, Gain: 1})
	// Match voice order so each mixer performs the same floating-point operations.
	wantPreview := reference.Play(PlayParams{Memory: &Memory{PCM: pcm[79:], Rate: rate, Channels: 1}, Gain: 1})
	other := m.Play(PlayParams{Memory: game, Gain: 1})
	wantOther := reference.Play(PlayParams{Memory: game, Gain: 1})
	if !preview.Valid() || !wantPreview.Valid() || !other.Valid() || !wantOther.Valid() ||
		!m.Pause(preview) || !reference.Pause(wantPreview) {
		t.Fatal("could not prepare voices")
	}
	render(m, 11)
	render(reference, 11)
	if !m.SeekMemory(preview, 79) {
		t.Fatal("seek was not queued")
	}
	m.Mix(nil) // Process a seek even when no audio frames are requested.
	if pos := m.Position(preview); pos != float32(79)/rate {
		t.Fatalf("paused seek position = %v", pos)
	}
	if m.Position(other) != reference.Position(wantOther) {
		t.Fatal("seeking the preview moved another voice")
	}
	if !m.Resume(preview) || !reference.Resume(wantPreview) {
		t.Fatal("could not resume the comparison")
	}
	got, want := render(m, 32), render(reference, 32)
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("sample %d: got %v, want %v", i, got[i], want[i])
		}
	}
	if m.Position(other) != reference.Position(wantOther) {
		t.Fatal("another voice diverged after preview resumed")
	}
}

func TestSeekMemoryOrderBoundsLoopAndStaleVoice(t *testing.T) {
	m := newMixer(t)
	src := dc(.2, 64)
	src.LoopStart, src.LoopEnd = 8, 48
	id := m.Play(PlayParams{Memory: src, Gain: 1, Loop: true})
	if !id.Valid() || !m.Pause(id) || !m.SeekMemory(id, 47) {
		t.Fatal("could not queue play, pause and seek")
	}
	m.Mix(nil)
	vi := m.find(id)
	if vi < 0 || m.voices[vi].state != voicePaused || m.voices[vi].pos != 47 {
		t.Fatal("queued seek did not follow play and pause")
	}
	for _, frame := range []int64{-1, 48, 64, 1<<53 + 1} {
		// Raw commands are also guarded on the audio thread, including those
		// received from a remote worker transport.
		if !m.Commands().Push(Command{Kind: CmdSeekMemory, Voice: id, Frame: frame}) {
			t.Fatal("could not queue invalid command")
		}
		m.Mix(nil)
		if m.voices[vi].pos != 47 {
			t.Fatalf("invalid frame %d moved the voice", frame)
		}
	}
	for _, frame := range []int64{-1, 1<<53 + 1} {
		if m.SeekMemory(id, frame) {
			t.Fatalf("producer accepted invalid frame %d", frame)
		}
	}
	if !m.Resume(id) {
		t.Fatal("could not resume")
	}
	render(m, 2)
	if m.voices[vi].pos != 9 {
		t.Fatalf("seek did not preserve loop wrapping: %v", m.voices[vi].pos)
	}
	if !m.Stop(id, 0) || !m.SeekMemory(id, 3) {
		t.Fatal("could not queue stop and seek")
	}
	m.Mix(nil)
	if m.voices[vi].pos != 9 {
		t.Fatal("seek moved a stopping voice")
	}
	if !m.Kill(id) {
		t.Fatal("could not queue release")
	}
	m.Mix(nil)
	if !m.SeekMemory(id, 4) {
		t.Fatal("could not queue stale voice command")
	}
	m.Mix(nil)
	if m.Live(id) {
		t.Fatal("stale seek revived a released voice")
	}
}

func TestSeekMemoryDoesNotMutateStreamOrBypassBackpressure(t *testing.T) {
	m := newMixer(t)
	stream := NewStream(rate, 1, 64)
	stream.Ring.Write([]float32{.1, .2, .3, .4})
	id := m.Play(PlayParams{Stream: stream, Gain: 1})
	m.Mix(nil)
	vi := m.find(id)
	if vi < 0 || !m.SeekMemory(id, 3) {
		t.Fatal("could not queue stream negative control")
	}
	m.Mix(nil)
	if m.voices[vi].pos != 0 || stream.Ring.Len() != 4 || stream.TakeSkip() != 0 {
		t.Fatal("memory-only seek changed the stream")
	}
	for i := 0; i < queueSize; i++ {
		if !m.SetBusGain(0, 1) {
			t.Fatal("queue filled too early")
		}
	}
	if m.SeekMemory(id, 1) {
		t.Fatal("seek bypassed the bounded command queue")
	}
	if m.SeekMemory(VoiceID{}, 0) {
		t.Fatal("accepted an invalid voice ID")
	}
}
