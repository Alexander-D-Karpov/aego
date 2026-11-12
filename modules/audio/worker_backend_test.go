package audio

import (
	"errors"
	"testing"

	"aego/audio/mix"
	"aego/audio/pump"
	"aego/platform"
)

type testWorkerBackend struct {
	ticks, closes int
	want          platform.AudioSpec
	err           error
}

func (b *testWorkerBackend) Tick()  { b.ticks++ }
func (b *testWorkerBackend) Close() { b.closes++ }
func (b *testWorkerBackend) Stats() pump.Stats {
	return pump.Stats{Blocks: 19, Underruns: 2}
}
func (b *testWorkerBackend) Reopen(want platform.AudioSpec) error {
	b.want = want
	return b.err
}

func TestWorkerBackendDelegatesWithoutUsingLocalPump(t *testing.T) {
	mixer, err := mix.New(mix.Config{SampleRate: 48000, Voices: 4})
	if err != nil {
		t.Fatal(err)
	}
	marker := errors.New("injected reopen failure")
	b := &testWorkerBackend{err: marker}
	m := &Module{mixer: mixer, worker: b, manual: true}
	m.update(1.0 / 60)
	if b.ticks != 1 {
		t.Fatal("frame update did not drive the worker")
	}
	want := platform.AudioSpec{SampleRate: 44100, Channels: 2, Frames: 512}
	if err := m.Reopen(want); !errors.Is(err, marker) || b.want != want {
		t.Fatal("reopen did not retain worker ownership and return its error")
	}
	if stats := m.Stats(); stats.Blocks != 19 || stats.Underruns != 2 {
		t.Fatalf("worker statistics were lost: %+v", stats)
	}
	m.Shutdown()
	if b.closes != 1 || m.worker != nil || m.mixer != nil {
		t.Fatal("worker was not closed before module shutdown")
	}
}
