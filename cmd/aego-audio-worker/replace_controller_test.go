package main

import (
	"math"
	"testing"

	"aego/audio/mix"
	"aego/audio/workerproto"
)

func TestControllerReplaceStreamTransfersAndReleasesSources(t *testing.T) {
	s := initService(t, 2<<20)
	m, err := mix.New(mix.Config{SampleRate: 48000, Voices: 4, Block: 512})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := workerproto.NewController(m)
	if err != nil {
		t.Fatal(err)
	}
	drive := func() {
		t.Helper()
		for i := 0; i < 1000; i++ {
			step, ok, err := controller.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				return
			}
			var result []byte
			if step.Chunk {
				copy(s.memory[step.Offset:], step.PCM)
			} else {
				if step.Op == opStreamWrite {
					buf, err := s.streamBuffer(len(step.PCM))
					if err != nil {
						t.Fatal(err)
					}
					copy(buf, step.PCM)
				}
				result = call(t, s, step.Op, step.Request)
			}
			if err := controller.Ack(step.Token, result); err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("controller did not become idle")
	}
	drain := func() {
		t.Helper()
		batch, err := workerproto.DecodeBatch(call(t, s, opDrain, request{}))
		if err != nil {
			t.Fatal(err)
		}
		if err := controller.ApplyBatch(batch.Mix()); err != nil {
			t.Fatalf("invalid batch: %+v, %v", batch, err)
		}
	}
	old := m.Play(mix.PlayParams{Memory: &mix.Memory{PCM: make([]float32, 128), Rate: 48000, Channels: 1}, Gain: 1})
	if !old.Valid() {
		t.Fatal("could not start old preview")
	}
	drive()
	if _, err := s.render(1); err != nil {
		t.Fatal(err)
	}
	drain()
	stream := mix.NewStream(24000, 1, 128)
	pcm := make([]float32, 128)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i)*.11)) * .5
	}
	if stream.Ring.Write(pcm) != len(pcm) {
		t.Fatal("could not prepare stream PCM")
	}
	newVoice := m.ReplaceStream(old, mix.PlayParams{Stream: stream, Gain: 1}, 1, false)
	if !newVoice.Valid() {
		t.Fatal("could not queue replacement")
	}
	drive()
	got, err := s.render(32)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := mix.New(mix.Config{SampleRate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	id := ref.Play(mix.PlayParams{Memory: &mix.Memory{PCM: pcm, Rate: 24000, Channels: 1}, Gain: 1})
	if !id.Valid() || !ref.SeekMemory(id, 1) {
		t.Fatal("could not prepare reference")
	}
	want := make([]float32, len(got))
	ref.Mix(want)
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("sample %d: worker=%v reference=%v", i, got[i], want[i])
		}
	}
	drain()
	if m.Live(old) || !m.Live(newVoice) {
		t.Fatal("producer did not reconcile the replaced voice")
	}
	if !m.StopPair(old, newVoice, 0) {
		t.Fatal("could not stop replacement")
	}
	drive()
	if _, err := s.render(512); err != nil {
		t.Fatal(err)
	}
	drain()
	drive() // Retire both source registrations and the stream sender claim.
	claim, err := mix.NewRemoteStreamSender(stream)
	if err != nil {
		t.Fatalf("replacement retained its stream claim: %v", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatal(err)
	}
}
