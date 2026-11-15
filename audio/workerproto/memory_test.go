package workerproto

import (
	"reflect"
	"testing"

	"aego/audio/mix"
)

func TestMemorySenderRequiresOrderedAcknowledgements(t *testing.T) {
	const id uint64 = 1<<53 + 7
	pcm := []float32{1, 2, 3, 4, 5, 6, 7}
	source := &mix.Memory{PCM: pcm, Rate: 48000, Channels: 1, LoopStart: 1, LoopEnd: 6, Loop: true}
	s, err := NewMemorySender(id, source, 3)
	if err != nil {
		t.Fatal(err)
	}
	source.Rate = 44100 // Metadata was captured at construction.
	begin, ok, err := s.Next()
	if err != nil || !ok || begin.Token != 1 || begin.Op != BeginMemory || begin.Request.Source != id ||
		begin.Request.Rate != 48000 || begin.Request.Samples != 7 || begin.Request.LoopStart != 1 || !begin.Request.Loop {
		t.Fatalf("begin = %+v, %v, %v", begin, ok, err)
	}
	if _, _, err := s.Next(); err == nil {
		t.Fatal("issued another step before the reply")
	}
	if err := s.Ack(begin.Token + 1); err == nil {
		t.Fatal("accepted a fabricated acknowledgement")
	}
	if err := s.Ack(begin.Token); err != nil {
		t.Fatal(err)
	}
	for i, want := range [][]float32{{1, 2, 3}, {4, 5, 6}, {7}} {
		chunk, ok, err := s.Next()
		if err != nil || !ok || !chunk.Chunk || chunk.Offset != i*3 || !reflect.DeepEqual(chunk.PCM, want) {
			t.Fatalf("chunk %d = %+v, %v, %v", i, chunk, ok, err)
		}
		if err := s.Ack(begin.Token); err == nil {
			t.Fatal("accepted a stale acknowledgement")
		}
		if err := s.Ack(chunk.Token); err != nil {
			t.Fatal(err)
		}
	}
	commit, ok, err := s.Next()
	if err != nil || !ok || commit.Op != CommitMemory || commit.Request.Source != id || commit.Chunk {
		t.Fatalf("commit = %+v, %v, %v", commit, ok, err)
	}
	if err := s.Ack(commit.Token); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.Next(); err != nil || ok {
		t.Fatalf("completed transfer = %v, %v", ok, err)
	}
	if err := s.Ack(commit.Token); err == nil {
		t.Fatal("acknowledged a committed transfer twice")
	}
}

func TestMemorySenderHandlesEmptyAndRejectsInvalidParameters(t *testing.T) {
	for _, tc := range []struct {
		id     uint64
		source *mix.Memory
		chunk  int
	}{{0, &mix.Memory{}, 1}, {1, nil, 1}, {1, &mix.Memory{}, 0}, {1, &mix.Memory{}, MaxTransferSamples + 1}} {
		if _, err := NewMemorySender(tc.id, tc.source, tc.chunk); err == nil {
			t.Fatalf("accepted invalid sender %+v", tc)
		}
	}
	s, err := NewMemorySender(1, &mix.Memory{}, MaxTransferSamples)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int32{BeginMemory, CommitMemory} {
		step, ok, err := s.Next()
		if err != nil || !ok || step.Op != want || step.Chunk {
			t.Fatalf("step = %+v, %v, %v", step, ok, err)
		}
		if err := s.Ack(step.Token); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok, err := s.Next(); err != nil || ok {
		t.Fatalf("empty transfer = %v, %v", ok, err)
	}
}
