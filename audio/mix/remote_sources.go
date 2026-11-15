package mix

import "errors"

var (
	ErrRemoteSource       = errors.New("mix: invalid or missing remote source")
	ErrRemoteSourceBudget = errors.New("mix: remote source memory budget exceeded")
	ErrRemoteSourceInUse  = errors.New("mix: remote source is still in use")
)

const (
	maxRemoteStreamFrames = 1 << 20
	maxRemoteSources      = 4096
)

type remoteSource struct {
	memory      *Memory
	stream      *Stream
	bytes       int64
	refs        int
	ended       bool
	ready       bool
	controlSeq  uint32
	control     uint32
	controlSkip int64
	controlHeld bool
	appliedHeld bool
}

// RemoteSourceCommand uses IDs for play/stream replacement and zero otherwise or for invalid sources; transport orders commands and copies samples before registration.
type RemoteSourceCommand struct {
	RemoteCommand
	Source uint64
}

type RemoteStreamStatus struct {
	Buffered int    `json:"buffered"`
	Free     int    `json:"free"`
	Consumed uint64 `json:"consumed,string"`
	Held     bool   `json:"held"`
	Skip     int64  `json:"skip,string"`
	Control  uint32 `json:"control"`
	EOF      bool   `json:"eof"`
}

// RemoteSources owns remote memory/streams; its owner manages bounded transfer, flow control, lifecycle, ordered batches and failures.
type RemoteSources struct {
	mixer   *RemoteMixer
	budget  int64
	used    int64
	sources map[uint64]*remoteSource
	voices  map[VoiceID]uint64
	broken  bool
}

func NewRemoteSources(cfg Config, budget int64) (*RemoteSources, error) {
	if budget <= 0 {
		return nil, ErrRemoteSourceBudget
	}
	m, err := NewRemoteMixer(cfg)
	if err != nil {
		return nil, err
	}
	return &RemoteSources{mixer: m, budget: budget, sources: make(map[uint64]*remoteSource), voices: make(map[VoiceID]uint64)}, nil
}

func (r *RemoteSources) add(id uint64, bytes int64) error {
	if r == nil || r.broken || id == 0 || bytes < 0 {
		return ErrRemoteSource
	}
	if _, exists := r.sources[id]; exists {
		return ErrRemoteSource
	}
	if len(r.sources) >= maxRemoteSources {
		return ErrRemoteSourceBudget
	}
	if bytes > r.budget-r.used {
		return ErrRemoteSourceBudget
	}
	return nil
}

// RegisterMemory copies PCM so later sender changes cannot mutate remote playback.
func (r *RemoteSources) RegisterMemory(id uint64, m *Memory) error {
	if m == nil {
		return ErrRemoteSource
	}
	bytes := int64(len(m.PCM)) * 4
	if err := r.add(id, bytes); err != nil {
		return err
	}
	copy := *m
	copy.PCM = append([]float32(nil), m.PCM...)
	r.sources[id] = &remoteSource{memory: &copy, bytes: bytes, ready: true}
	r.used += bytes
	return nil
}

// BeginMemory allocates worker PCM for direct staged transfer; play requires CommitMemory, cancellation uses Drop, and the slice must not be retained or changed after commit.
func (r *RemoteSources) BeginMemory(id uint64, desc Memory, samples int) ([]float32, error) {
	if samples < 0 || int64(samples) > int64(^uint32(0))/4 || len(desc.PCM) != 0 {
		return nil, ErrRemoteSource
	}
	bytes := int64(samples) * 4
	if err := r.add(id, bytes); err != nil {
		return nil, err
	}
	desc.PCM = make([]float32, samples)
	r.sources[id] = &remoteSource{memory: &desc, bytes: bytes}
	r.used += bytes
	return desc.PCM, nil
}

func (r *RemoteSources) CommitMemory(id uint64) error {
	if r == nil || r.broken {
		return ErrRemoteSource
	}
	s, ok := r.sources[id]
	if !ok || s.memory == nil || s.ready {
		return ErrRemoteSource
	}
	s.ready = true
	return nil
}

// RegisterStream allocates a bounded remote ring and cache; retain accepted PCM until consumption is reconciled.
func (r *RemoteSources) RegisterStream(id uint64, rate, channels, frames int) error {
	if rate <= 0 || (channels != 1 && channels != 2) || frames <= 0 || frames > maxRemoteStreamFrames {
		return ErrRemoteSource
	}
	ringSamples := 2
	for ringSamples < frames*channels {
		ringSamples <<= 1
	}
	bytes := int64(ringSamples+streamCache*channels) * 4
	if err := r.add(id, bytes); err != nil {
		return err
	}
	r.sources[id] = &remoteSource{stream: NewStream(rate, channels, frames), bytes: bytes, ready: true}
	r.used += bytes
	return nil
}

func (r *RemoteSources) stream(id uint64) (*remoteSource, error) {
	if r == nil || r.broken {
		return nil, ErrRemoteSource
	}
	s, ok := r.sources[id]
	if !ok || s.stream == nil {
		return nil, ErrRemoteSource
	}
	return s, nil
}

