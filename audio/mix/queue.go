package mix

import "sync/atomic"

type CommandKind uint8

const (
	CmdPlay CommandKind = iota + 1
	CmdStop
	CmdSetGain
	CmdSetPan
	CmdSetPitch
	CmdPause
	CmdResume
	CmdBusGain
	CmdBusMute
	CmdBusSolo
	CmdPauseAll
	CmdResumeAll
	CmdBusLayer
	CmdKill
	CmdSeekMemory
	CmdReplaceStream
	CmdStopPair
)

type Command struct {
	Kind     CommandKind
	Voice    VoiceID
	Bus      int32
	Layer    uint8
	Memory   *Memory
	Stream   *Stream
	Gain     float32
	Pan      float32
	Pitch    float32
	Fade     float32
	Priority int32
	Flag     bool
	Frame    int64
	Previous VoiceID
	Paused   bool
}

type EventKind uint8

const (
	EvFinished EventKind = iota + 1
	EvStolen
	EvRejected
	EvStarved
)

type Event struct {
	Kind  EventKind
	Voice VoiceID
}

type spsc[T any] struct {
	buf  []T
	mask uint64
	head atomic.Uint64
	_    [56]byte
	tail atomic.Uint64
}

func newSPSC[T any](size int) *spsc[T] {
	n := 1
	for n < size {
		n <<= 1
	}
	return &spsc[T]{buf: make([]T, n), mask: uint64(n - 1)}
}

func (q *spsc[T]) push(v T) bool {
	t := q.tail.Load()
	if t-q.head.Load() == uint64(len(q.buf)) {
		return false
	}
	q.buf[t&q.mask] = v
	q.tail.Store(t + 1)
	return true
}

func (q *spsc[T]) pop() (T, bool) {
	var zero T
	h := q.head.Load()
	if h == q.tail.Load() {
		return zero, false
	}
	v := q.buf[h&q.mask]
	q.buf[h&q.mask] = zero
	q.head.Store(h + 1)
	return v, true
}

func (q *spsc[T]) len() int {
	return int(q.tail.Load() - q.head.Load())
}

type CommandQueue struct {
	q       *spsc[Command]
	dropped atomic.Uint64
	wake    chan struct{}
	// Set only before a remote consumer starts. This counts queued and
	// transferred-but-not-yet-applied commands against the same capacity.
	remote *atomic.Int32
}

func (c *CommandQueue) Push(cmd Command) bool {
	if c.remote != nil {
		for {
			n := c.remote.Load()
			if n >= int32(len(c.q.buf)) {
				c.dropped.Add(1)
				return false
			}
			if c.remote.CompareAndSwap(n, n+1) {
				break
			}
		}
	}
	if c.q.push(cmd) {
		select {
		case c.wake <- struct{}{}:
		default:
		}
		return true
	}
	if c.remote != nil {
		c.remote.Add(-1)
	}
	c.dropped.Add(1)
	return false
}

func (c *CommandQueue) Dropped() uint64 {
	return c.dropped.Load()
}

// Wake receives after a push, for a consumer that sleeps while the queue is empty.
func (c *CommandQueue) Wake() <-chan struct{} {
	return c.wake
}

func (c *CommandQueue) Len() int {
	return c.q.len()
}

type EventQueue struct {
	q       *spsc[Event]
	dropped atomic.Uint64
}

func (e *EventQueue) Pop() (Event, bool) {
	return e.q.pop()
}

func (e *EventQueue) Dropped() uint64 {
	return e.dropped.Load()
}

func (e *EventQueue) push(ev Event) {
	if !e.q.push(ev) {
		e.dropped.Add(1)
	}
}

type Ring struct {
	buf  []float32
	mask uint64
	ch   int
	head atomic.Uint64
	_    [56]byte
	tail atomic.Uint64
	// A remote stream can suspend its producer before reconciling a skip.
	// The writer counter makes attaching that guard safe even if a producer
	// was already in Write; the audio consumer never waits on this counter.
	remote  atomic.Pointer[Stream]
	writing atomic.Int32
}

func NewRing(frames, channels int) *Ring {
	if channels != 2 {
		channels = 1
	}
	n := 2
	for n < frames*channels {
		n <<= 1
	}
	return &Ring{buf: make([]float32, n), mask: uint64(n - 1), ch: channels}
}

func (r *Ring) Channels() int {
	return r.ch
}

func (r *Ring) Cap() int {
	return len(r.buf) / r.ch
}

func (r *Ring) Len() int {
	return int(r.tail.Load()-r.head.Load()) / r.ch
}

func (r *Ring) Free() int {
	return r.Cap() - r.Len()
}

func (r *Ring) Write(p []float32) int {
	if len(p) == 0 {
		return 0
	}
	r.writing.Add(1)
	defer r.writing.Add(-1)
	if s := r.remote.Load(); s != nil && (s.hold.Load() || s.skipped.Load() > 0) {
		return 0
	}
	t := r.tail.Load()
	n := min(len(p), len(r.buf)-int(t-r.head.Load()))
	n -= n % r.ch
	if n <= 0 {
		return 0
	}
	k := copy(r.buf[int(t&r.mask):], p[:n])
	copy(r.buf, p[k:n])
	r.tail.Store(t + uint64(n))
	return n / r.ch
}

// Discard drops up to frames frames from the read side and returns how many it dropped.
func (r *Ring) Discard(frames int64) int64 {
	if frames <= 0 {
		return 0
	}
	h := r.head.Load()
	// Bound by the number of complete frames before multiplying by channels:
	// a long skip must not overflow and leave the buffered prefix behind.
	n := min(frames, int64((r.tail.Load()-h)/uint64(r.ch)))
	r.head.Store(h + uint64(n)*uint64(r.ch))
	return n
}

func (r *Ring) Read(p []float32) int {
	h := r.head.Load()
	n := min(len(p), int(r.tail.Load()-h))
	n -= n % r.ch
	if n <= 0 {
		return 0
	}
	k := copy(p[:n], r.buf[int(h&r.mask):])
	copy(p[k:n], r.buf)
	r.head.Store(h + uint64(n))
	return n / r.ch
}
