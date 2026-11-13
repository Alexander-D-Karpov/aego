package main

import (
	"math"
	"testing"

	"aego/audio/mix"
	"aego/audio/workerproto"
)

// The actual service's JSON decoder, source registry and mixer all participate;
// this does not attach a browser worker or physical output.
func TestServiceMemorySeekRendersRequestedFrame(t *testing.T) {
	s := initService(t, 1<<20)
	pcm := make([]float32, 128)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i)*.11)) * .5
	}
	call(t, s, opBeginMemory, request{Source: 7, Rate: 48000, Channels: 1, Samples: len(pcm)})
	copy(s.memory, pcm)
	call(t, s, opCommitMemory, request{Source: 7})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1,
		Source: 7, GainBits: math.Float32bits(1), PitchBits: math.Float32bits(1)}})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 2, Kind: mix.CmdPause, Gen: 1}})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 3, Kind: mix.CmdSeekMemory, Gen: 1, Frame: 53}})
	call(t, s, opSkip, request{Frames: 0})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 4, Kind: mix.CmdResume, Gen: 1}})
	got, err := s.render(16)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Voices: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !ref.Play(mix.PlayParams{Memory: &mix.Memory{PCM: pcm[53:], Rate: 48000, Channels: 1}, Gain: 1}).Valid() {
		t.Fatal("could not play reference PCM")
	}
	want := make([]float32, len(got))
	ref.Mix(want)
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("sample %d: worker=%v reference=%v", i, got[i], want[i])
		}
	}
}

func TestServiceReplaceStreamPreservesPrecedingSampleAndLifetime(t *testing.T) {
	s := initService(t, 1<<20)
	call(t, s, opBeginMemory, request{Source: 1, Rate: 48000, Channels: 1, Samples: 128})
	for i := range s.memory {
		s.memory[i] = .2
	}
	call(t, s, opCommitMemory, request{Source: 1})
	pcm := make([]float32, 128)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i)*.11)) * .5
	}
	call(t, s, opStream, request{Source: 2, Rate: 24000, Channels: 1, Frames: 128})
	buf, err := s.streamBuffer(len(pcm))
	if err != nil {
		t.Fatal(err)
	}
	copy(buf, pcm)
	call(t, s, opStreamWrite, request{Source: 2, Samples: len(pcm)})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 1, Kind: mix.CmdPlay, Gen: 1,
		Source: 1, GainBits: math.Float32bits(1), PitchBits: math.Float32bits(1)}})
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 2, Kind: mix.CmdReplaceStream, Index: 1, Gen: 1,
		PrevGen: 1, Source: 2, Frame: 1, Paused: true, GainBits: math.Float32bits(1), PitchBits: math.Float32bits(1)}})
	call(t, s, opSkip, request{Frames: 0})
	batch, err := workerproto.DecodeBatch(call(t, s, opDrain, request{}))
	if err != nil || len(batch.Released) != 1 || batch.Released[0].Index != 0 || batch.Active != 1 {
		t.Fatalf("replacement batch = %+v, %v", batch, err)
	}
	call(t, s, opDrop, request{Source: 1})
	if _, err := s.invoke(opDrop, []byte(`{"source":"2"}`)); err == nil {
		t.Fatal("dropped the new stream while it was in use")
	}
	call(t, s, opCommand, request{Command: commandRequest{Sequence: 3, Kind: mix.CmdResume, Index: 1, Gen: 1}})
	got, err := s.render(32)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Voices: 4})
	if err != nil {
		t.Fatal(err)
	}
	id := ref.Play(mix.PlayParams{Memory: &mix.Memory{PCM: pcm, Rate: 24000, Channels: 1}, Gain: 1})
	if !id.Valid() || !ref.SeekMemory(id, 1) {
		t.Fatal("could not prepare reference PCM")
	}
	want := make([]float32, len(got))
	ref.Mix(want)
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("sample %d: worker=%v reference=%v", i, got[i], want[i])
		}
	}
}
