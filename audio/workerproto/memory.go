package workerproto

import (
	"errors"

	"aego/audio/mix"
)

// MaxTransferSamples is the maximum PCM chunk accepted by the Worker. A
// caller must copy each chunk into its message before acknowledging the step.
const MaxTransferSamples = 1 << 16

var ErrTransfer = errors.New("audio worker: invalid memory transfer state")

// MemoryStep is one operation in a staged transfer. Chunk steps use the
// Worker's "memory-chunk" operation instead of a numeric reactor operation.
type MemoryStep struct {
	Token   uint64
	Op      int32
	Request Request
	Chunk   bool
	Offset  int
	PCM     []float32
}

// MemorySender emits one acknowledged operation at a time. The source slice
// and its samples must remain immutable for the transfer, just as while the
// native mixer is reading them. It does not own a worker or retry an operation
// whose delivery is uncertain.
type MemorySender struct {
	id      uint64
	desc    mix.Memory
	chunk   int
	offset  int
	state   uint8
	serial  uint64
	pending uint64
	length  int
}

func NewMemorySender(id uint64, source *mix.Memory, chunk int) (*MemorySender, error) {
	if id == 0 || source == nil || chunk <= 0 || chunk > MaxTransferSamples {
		return nil, ErrTransfer
	}
	return &MemorySender{id: id, desc: *source, chunk: chunk}, nil
}

// Next refuses a second outstanding operation. An acknowledged final commit
// returns ok=false; this does not mean the source has been played or released.
func (s *MemorySender) Next() (step MemoryStep, ok bool, err error) {
	if s == nil || s.pending != 0 || s.state > 3 || s.serial == ^uint64(0) {
		return step, false, ErrTransfer
	}
	if s.state == 3 {
		return step, false, nil
	}
	if s.state == 1 && s.offset == len(s.desc.PCM) {
		s.state = 2
	}
	s.serial++
	step.Token = s.serial
	switch s.state {
	case 0:
		step.Op = BeginMemory
		step.Request = Request{Source: s.id, Rate: s.desc.Rate, Channels: s.desc.Channels,
			Samples: len(s.desc.PCM), LoopStart: s.desc.LoopStart, LoopEnd: s.desc.LoopEnd, Loop: s.desc.Loop}
	case 1:
		s.length = min(s.chunk, len(s.desc.PCM)-s.offset)
		step.Chunk, step.Offset = true, s.offset
		step.PCM = s.desc.PCM[s.offset : s.offset+s.length]
	case 2:
		step.Op = CommitMemory
		step.Request.Source = s.id
	}
	s.pending = step.Token
	return step, true, nil
}

// Ack accepts only the successful reply to the outstanding operation. If a
// worker or connection fails, the owner must terminate or reconcile it rather
// than acknowledging or blindly retrying an operation that may have happened.
func (s *MemorySender) Ack(token uint64) error {
	if s == nil || token == 0 || token != s.pending {
		return ErrTransfer
	}
	s.pending = 0
	switch s.state {
	case 0:
		s.state = 1
	case 1:
		s.offset += s.length
		s.length = 0
	case 2:
		s.state = 3
	default:
		return ErrTransfer
	}
	return nil
}
