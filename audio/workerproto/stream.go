package workerproto

import (
	"errors"

	"aego/audio/mix"
)

const (
	streamRegister uint8 = iota
	streamIdle
	streamStatus
	streamControlBusy
	streamControlAck
	streamWrite
	streamEnd
	streamEnded
	streamClosed
)

// StreamStep is one ordered operation. The transport must copy PCM before
// sending; the source retains accepted frames until consumption is reconciled.
type StreamStep struct {
	Token   uint64
	Op      int32
	Request Request
	PCM     []float32
}

// StreamSender coordinates a single producer ring with a remote ring. The
// owner serializes calls, starts a Poll on its own schedule, delivers replies
// in order, and handles transport failure; source format and ring identity
// must not change while attached. This type is not an output driver.
type StreamSender struct {
	id      uint64
	source  *mix.Stream
	sender  *mix.RemoteStreamSender
	buffer  []float32
	state   uint8
	control uint32
	offered int
	serial  uint64
	pending uint64
	broken  bool
	ended   bool
	closing bool
	final   bool
}

func NewStreamSender(id uint64, source *mix.Stream, chunkFrames int) (*StreamSender, error) {
	if id == 0 || source == nil || source.Ring == nil || (source.Channels != 1 && source.Channels != 2) ||
		source.Channels != source.Ring.Channels() || source.Rate <= 0 || source.Ring.Cap() <= 0 ||
		source.Ring.Cap() > 1<<20 || chunkFrames <= 0 || chunkFrames > MaxTransferSamples/source.Channels {
		return nil, ErrTransfer
	}
	sender, err := mix.NewRemoteStreamSender(source)
	if err != nil {
		return nil, err
	}
	return &StreamSender{id: id, source: source, sender: sender, buffer: make([]float32, chunkFrames*source.Channels)}, nil
}

// Poll begins one status/transfer cycle, including after EOF so consumed PCM
// can still be reconciled. It does not install a timer or spin while the
// decoder has no PCM. Finish the current cycle before polling again.
func (s *StreamSender) Poll() error {
	if s == nil || s.broken || s.closing || (s.state != streamIdle && s.state != streamEnded) || s.pending != 0 {
		return ErrTransfer
	}
	s.state = streamStatus
	return nil
}

// Idle reports that a transfer cycle has completed. In particular, a false
// return from Next while a producer is finishing a write is not idle: the
// caller must retry the pending control before polling or closing the source.
func (s *StreamSender) Idle() bool {
	return s != nil && !s.broken && s.pending == 0 && (s.state == streamIdle || s.state == streamEnded)
}

// FinishAfterStop requests a final status after the owner has stopped all
// voices using this stream and drained their releases. No further worker voice
// may use this stream, and no writes or source removal may occur until the
// resulting steps are acknowledged. Other independent voices may still mix.
// The caller must then drop the worker source before Close. This explicit
// quiescence contract prevents an orderly stop from replaying consumed PCM.
func (s *StreamSender) FinishAfterStop() error {
	if s == nil || s.broken || s.closing || (s.state != streamIdle && s.state != streamEnded) || s.pending != 0 {
		return ErrTransfer
	}
	s.closing, s.state = true, streamStatus
	return nil
}

// ReadyToClose reports that the final status and any resulting control have
// been reconciled. The owner must still drop the quiescent worker source first.
func (s *StreamSender) ReadyToClose() bool {
	return s != nil && !s.broken && s.closing && s.final && s.pending == 0
}

// Ended means the worker accepted EOF; it does not mean its queued audio was
// played or that the source is safe to drop while a voice still references it.
func (s *StreamSender) Ended() bool {
	return s != nil && !s.broken && s.ended && s.state != streamClosed
}

func (s *StreamSender) Next() (step StreamStep, ok bool, err error) {
	if s == nil || s.broken || s.pending != 0 || s.state == streamClosed {
		return step, false, ErrTransfer
	}
	if s.state == streamIdle || s.state == streamEnded {
		return step, false, nil
	}
	if s.state == streamControlBusy {
		if err := s.sender.FinishControl(); err != nil {
			if errors.Is(err, mix.ErrRemoteStreamBusy) {
				return step, false, nil
			}
			s.broken = true
			return step, false, err
		}
		s.state = streamControlAck
	}
	if s.serial == ^uint64(0) {
		return step, false, ErrTransfer
	}
	s.serial++
	step.Token = s.serial
	step.Request.Source = s.id
	switch s.state {
	case streamRegister:
		step.Op = Stream
		step.Request.Rate, step.Request.Channels, step.Request.Frames = s.source.Rate, s.source.Channels, s.source.Ring.Cap()
	case streamStatus:
		step.Op = StreamStatus
	case streamControlAck:
		step.Op, step.Request.Control = StreamControlAck, s.control
	case streamWrite:
		step.Op = StreamWrite
		step.PCM = s.buffer[:s.offered*s.source.Channels]
		step.Request.Samples = len(step.PCM)
	case streamEnd:
		step.Op = TryStreamEnd
	default:
		return StreamStep{}, false, ErrTransfer
	}
	s.pending = step.Token
	return step, true, nil
}

