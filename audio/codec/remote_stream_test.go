package codec

import (
	"sync"
	"testing"
	"time"

	"aego/audio/mix"
)

// Use the real decoder feeder and sender, with decoding held across a remote
// skip. No device or browser transport is involved.
func TestRemoteStreamSkipDuringDecodePreservesFrameOrder(t *testing.T) {
	const rate = 48000
	d := &observedSeekDecoder{
		info:  Info{SampleRate: rate, Channels: 1, Frames: 1000},
		infos: make(chan struct{}, 16), seeks: make(chan int64, 16),
		reads: make(chan int64, 16), readGate: make(chan struct{}),
	}
	d.blockRead.Store(true)
	s := mix.NewStream(rate, 1, 64)
	sender, err := mix.NewRemoteStreamSender(s)
	if err != nil {
		t.Fatal(err)
	}
	cancel := make(chan struct{})
	done := Stream(d, s, StreamOptions{Chunk: 16}, cancel)
	var gate sync.Once
	openGate := func() { gate.Do(func() { close(d.readGate) }) }
	t.Cleanup(func() {
		openGate()
		close(cancel)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stream feeder: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("stream feeder did not cancel")
		}
		if err := sender.Close(); err != nil {
			t.Errorf("close sender: %v", err)
		}
	})
	waitDecoderInfo(t, d)
	select {
	case frame := <-d.reads:
		if frame != 0 {
			t.Fatalf("first read began at %d", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("decoder did not start reading")
	}
	if err := sender.BeginControl(5, false); err != nil {
		t.Fatal(err)
	}
	openGate()
	waitStreamRing(t, s, 8)
	got := make([]float32, 8)
	n, eof, err := sender.Peek(got)
	if err != nil || eof || n != len(got) {
		t.Fatalf("peek = %d frames, eof=%v, %v", n, eof, err)
	}
	for i, sample := range got {
		if sample != float32(i+5) {
			t.Fatalf("sample %d = %v, want %d", i, sample, i+5)
		}
	}
	if err := sender.Ack(n); err != nil {
		t.Fatal(err)
	}
}
