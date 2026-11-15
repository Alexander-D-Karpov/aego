package mix

import "errors"

var (
	ErrRemoteStream     = errors.New("mix: invalid remote stream state")
	ErrRemoteStreamBusy = errors.New("mix: remote stream producer is finishing a write")
)

// RemoteStreamSender solely consumes the producer ring and retains accepted PCM until reported consumed; producers may write concurrently.
type RemoteStreamSender struct {
	source         *Stream
	pending        int
	pendingHead    uint64
	sent           uint64
	consumed       uint64
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

// Peek copies without advancing; allow one transfer, and deliver EOF only after all frames are accepted and prior PCM sent.
func (r *RemoteStreamSender) Peek(dst []float32) (frames int, eof bool, err error) {
	if r == nil || r.closed || r.pending != 0 || r.controlPending {
		return 0, false, ErrRemoteStream
	}
	s := r.source
	head := s.Ring.head.Load()
	tail := s.Ring.tail.Load()
	available := (tail - head) / uint64(s.Channels)
	if r.sent > available {
		return 0, false, ErrRemoteStream
	}
	if s.Held() {
		// Publish EOF even while held so a skipped-past-end voice cannot remain live.
		return 0, s.EOF() && r.sent == available, nil
	}
	start := head + r.sent*uint64(s.Channels)
	n := min(len(dst), int(tail-start))
	n -= n % s.Channels
	if n == 0 {
		return 0, s.EOF() && r.sent == available, nil
	}
	k := copy(dst[:n], s.Ring.buf[int(start&s.Ring.mask):])
	copy(dst[k:n], s.Ring.buf)
	r.pending, r.pendingHead = n/s.Channels, head
	return r.pending, false, nil
}

// Ack records the accepted prefix without consuming it; the suffix remains available and Ack(0) is valid.
func (r *RemoteStreamSender) Ack(frames int) error {
	if r == nil || r.closed || r.controlPending || r.pending == 0 || frames < 0 || frames > r.pending ||
		r.source.Ring.head.Load() != r.pendingHead {
		return ErrRemoteStream
	}
	r.sent += uint64(frames)
	r.pending = 0
	return nil
}

// Reconcile consumes worker-read frames only when counts match ordered write acknowledgements; stale or fabricated reports change nothing.
func (r *RemoteStreamSender) Reconcile(consumed uint64, buffered int) error {
	if r == nil || r.closed || r.pending != 0 || r.controlPending || buffered < 0 || consumed < r.consumed {
		return ErrRemoteStream
	}
	delta := consumed - r.consumed
	if delta > r.sent || uint64(buffered) != r.sent-delta || delta > uint64(r.source.Ring.Len()) {
		return ErrRemoteStream
	}
	if got := r.source.Ring.Discard(int64(delta)); got != int64(delta) {
		return ErrRemoteStream
	}
	r.sent -= delta
	r.consumed = consumed
	r.source.drained()
	return nil
}

// AbortTransfer preserves the source ring after termination, not for retrying uncertain delivery.
func (r *RemoteStreamSender) AbortTransfer() error {
	if r == nil || r.closed || r.controlPending || r.pending == 0 || r.source.Ring.head.Load() != r.pendingHead {
		return ErrRemoteStream
	}
	r.pending = 0
	return nil
}

// BeginControl reconciles skip/hold after prior PCM acks; reject stale PCM, keep a busy producer held without blocking, and retry with FinishControl.
func (r *RemoteStreamSender) BeginControl(skip int64, held bool) error {
	if r == nil || r.closed || r.pending != 0 || r.controlPending || skip < 0 || (skip > 0 && r.sent != 0) {
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

// Close releases the claim and hold after transfers/controls and worker termination; the caller must reconcile uncertain playback and producer ownership.
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
