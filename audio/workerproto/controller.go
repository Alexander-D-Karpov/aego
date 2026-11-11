package workerproto

import (
	"errors"

	"aego/audio/mix"
)

// ErrController means that the ordered transport or its source lifetime is
// inconsistent. The owner must stop the worker; it must not replay uncertain
// commands through a local mixer.
var ErrController = errors.New("audio worker: invalid controller state")

// ControllerStep is one operation for an ordered, single-flight transport.
// Chunk steps use the Worker's memory-chunk message rather than a numeric op.
// PCM is borrowed until Ack; the transport must copy it before sending.
type ControllerStep struct {
	Token   uint64
	Op      int32
	Request Request
	Chunk   bool
	Offset  int
	PCM     []float32
}

type controlledSource struct {
	id         uint64
	memory     *mix.Memory
	stream     *mix.Stream
	memSender  *MemorySender
	streamSend *StreamSender
	refs       int
	registered bool
	primed     bool
	poll       bool
	polling    bool
	closing    bool
	aborted    bool
}

type controlledVoice struct {
	source   *controlledSource
	sequence uint64
}

type controllerFlight struct {
	step   ControllerStep
	source *controlledSource
	sub    uint64
	kind   uint8
}

const (
	flightMemory uint8 = iota + 1
	flightStream
	flightCommand
	flightDrop
)

// Controller coordinates commands, bounded source transfers, batches and
// source release. It does not create a worker, attach an output or install a
// timer. Its owner serializes all calls and must feed batches in order, request
// stream polls regularly and terminate the transport on failure.
//
// It must be constructed before the mixer renders or skips. Once a command is
// taken from the mixer, playback may have advanced remotely: automatic local
// fallback is unsafe. This controller is not yet a browser output driver.
type Controller struct {
	client       *mix.RemoteClient
	memory       map[*mix.Memory]*controlledSource
	streams      map[*mix.Stream]*controlledSource
	sources      []*controlledSource
	voices       map[mix.VoiceID]controlledVoice
	command      *mix.RemoteCommand
	cleanup      *controlledSource
	flight       controllerFlight
	nextID       uint64
	serial       uint64
	streamCursor int
	failed       bool
}

func NewController(m *mix.Mixer) (*Controller, error) {
	client, err := mix.NewRemoteClient(m)
	if err != nil {
		return nil, err
	}
	return &Controller{client: client, memory: make(map[*mix.Memory]*controlledSource),
		streams: make(map[*mix.Stream]*controlledSource), voices: make(map[mix.VoiceID]controlledVoice)}, nil
}

// CancelUnsent permits local startup fallback only before a command or batch
// has crossed this controller. The owner must first stop the worker and exclude
// concurrent mixer producers, as required by mix.RemoteClient.CancelUnsent.
func (c *Controller) CancelUnsent() error {
	if c == nil || c.failed || c.client == nil || c.command != nil || c.flight.step.Token != 0 || len(c.sources) != 0 {
		return ErrController
	}
	if err := c.client.CancelUnsent(); err != nil {
		return err
	}
	c.client = nil
	return nil
}

func (c *Controller) fail(err error) error {
	c.failed = true
	if err == nil {
		return ErrController
	}
	return err
}

func (c *Controller) newSource(cmd mix.Command) (*controlledSource, error) {
	// The mixer rejects the both-or-neither case itself, including its normal
	// release/event. Do not turn it into a valid worker source by guessing.
	if (cmd.Memory == nil) == (cmd.Stream == nil) {
		return nil, nil
	}
	// These formats are rejected by the mixer before it touches a stream's
	// ring. RegisterStream cannot represent them, so preserve the ordinary
	// rejected-play event by sending the existing invalid-source sentinel.
	if cmd.Stream != nil && (cmd.Stream.Rate <= 0 || (cmd.Stream.Channels != 1 && cmd.Stream.Channels != 2)) {
		return nil, nil
	}
	if cmd.Memory != nil {
		if s := c.memory[cmd.Memory]; s != nil {
			return s, nil
		}
	} else if s := c.streams[cmd.Stream]; s != nil {
		return s, nil
	}
	if c.nextID == ^uint64(0) {
		return nil, ErrController
	}
	c.nextID++
	s := &controlledSource{id: c.nextID, memory: cmd.Memory, stream: cmd.Stream}
	var err error
	if s.memory != nil {
		s.memSender, err = NewMemorySender(s.id, s.memory, MaxTransferSamples)
	} else {
		s.streamSend, err = NewStreamSender(s.id, s.stream, MaxTransferSamples/s.stream.Channels)
	}
	if err != nil {
		return nil, err
	}
	if s.memory != nil {
		c.memory[s.memory] = s
	} else {
		c.streams[s.stream] = s
	}
	c.sources = append(c.sources, s)
	return s, nil
}