func (s *StreamSender) status(raw []byte) error {
	var wire struct {
		Buffered *int    `json:"buffered"`
		Free     *int    `json:"free"`
		Consumed *uint64 `json:"consumed,string"`
		Held     *bool   `json:"held"`
		Skip     *int64  `json:"skip,string"`
		Control  *uint32 `json:"control"`
		EOF      *bool   `json:"eof"`
	}
	if err := decode(raw, &wire); err != nil || wire.Buffered == nil || wire.Free == nil || wire.Consumed == nil || wire.Held == nil ||
		wire.Skip == nil || wire.Control == nil || wire.EOF == nil {
		return ErrTransfer
	}
	status := mix.RemoteStreamStatus{Buffered: *wire.Buffered, Free: *wire.Free, Consumed: *wire.Consumed, Held: *wire.Held,
		Skip: *wire.Skip, Control: *wire.Control, EOF: *wire.EOF}
	if status.Buffered < 0 || status.Free < 0 || status.Buffered > s.source.Ring.Cap() ||
		status.Free != s.source.Ring.Cap()-status.Buffered || status.Skip < 0 ||
		(status.Skip != 0 && (status.Control == 0 || status.Buffered != 0)) || status.EOF != s.ended {
		return ErrTransfer
	}
	if err := s.sender.Reconcile(status.Consumed, status.Buffered); err != nil {
		return err
	}
	if status.Control != 0 {
		s.control = status.Control
		err := s.sender.BeginControl(status.Skip, status.Held)
		if errors.Is(err, mix.ErrRemoteStreamBusy) {
			s.state = streamControlBusy
			return nil
		}
		if err != nil {
			return err
		}
		s.state = streamControlAck
		return nil
	}
	if s.ended || s.closing {
		if s.closing {
			s.final = true
		}
		if s.ended {
			s.state = streamEnded
		} else {
			s.state = streamIdle
		}
		return nil
	}
	capacity := min(len(s.buffer), status.Free*s.source.Channels)
	frames, eof, err := s.sender.Peek(s.buffer[:capacity])
	if err != nil {
		return err
	}
	switch {
	case frames > 0:
		s.offered, s.state = frames, streamWrite
	case eof:
		s.state = streamEnd
	default:
		s.state = streamIdle
	}
	return nil
}

// Ack accepts only a successful reply for the outstanding step. A partial
// stream write acknowledges only its accepted frames, which remain in the
// source until consumption is reconciled. Stale-status EOF may be deferred.
func (s *StreamSender) Ack(token uint64, result []byte) error {
	if s == nil || s.broken || token == 0 || token != s.pending {
		return ErrTransfer
	}
	var err error
	switch s.state {
	case streamRegister, streamControlAck:
		if len(result) != 0 {
			err = ErrTransfer
		} else {
			s.control = 0
			if s.ended {
				s.state = streamEnded
			} else {
				s.state = streamIdle
			}
			if s.closing {
				s.final = true
			}
		}
	case streamStatus:
		err = s.status(result)
	case streamWrite:
		var reply struct {
			Frames *int `json:"frames"`
		}
		if err = decode(result, &reply); err == nil && (reply.Frames == nil || *reply.Frames < 0 || *reply.Frames > s.offered) {
			err = ErrTransfer
		}
		if err == nil {
			err = s.sender.Ack(*reply.Frames)
		}
		if err == nil {
			s.offered, s.state = 0, streamIdle
		}
	case streamEnd:
		var reply struct {
			Ended *bool `json:"ended"`
		}
		if err = decode(result, &reply); err == nil && reply.Ended == nil {
			err = ErrTransfer
		}
		if err == nil {
			if *reply.Ended {
				s.ended, s.state = true, streamEnded
			} else {
				s.state = streamIdle
			}
		}
	default:
		err = ErrTransfer
	}
	if err != nil {
		s.broken = true
		return err
	}
	s.pending = 0
	return nil
}

// Close relinquishes an unregistered source, or a quiescent source after
// FinishAfterStop and after the owner dropped it from the worker. For uncertain
// worker failure, terminate the transport and use AbortAfterTermination.
func (s *StreamSender) Close() error {
	if s == nil || s.broken || s.pending != 0 || (s.state != streamRegister && !s.ReadyToClose()) {
		return ErrTransfer
	}
	if err := s.sender.Close(); err != nil {
		return err
	}
	s.state = streamClosed
	return nil
}

// AbortAfterTermination releases the producer claim after the owner has
// terminated the worker and stopped or otherwise reconciled its decoder.
// Any unacknowledged PCM remains in the local ring. Some of it might already
// have been accepted or played remotely, so it must not be automatically
// replayed as though the playback position were known. A producer currently
// finishing a write may require retrying this method after ErrRemoteStreamBusy.
func (s *StreamSender) AbortAfterTermination() error {
	if s == nil || s.state == streamClosed {
		return ErrTransfer
	}
	if s.state == streamControlBusy {
		if err := s.sender.FinishControl(); err != nil {
			return err
		}
		s.state = streamControlAck
	}
	if s.offered != 0 {
		if err := s.sender.AbortTransfer(); err != nil {
			return err
		}
		s.offered = 0
	}
	if err := s.sender.Close(); err != nil {
		return err
	}
	s.pending, s.state = 0, streamClosed
	return nil
}
