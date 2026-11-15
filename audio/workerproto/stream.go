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

// StreamStep is ordered; copy PCM before sending, and retain accepted frames until consumption is reconciled.
type StreamStep struct {
	Token   uint64
	Op      int32
	Request Request
	PCM     []float32
}

// StreamSender connects producer and remote rings; the owner serializes calls, polls, orders replies and handles failures without changing ring identity or format.
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

// Poll starts one status/transfer cycle, including after EOF; finish it before polling again and do not spin on an empty decoder.
func (s *StreamSender) Poll() error {
	if s == nil || s.broken || s.closing || (s.state != streamIdle && s.state != streamEnded) || s.pending != 0 {
		return ErrTransfer
	}
	s.state = streamStatus
	return nil
}

// Idle means the cycle completed; retry pending controls before polling or closing while a producer finishes writing.
func (s *StreamSender) Idle() bool {
	return s != nil && !s.broken && s.pending == 0 && (s.state == streamIdle || s.state == streamEnded)
}

// FinishAfterStop reconciles stopped voices; forbid further voice use, defer writes/removal until ack, then drop the worker source before Close.
func (s *StreamSender) FinishAfterStop() error {
	if s == nil || s.broken || s.closing || (s.state != streamIdle && s.state != streamEnded) || s.pending != 0 {
		return ErrTransfer
	}
	s.closing, s.state = true, streamStatus
	return nil
}

// ReadyToClose reports reconciled status and controls; drop the quiescent worker source before Close.
func (s *StreamSender) ReadyToClose() bool {
	return s != nil && !s.broken && s.closing && s.final && s.pending == 0
}

// Ended means EOF was accepted, not that queued audio played or referenced sources can be dropped.
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

// Ack accepts a successful outstanding reply; partial writes retain accepted frames until reconciled, and stale-status EOF may be deferred.
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

// Close relinquishes an unregistered source or one reconciled by FinishAfterStop and dropped by the worker; use AbortAfterTermination after uncertain failures.
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

// AbortAfterTermination releases the producer after stopping or reconciling its decoder; never replay uncertain PCM, and retry ErrRemoteStreamBusy while a write finishes.
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