// PollStreams requests one transfer/status cycle for every registered live
// stream. Repeated requests coalesce; Next and Ack perform the actual work.
func (c *Controller) PollStreams() error {
	if c == nil || c.failed || c.client == nil {
		return ErrController
	}
	for _, s := range c.sources {
		if s.streamSend != nil && s.registered && s.refs > 0 && !s.closing {
			s.poll = true
		}
	}
	return nil
}

func (c *Controller) issue(kind uint8, s *controlledSource, sub uint64, step ControllerStep) (ControllerStep, bool, error) {
	if c.serial == ^uint64(0) {
		return ControllerStep{}, false, c.fail(ErrController)
	}
	c.serial++
	step.Token = c.serial
	c.flight = controllerFlight{step: step, source: s, sub: sub, kind: kind}
	return step, true, nil
}

func (c *Controller) streamStep(s *controlledSource) (ControllerStep, bool, error) {
	step, ok, err := s.streamSend.Next()
	if err != nil {
		return ControllerStep{}, false, c.fail(err)
	}
	if !ok {
		return ControllerStep{}, false, nil
	}
	return c.issue(flightStream, s, step.Token, ControllerStep{Op: step.Op, Request: step.Request, PCM: step.PCM})
}

// Next returns at most one operation. After every successful reply the owner
// must call Ack before requesting another operation. An idle result is not an
// invitation to spin: retry on a command wake, stream poll or producer wake.
func (c *Controller) Next() (ControllerStep, bool, error) {
	if c == nil || c.failed || c.client == nil || c.flight.step.Token != 0 {
		return ControllerStep{}, false, ErrController
	}
	if c.cleanup != nil {
		return c.nextCleanup()
	}
	// Service previously requested stream cycles even during a large memory
	// transfer. This avoids starving an existing voice while copying a new clip.
	for n := 0; n < len(c.sources); n++ {
		i := (c.streamCursor + n) % len(c.sources)
		s := c.sources[i]
		if s.streamSend == nil || !s.registered || s.closing || (!s.poll && !s.polling) {
			continue
		}
		if !s.polling {
			if err := s.streamSend.Poll(); err != nil {
				return ControllerStep{}, false, c.fail(err)
			}
			s.poll, s.polling = false, true
		}
		step, ok, err := c.streamStep(s)
		if err != nil || ok {
			if ok {
				c.streamCursor = (i + 1) % len(c.sources)
			}
			return step, ok, err
		}
		if s.streamSend.Idle() {
			s.polling, s.primed = false, true
		}
	}
	if c.command == nil {
		// Retire released sources even under a continuous command load. A
		// later queued play can register the same Go source again with a new ID.
		if step, ok, err := c.beginCleanup(); ok || err != nil || c.cleanup != nil {
			return step, ok, err
		}
		command, ok, err := c.client.Next()
		if err != nil {
			return ControllerStep{}, false, c.fail(err)
		}
		if !ok {
			return ControllerStep{}, false, nil
		}
		c.command = &command
		if command.Command.Kind == mix.CmdPlay {
			s, err := c.newSource(command.Command)
			if err != nil {
				return ControllerStep{}, false, c.fail(err)
			}
			if _, exists := c.voices[command.Command.Voice]; exists {
				return ControllerStep{}, false, c.fail(ErrController)
			}
			c.voices[command.Command.Voice] = controlledVoice{source: s, sequence: command.Sequence}
			if s != nil {
				s.refs++
			}
		}
	}
	cmd := *c.command
	var s *controlledSource
	if cmd.Command.Kind == mix.CmdPlay {
		s = c.voices[cmd.Command.Voice].source
	}
	if s != nil && !s.registered {
		if s.memSender != nil {
			step, ok, err := s.memSender.Next()
			if err != nil || !ok {
				return ControllerStep{}, false, c.fail(err)
			}
			return c.issue(flightMemory, s, step.Token, ControllerStep{Op: step.Op, Request: step.Request,
				Chunk: step.Chunk, Offset: step.Offset, PCM: step.PCM})
		}
		return c.streamStep(s)
	}
	if s != nil && s.streamSend != nil && !s.primed {
		// A producer can be finishing a write or control. Do not deliver Play
		// until the initial ordered status cycle has completed.
		return ControllerStep{}, false, nil
	}
	var id uint64
	if s != nil {
		id = s.id
	}
	return c.issue(flightCommand, s, 0, ControllerStep{Op: Command,
		Request: Request{Command: CommandFromMix(mix.RemoteSourceCommand{RemoteCommand: cmd, Source: id})}})
}

