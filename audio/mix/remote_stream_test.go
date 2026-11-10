package mix

import (
	"errors"
	"math"
	"testing"
	"testing/synctest"
)

func TestRemoteStreamDiscardHugeSkipDoesNotOverflow(t *testing.T) {
	for _, channels := range []int{1, 2} {
		s := NewStream(rate, channels, 8)
		if got := s.Ring.Write(make([]float32, 8*channels)); got != 8 {
			t.Fatalf("channels %d: wrote %d frames", channels, got)
		}
		sender, err := NewRemoteStreamSender(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := sender.BeginControl(math.MaxInt64, true); err != nil {
			t.Fatal(err)
		}
		if s.Ring.Len() != 0 || s.TakeSkip() != math.MaxInt64-8 {
			t.Fatalf("channels %d: large skip did not discard the buffered prefix", channels)
		}
		if err := sender.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemoteStreamTransferAcknowledgesOnlyAcceptedFrames(t *testing.T) {
	s := NewStream(rate, 2, 8)
	input := make([]float32, 16)
	for i := range input {
		input[i] = float32(i + 1)
	}
	if n := s.Ring.Write(input); n != 8 {
		t.Fatalf("wrote %d frames", n)
	}
	s.SetEOF()
	sender, err := NewRemoteStreamSender(s)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 8)
	n, eof, err := sender.Peek(buf)
	if err != nil || n != 4 || eof || s.Ring.Len() != 8 {
		t.Fatalf("peek=%d eof=%v len=%d err=%v", n, eof, s.Ring.Len(), err)
	}
	if _, _, err := sender.Peek(buf); err == nil {
		t.Fatal("accepted a second transfer before acknowledgement")
	}
	if err := sender.Ack(5); err == nil || s.Ring.Len() != 8 {
		t.Fatal("invalid acknowledgement changed the ring")
	}
	if err := sender.Ack(2); err != nil || s.Ring.Len() != 6 {
		t.Fatalf("partial acknowledgement: len=%d err=%v", s.Ring.Len(), err)
	}
	n, eof, err = sender.Peek(buf)
	if err != nil || n != 4 || eof || buf[0] != input[4] {
		t.Fatalf("retry began at %v, frames=%d eof=%v err=%v", buf[0], n, eof, err)
	}
	if err := sender.Ack(0); err != nil || s.Ring.Len() != 6 {
		t.Fatal("zero acknowledgement discarded data")
	}
	all := make([]float32, 16)
	n, eof, err = sender.Peek(all)
	if err != nil || n != 6 || eof {
		t.Fatalf("remaining frames=%d eof=%v err=%v", n, eof, err)
	}
	if err := sender.Ack(n); err != nil {
		t.Fatal(err)
	}
	n, eof, err = sender.Peek(all)
	if err != nil || n != 0 || !eof {
		t.Fatalf("final frames=%d eof=%v err=%v", n, eof, err)
	}
}

func TestRemoteStreamSkipPreservesUnacknowledgedData(t *testing.T) {
	s := NewStream(rate, 1, 8)
	if n := s.Ring.Write([]float32{1, 2, 3, 4, 5, 6, 7, 8}); n != 8 {
		t.Fatal(n)
	}
	sender, err := NewRemoteStreamSender(s)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4)
	if n, _, err := sender.Peek(buf); err != nil || n != 4 {
		t.Fatalf("peek=%d err=%v", n, err)
	}
	if err := sender.BeginControl(8, true); err == nil || s.Held() || s.Ring.Len() != 8 {
		t.Fatal("skip crossed an unacknowledged transfer")
	}
	if err := sender.Ack(4); err != nil {
		t.Fatal(err)
	}
	if err := sender.BeginControl(8, true); err != nil {
		t.Fatal(err)
	}
	if !s.Held() || s.Ring.Len() != 0 || s.TakeSkip() != 4 {
		t.Fatal("remote skip did not discard queued frames and forward the remainder")
	}
	if n := s.Ring.Write([]float32{9}); n != 0 {
		t.Fatal("a held stream accepted more frames")
	}
	if err := sender.BeginControl(0, false); err != nil || s.Held() {
		t.Fatalf("release hold: %v", err)
	}
	if n := s.Ring.Write([]float32{9}); n != 1 {
		t.Fatal("released stream did not accept frames")
	}
}

func TestRemoteStreamControlWaitsWithoutBlockingForAnActiveWriter(t *testing.T) {
	s := NewStream(rate, 1, 8)
	s.Ring.Write([]float32{1, 2, 3})
	sender, err := NewRemoteStreamSender(s)
	if err != nil {
		t.Fatal(err)
	}
	// Model a producer that entered Ring.Write just before the hold was set.
	s.Ring.writing.Add(1)
	if err := sender.BeginControl(2, false); !errors.Is(err, ErrRemoteStreamBusy) {
		t.Fatalf("active writer: %v", err)
	}
	if !s.Held() || s.Ring.Len() != 3 {
		t.Fatal("pending control consumed data before the writer left")
	}
	if _, _, err := sender.Peek(make([]float32, 8)); err == nil {
		t.Fatal("transferred data while control was pending")
	}
	s.Ring.writing.Add(-1)
	if err := sender.FinishControl(); err != nil {
		t.Fatal(err)
	}
	if s.Held() || s.Ring.Len() != 1 {
		t.Fatal("control failed to discard exactly two frames and release hold")
	}
}

func TestRemoteStreamSkipCannotPublishPendingFramesBeforeFeederProcessesIt(t *testing.T) {
	s := NewStream(rate, 1, 8)
	sender, err := NewRemoteStreamSender(s)
	if err != nil {
		t.Fatal(err)
	}
	// The feeder may have already checked Held and TakeSkip before the worker
	// reports a skip. Its next Write must not publish those old pending frames,
	// even when the worker is already rendering again and released the hold.
	if err := sender.BeginControl(5, false); err != nil {
		t.Fatal(err)
	}
	if s.Held() {
		t.Fatal("the worker released its hold")
	}
	if n := s.Ring.Write([]float32{1, 2, 3}); n != 0 || s.Ring.Len() != 0 {
		t.Fatal("published pending frames before the feeder observed the skip")
	}
	if got := s.TakeSkip(); got != 5 {
		t.Fatalf("skip = %d, want 5", got)
	}
	if n := s.Ring.Write([]float32{6, 7, 8}); n != 3 {
		t.Fatal("the feeder could not publish frames after handling the skip")
	}
}

func TestRemoteStreamHeldEOFWaitsForQueuedFramesButDoesNotLeak(t *testing.T) {
	s := NewStream(rate, 1, 8)
	if n := s.Ring.Write([]float32{1, 2, 3}); n != 3 {
		t.Fatal(n)
	}
	s.SetEOF()
	sender, err := NewRemoteStreamSender(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.BeginControl(1, true); err != nil {
		t.Fatal(err)
	}
	if n, eof, err := sender.Peek(make([]float32, 8)); err != nil || n != 0 || eof {
		t.Fatalf("held stream with queued data: frames=%d eof=%v err=%v", n, eof, err)
	}
	if err := sender.BeginControl(5, true); err != nil {
		t.Fatal(err)
	}
	if n, eof, err := sender.Peek(make([]float32, 8)); err != nil || n != 0 || !eof {
		t.Fatalf("held exhausted stream must forward EOF: frames=%d eof=%v err=%v", n, eof, err)
	}
}

func TestRemoteStreamCloseReleasesProducerAndGuard(t *testing.T) {
	s := NewStream(rate, 1, 8)
	sender, err := NewRemoteStreamSender(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.BeginControl(0, true); err != nil {
		t.Fatal(err)
	}
	if err := sender.Close(); err != nil {
		t.Fatal(err)
	}
	if s.Held() || s.Ring.remote.Load() != nil {
		t.Fatal("closing the sender left the feeder held or the ring guarded")
	}
	if n := s.Ring.Write([]float32{1}); n != 1 {
		t.Fatal("the feeder remained unable to write after close")
	}
	if _, err := NewRemoteStreamSender(s); err != nil {
		t.Fatalf("could not attach a new sender: %v", err)
	}
}

func TestRemoteStreamAcknowledgementWakesBlockedFeeder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewStream(rate, 1, 8)
		s.Ring.Write(make([]float32, 8))
		sender, err := NewRemoteStreamSender(s)
		if err != nil {
			t.Fatal(err)
		}
		cancel, done := make(chan struct{}), make(chan bool, 1)
		go func() { done <- s.WaitSpace(cancel) }()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("feeder did not wait on the full ring")
		default:
		}
		if n, _, err := sender.Peek(make([]float32, 2)); err != nil || n != 2 {
			t.Fatalf("peek=%d err=%v", n, err)
		}
		if err := sender.Ack(2); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case ok := <-done:
			if !ok {
				t.Fatal("feeder reported cancellation")
			}
		default:
			t.Fatal("accepted frames did not wake the feeder")
		}
	})
}
