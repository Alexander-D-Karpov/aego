package mix

import "errors"

var (
	ErrRemoteStream     = errors.New("mix: invalid remote stream state")
	ErrRemoteStreamBusy = errors.New("mix: remote stream producer is finishing a write")
)

// RemoteStreamSender transfers one stream to another mixer without consuming
// decoded frames until the worker acknowledges them. It is the sole consumer
// of the source ring. The producer may continue writing concurrently.
type RemoteStreamSender struct {
	source         *Stream
	pending        int
	pendingHead    uint64
	controlPending bool
	controlSkip    int64
	controlHeld    bool
	closed         bool
}

func NewRemoteStreamSender(s *Stream) (*RemoteStreamSender, error) {
	if s == nil || s.Ring == nil || s.Channels != s.Ring.Channels() || !s.remote.CompareAndSwap(false, true) {
		return nil, ErrRemoteStream
	}
	if !s.Ring.remote.CompareAndSwap(nil, s) {
		s.remote.Store(false)
		return nil, ErrRemoteStream
	}
	return &RemoteStreamSender{source: s}, nil
}

// Peek copies up to len(dst) samples without advancing the ring. Only one
// transfer may be outstanding. EOF is reported only after every frame has been
// acknowledged; the transport must deliver EOF after all prior PCM messages.
func (r *RemoteStreamSender) Peek(dst []float32) (frames int, eof bool, err error) {
	if r == nil || r.closed || r.pending != 0 || r.controlPending {
		return 0, false, ErrRemoteStream
	}
	s := r.source
	if s.Held() {
		// A completed decoder with no queued PCM must still publish EOF while
		// held, or a voice skipped beyond the end can remain live while muted.
		return 0, s.EOF() && s.Ring.Len() == 0, nil
	}
	head := s.Ring.head.Load()
	n := min(len(dst), int(s.Ring.tail.Load()-head))
	n -= n % s.Channels
	if n == 0 {
		return 0, s.EOF() && s.Ring.Len() == 0, nil
	}
	k := copy(dst[:n], s.Ring.buf[int(head&s.Ring.mask):])
	copy(dst[k:n], s.Ring.buf)
	r.pending, r.pendingHead = n/s.Channels, head
	return r.pending, false, nil
}

// Ack advances only the accepted prefix. The unaccepted suffix remains in the
// source ring for a later transfer; Ack(0) is a valid refusal of the chunk.
func (r *RemoteStreamSender) Ack(frames int) error {
	if r == nil || r.closed || r.controlPending || r.pending == 0 || frames < 0 || frames > r.pending ||
		r.source.Ring.head.Load() != r.pendingHead {
		return ErrRemoteStream
	}
	if got := r.source.Ring.Discard(int64(frames)); got != int64(frames) {
		return ErrRemoteStream
	}
	r.pending = 0
	r.source.drained()
	return nil
}

// AbortTransfer leaves the source ring unchanged. It is intended for a
// terminated transport, not for retrying a message that may still be accepted.
func (r *RemoteStreamSender) AbortTransfer() error {
	if r == nil || r.closed || r.controlPending || r.pending == 0 || r.source.Ring.head.Load() != r.pendingHead {
		return ErrRemoteStream
	}
	r.pending = 0
	return nil
}

// BeginControl starts reconciliation of a worker's skip/hold after every
// earlier PCM transfer has been acknowledged. An active producer is never
// blocked: ErrRemoteStreamBusy leaves it held, and FinishControl retries once
// its current write has returned. The worker transport must reject stale
// in-flight PCM after a skip before delivering this control to the sender.
func (r *RemoteStreamSender) BeginControl(skip int64, held bool) error {
	if r == nil || r.closed || r.pending != 0 || r.controlPending || skip < 0 {
		return ErrRemoteStream
	}
	r.controlPending, r.controlSkip, r.controlHeld = true, skip, held
	r.source.hold.Store(true)
	r.source.signal()
	return r.FinishControl()
}

func (r *RemoteStreamSender) FinishControl() error {
	if r == nil || r.closed || !r.controlPending {
		return ErrRemoteStream
	}
	s := r.source
	if s.Ring.writing.Load() != 0 {
		return ErrRemoteStreamBusy
	}
	got := s.Ring.Discard(r.controlSkip)
	if left := r.controlSkip - got; left > 0 {
		s.skipped.Add(left)
	}
	s.hold.Store(r.controlHeld)
	s.signal()
	s.drained()
	r.controlPending = false
	r.controlSkip = 0
	return nil
}

// Close releases the single-consumer claim and any hold only after transfers
// and controls have completed. The caller must first terminate the old worker
// transport; on failure it is responsible for cancelling or reattaching the
// producer because it cannot infer how much audio the worker actually played.
func (r *RemoteStreamSender) Close() error {
	if r == nil || r.closed || r.pending != 0 || r.controlPending {
		return ErrRemoteStream
	}
	if !r.source.Ring.remote.CompareAndSwap(r.source, nil) {
		return ErrRemoteStream
	}
	r.closed = true
	r.source.release()
	r.source.remote.Store(false)
	return nil
}