// WriteStream returns accepted complete frames; partial writes do not acknowledge the remainder.
func (r *RemoteSources) WriteStream(id uint64, samples []float32) (int, error) {
	s, err := r.stream(id)
	if err != nil || s.ended {
		return 0, ErrRemoteSource
	}
	if len(samples)%s.stream.Channels != 0 {
		return 0, ErrRemoteSource
	}
	// Block stale PCM across skip acknowledgement and until the sender observes a resume.
	held := s.stream.Held()
	if s.control != 0 || s.stream.skipped.Load() > 0 || held || held != s.appliedHeld {
		return 0, nil
	}
	return s.stream.Ring.Write(samples), nil
}

func (r *RemoteSources) EndStream(id uint64) error {
	ended, err := r.TryEndStream(id)
	if err != nil || !ended {
		return ErrRemoteSource
	}
	return nil
}

// TryEndStream defers EOF for a later skip/hold; reconcile the control and acknowledge earlier PCM before retrying.
func (r *RemoteSources) TryEndStream(id uint64) (bool, error) {
	s, err := r.stream(id)
	if err != nil || s.ended {
		return false, ErrRemoteSource
	}
	if s.control != 0 || s.stream.skipped.Load() > 0 || s.stream.Held() != s.appliedHeld {
		return false, nil
	}
	s.ended = true
	s.stream.SetEOF()
	return true, nil
}

// StreamStatus holds each numbered control until ack; serialize PCM and controls, apply once after prior replies, then ack before new PCM or EOF.
func (r *RemoteSources) StreamStatus(id uint64) (RemoteStreamStatus, error) {
	s, err := r.stream(id)
	if err != nil {
		return RemoteStreamStatus{}, err
	}
	if s.control == 0 {
		held := s.stream.Held()
		if s.stream.skipped.Load() > 0 || held != s.appliedHeld {
			if s.controlSeq == ^uint32(0) {
				return RemoteStreamStatus{}, ErrRemoteSource
			}
			s.controlSeq++
			s.control = s.controlSeq
			s.controlSkip = s.stream.TakeSkip()
			s.controlHeld = held
		}
	}
	held := s.appliedHeld
	if s.control != 0 {
		held = s.controlHeld
	}
	return RemoteStreamStatus{Buffered: s.stream.Ring.Len(), Free: s.stream.Ring.Free(),
		Consumed: s.stream.Ring.head.Load() / uint64(s.stream.Channels),
		Held:     held, Skip: s.controlSkip, Control: s.control, EOF: s.ended}, nil
}

// AckStreamControl admits data after the matching control and rejects duplicate, stale or fabricated acknowledgements.
func (r *RemoteSources) AckStreamControl(id uint64, control uint32) error {
	s, err := r.stream(id)
	if err != nil || control == 0 || control != s.control {
		return ErrRemoteSource
	}
	s.appliedHeld = s.controlHeld
	s.control, s.controlSkip = 0, 0
	return nil
}

func (r *RemoteSources) Accept(p RemoteSourceCommand) error {
	if r == nil || r.broken || p.Command.Memory != nil || p.Command.Stream != nil {
		return ErrRemoteSource
	}
	var source *remoteSource
	if p.Command.Kind == CmdPlay || p.Command.Kind == CmdReplaceStream {
		if p.Source != 0 {
			var ok bool
			source, ok = r.sources[p.Source]
			if !ok || !source.ready {
				return ErrRemoteSource
			}
			p.Command.Memory, p.Command.Stream = source.memory, source.stream
		}
		if _, exists := r.voices[p.Command.Voice]; exists {
			return errRemoteProtocol
		}
	} else if p.Source != 0 {
		return ErrRemoteSource
	}
	if err := r.mixer.Accept(p.RemoteCommand); err != nil {
		return err
	}
	if p.Command.Kind == CmdPlay || p.Command.Kind == CmdReplaceStream {
		if source != nil {
			source.refs++
		}
		r.voices[p.Command.Voice] = p.Source
	}
	return nil
}

func (r *RemoteSources) Mix(out []float32) error {
	if r == nil || r.broken {
		return errRemoteProtocol
	}
	return r.mixer.Mix(out)
}

func (r *RemoteSources) Skip(frames int) error {
	if r == nil || r.broken {
		return errRemoteProtocol
	}
	return r.mixer.Skip(frames)
}

func (r *RemoteSources) SetSampleRate(rate int) error {
	if r == nil || r.broken {
		return errRemoteProtocol
	}
	return r.mixer.SetSampleRate(rate)
}

func (r *RemoteSources) Drain() (RemoteBatch, error) {
	if r == nil || r.broken {
		return RemoteBatch{}, errRemoteProtocol
	}
	b, err := r.mixer.Drain()
	if err != nil {
		return RemoteBatch{}, err
	}
	for _, voice := range b.Released {
		id, ok := r.voices[voice]
		if !ok {
			r.broken = true
			return RemoteBatch{}, errRemoteProtocol
		}
		if id != 0 {
			s := r.sources[id]
			if s == nil || s.refs <= 0 {
				r.broken = true
				return RemoteBatch{}, errRemoteProtocol
			}
			s.refs--
		}
		delete(r.voices, voice)
	}
	return b, nil
}

// Drop is explicit and cannot invalidate a queued or playing voice.
func (r *RemoteSources) Drop(id uint64) error {
	if r == nil || r.broken {
		return ErrRemoteSource
	}
	s, ok := r.sources[id]
	if !ok {
		return ErrRemoteSource
	}
	if s.refs != 0 {
		return ErrRemoteSourceInUse
	}
	delete(r.sources, id)
	r.used -= s.bytes
	return nil
}
