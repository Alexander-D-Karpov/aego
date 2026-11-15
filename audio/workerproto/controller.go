package workerproto

import (
	"errors"
	"math"

	"aego/audio/mix"
)

// ErrController means inconsistent transport or source lifetime; stop the worker without locally replaying uncertain commands.
var ErrController = errors.New("audio worker: invalid controller state")

// ControllerStep is a single-flight operation; chunks use memory-chunk, and PCM borrowed until Ack must be copied before sending.
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
	flightSkip
)

// Bound each silent reactor call and retain the remainder for later acknowledgements without enlarging audio buffers.
const (
	maxSkipFrames     = 1 << 20
	maxSkipBoundaries = 128
)

// before counts commands published before this boundary so later commands cannot overtake its silent advancement.
type skipBoundary struct {
	before int
	frames int64
}

// Controller is created before mixer render/skip; its owner serializes calls, polls streams, orders batches and stops on failure without replaying uncertain commands.
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
	skipFrames   int64
	skips        [maxSkipBoundaries]skipBoundary
	skipHead     int
	skipCount    int
	skipped      bool
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

// CancelUnsent permits fallback before commands, batches or skips cross, after stopping the worker and excluding concurrent producers.
func (c *Controller) CancelUnsent() error {
	if c == nil || c.failed || c.client == nil || c.command != nil || c.flight.step.Token != 0 || c.skipCount != 0 || c.skipped || len(c.sources) != 0 {
		return ErrController
	}
	if err := c.client.CancelUnsent(); err != nil {
		return err
	}
	c.client = nil
	return nil
}

// QueueSkip preserves command order; first quiesce and flush old PCM, serialize with Next/Ack, and use zero to drain without advancing.
func (c *Controller) QueueSkip(frames int64) error {
	if c == nil || c.failed || c.client == nil || frames < 0 || frames > math.MaxInt64-c.skipFrames {
		return ErrController
	}
	before := c.client.Queued()
	if before < 0 {
		return ErrController
	}
	if c.skipCount > 0 {
		last := (c.skipHead + c.skipCount - 1) % len(c.skips)
		if c.skips[last].before == before {
			c.skips[last].frames += frames
			c.skipFrames += frames
			return nil
		}
	}
	if c.skipCount == len(c.skips) {
		return ErrController
	}
	at := (c.skipHead + c.skipCount) % len(c.skips)
	c.skips[at] = skipBoundary{before: before, frames: frames}
	c.skipCount++
	c.skipFrames += frames
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
	// Let the mixer reject both-or-neither sources with its normal release/event.
	if (cmd.Memory == nil) == (cmd.Stream == nil) {
		return nil, nil
	}
	// Preserve rejected-play events for formats RegisterStream cannot represent.
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

// PollStreams coalesces requests for one transfer/status cycle per live stream; Next and Ack perform the work.
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

// Idle snapshots acknowledged work; serialize it with Next, Ack and QueueSkip, as producers may append later.
func (c *Controller) Idle() bool {
	if c == nil || c.failed || c.client == nil || c.flight.step.Token != 0 || c.command != nil ||
		c.cleanup != nil || c.skipCount != 0 || c.client.Queued() != 0 {
		return false
	}
	for _, s := range c.sources {
		if s.poll || s.polling || (s.registered && s.refs == 0) {
			return false
		}
	}
	return true
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

// Next returns one operation at most; Ack replies before retrying, and wake on commands, stream polls or producers rather than spinning.
func (c *Controller) Next() (ControllerStep, bool, error) {
	if c == nil || c.failed || c.client == nil || c.flight.step.Token != 0 {
		return ControllerStep{}, false, ErrController
	}
	if c.cleanup != nil {
		return c.nextCleanup()
	}
	// Service requested stream cycles during large transfers to avoid starving existing voices.
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
		// Retire released sources under load; later plays can register them with new IDs.
		if step, ok, err := c.beginCleanup(); ok || err != nil || c.cleanup != nil {
			return step, ok, err
		}
		if c.skipCount > 0 && c.skips[c.skipHead].before == 0 {
			skip := &c.skips[c.skipHead]
			frames := min(skip.frames, int64(maxSkipFrames))
			skip.frames -= frames
			c.skipFrames -= frames
			if skip.frames == 0 {
				*skip = skipBoundary{}
				c.skipHead = (c.skipHead + 1) % len(c.skips)
				c.skipCount--
			}
			// Once sent, even an empty skip makes startup-only fallback unsafe.
			c.skipped = true
			return c.issue(flightSkip, nil, 0, ControllerStep{Op: Skip, Request: Request{Frames: int(frames)}})
		}
		command, ok, err := c.client.Next()
		if err != nil {
			return ControllerStep{}, false, c.fail(err)
		}
		if !ok {
			if c.skipCount > 0 {
				return ControllerStep{}, false, c.fail(ErrController)
			}
			return ControllerStep{}, false, nil
		}
		for n := 0; n < c.skipCount; n++ {
			i := (c.skipHead + n) % len(c.skips)
			if c.skips[i].before <= 0 {
				return ControllerStep{}, false, c.fail(ErrController)
			}
			c.skips[i].before--
		}
		c.command = &command
		if command.Command.Kind == mix.CmdPlay || command.Command.Kind == mix.CmdReplaceStream {
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
	if cmd.Command.Kind == mix.CmdPlay || cmd.Command.Kind == mix.CmdReplaceStream {
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
		// Wait for initial status while a producer finishes a write or control.
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

// Ack applies a successful outstanding response; terminate on failure, malformed data or uncertainty.
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
	case flightSkip:
		if len(result) != 0 {
			err = ErrController
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

// ApplyBatch validates releases before applying the batch; acknowledge the worker only on success, not from events alone.
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

// AbortAfterTermination releases stopped or reconciled streams without restoring position; retry while a producer finishes writing.
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