func (c *Controller) beginCleanup() (ControllerStep, bool, error) {
	for _, s := range c.sources {
		if s.refs != 0 || !s.registered || s.polling {
			continue
		}
		c.cleanup = s
		return c.nextCleanup()
	}
	return ControllerStep{}, false, nil
}

func (c *Controller) nextCleanup() (ControllerStep, bool, error) {
	s := c.cleanup
	if s == nil || s.refs != 0 {
		return ControllerStep{}, false, c.fail(ErrController)
	}
	if s.streamSend != nil {
		if !s.closing {
			if err := s.streamSend.FinishAfterStop(); err != nil {
				return ControllerStep{}, false, c.fail(err)
			}
			s.closing = true
		}
		if !s.streamSend.ReadyToClose() {
			return c.streamStep(s)
		}
	}
	return c.issue(flightDrop, s, 0, ControllerStep{Op: Drop, Request: Request{Source: s.id}})
}

// Ack applies a successful response to the unique outstanding step. A failed,
// malformed or uncertain response must instead be followed by termination.
func (c *Controller) Ack(token uint64, result []byte) error {
	if c == nil || c.failed || c.client == nil {
		return ErrController
	}
	if token == 0 || c.flight.step.Token != token {
		return c.fail(ErrController)
	}
	f := c.flight
	var err error
	switch f.kind {
	case flightMemory:
		if len(result) != 0 {
			err = ErrController
		} else if err = f.source.memSender.Ack(f.sub); err == nil && f.step.Op == CommitMemory {
			f.source.registered, f.source.primed = true, true
		}
	case flightStream:
		err = f.source.streamSend.Ack(f.sub, result)
		if err == nil && f.step.Op == Stream {
			f.source.registered, f.source.poll = true, true
		}
	case flightCommand:
		if len(result) != 0 || c.command == nil {
			err = ErrController
		} else {
			c.command = nil
		}
	case flightDrop:
		if len(result) != 0 || c.cleanup != f.source || f.source.refs != 0 {
			err = ErrController
		} else {
			if f.source.streamSend != nil {
				err = f.source.streamSend.Close()
			}
			if err == nil {
				c.removeSource(f.source)
				c.cleanup = nil
			}
		}
	default:
		err = ErrController
	}
	if err != nil {
		return c.fail(err)
	}
	c.flight = controllerFlight{}
	return nil
}

func (c *Controller) removeSource(s *controlledSource) {
	delete(c.memory, s.memory)
	delete(c.streams, s.stream)
	for i, old := range c.sources {
		if old == s {
			copy(c.sources[i:], c.sources[i+1:])
			c.sources[len(c.sources)-1] = nil
			c.sources = c.sources[:len(c.sources)-1]
			return
		}
	}
}

// ApplyBatch validates source releases before applying the complete mixer
// batch. The transport must acknowledge the batch to the worker only after this
// method succeeds; events alone are not a reliable source-lifetime signal.
func (c *Controller) ApplyBatch(batch mix.RemoteBatch) error {
	if c == nil || c.failed || c.client == nil {
		return ErrController
	}
	seen := make(map[mix.VoiceID]struct{}, len(batch.Released))
	validVoice := func(id mix.VoiceID) bool {
		v, ok := c.voices[id]
		return ok && v.sequence <= batch.Applied
	}
	for _, voice := range batch.Released {
		v := c.voices[voice]
		if _, duplicate := seen[voice]; !validVoice(voice) || duplicate || (v.source != nil && v.source.refs <= 0) {
			return c.fail(ErrController)
		}
		seen[voice] = struct{}{}
	}
	for _, event := range batch.Events {
		if !validVoice(event.Voice) {
			return c.fail(ErrController)
		}
	}
	for _, voice := range batch.Voices {
		if !validVoice(voice.ID) {
			return c.fail(ErrController)
		}
	}
	if err := c.client.Apply(batch); err != nil {
		return c.fail(err)
	}
	for _, voice := range batch.Released {
		if s := c.voices[voice].source; s != nil {
			s.refs--
		}
		delete(c.voices, voice)
	}
	return nil
}

// AbortAfterTermination releases attached stream claims only after the caller
// has terminated the worker and stopped or reconciled the decoder. It does not
// restore playback position. Retry if a producer is finishing a ring write.
func (c *Controller) AbortAfterTermination() error {
	if c == nil || c.client == nil {
		return ErrController
	}
	c.failed = true
	for _, s := range c.sources {
		if s.streamSend == nil || s.aborted {
			continue
		}
		if err := s.streamSend.AbortAfterTermination(); err != nil {
			return err
		}
		s.aborted = true
	}
	c.client = nil
	return nil
}
