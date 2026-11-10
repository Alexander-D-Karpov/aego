package mix

import (
	"errors"
	"math"
	"sync/atomic"
)

var errRemoteProtocol = errors.New("mix: invalid remote mixer protocol")

// RemoteCommand is an ordered command for a mixer running on another thread.
// Memory and Stream are references in Go; a transport must copy or otherwise
// explicitly transfer their data rather than sending Go pointers to a worker.
type RemoteCommand struct {
	Sequence uint64
	Command  Command
}

type RemoteVoice struct {
	ID       VoiceID
	Position float32
	Gain     float32
}

type RemoteBus struct {
	Gain float32
	Peak float32
	RMS  float32
}

// RemoteBatch contains a processed command acknowledgement and an ordered
// release ledger. Releases are independent of Events: the public event queue
// is bounded and is allowed to drop notifications, but handles must not leak.
type RemoteBatch struct {
	Serial        uint64
	Applied       uint64
	Events        []Event
	EventsDropped uint64
	Released      []VoiceID
	Active        int
	Voices        []RemoteVoice
	Buses         []RemoteBus
}

// RemoteClient is the producer-side half of a remote mixer. It must be
// installed before the mixer has rendered or skipped and before concurrent
// producers start. It is the sole consumer of the command queue. Callers must
// not call Mix or Skip on that mixer; the remote mixer owns playback and
// reports its state back in batches.
type RemoteClient struct {
	m       *Mixer
	pending atomic.Int32
	sent    uint64
	applied uint64
	serial  uint64
	dropped uint64
	live    map[VoiceID]struct{}
	scratch []uint8
}

func NewRemoteClient(m *Mixer) (*RemoteClient, error) {
	if m == nil || m.remote || m.applied != 0 || m.clock != 0 {
		return nil, errRemoteProtocol
	}
	c := &RemoteClient{m: m, live: make(map[VoiceID]struct{}), scratch: make([]uint8, len(m.handles))}
	c.pending.Store(int32(m.cmds.Len()))
	m.cmds.remote = &c.pending
	m.remote = true
	return c, nil
}

// CancelUnsent returns an unstarted mixer to local ownership if startup fails
// before any command or batch crossed the transport. Stop and detach the
// worker first. The caller must also exclude concurrent producers and consumers
// while cancelling, as during NewRemoteClient. Pending commands remain in their
// original queue and retain their voice handles.
//
// Once Next or Apply succeeds, the remote may have advanced and this method
// deliberately refuses to guess its playback state. It is not crash recovery.
func (c *RemoteClient) CancelUnsent() error {
	if c == nil || c.m == nil || c.sent != 0 || c.applied != 0 || c.serial != 0 || len(c.live) != 0 ||
		c.m.applied != 0 || c.m.clock != 0 || c.m.cmds.remote != &c.pending ||
		c.pending.Load() != int32(c.m.cmds.q.len()) {
		return errRemoteProtocol
	}
	m := c.m
	m.cmds.remote = nil
	m.remote = false
	c.m = nil
	return nil
}

// Next transfers one command while preserving its place in the bounded queue
// until the remote mixer actually applies it. Commands must be delivered in
// order and acknowledgements must not be fabricated by the transport.
func (c *RemoteClient) Next() (RemoteCommand, bool, error) {
	if c == nil || c.m == nil || c.m.applied != 0 || c.m.clock != 0 {
		return RemoteCommand{}, false, errRemoteProtocol
	}
	cmd, ok := c.m.cmds.q.pop()
	if !ok {
		return RemoteCommand{}, false, nil
	}
	if c.sent == ^uint64(0) {
		return RemoteCommand{}, false, errRemoteProtocol
	}
	if cmd.Kind == CmdPlay {
		if !c.m.Live(cmd.Voice) {
			return RemoteCommand{}, false, errRemoteProtocol
		}
		if _, exists := c.live[cmd.Voice]; exists {
			return RemoteCommand{}, false, errRemoteProtocol
		}
		c.live[cmd.Voice] = struct{}{}
	}
	c.sent++
	return RemoteCommand{Sequence: c.sent, Command: cmd}, true, nil
}

// Apply validates the complete batch before changing any local state. A
// transport failure must be surfaced to its owner; this method cannot infer
// whether an interrupted worker played or discarded audio.
func (c *RemoteClient) Apply(b RemoteBatch) error {
	if c == nil || c.m == nil || c.m.applied != 0 || c.m.clock != 0 ||
		b.Serial != c.serial+1 || b.Applied < c.applied || b.Applied > c.sent ||
		b.EventsDropped < c.dropped || b.Active < 0 || b.Active > len(c.m.voices) ||
		len(b.Buses) != len(c.m.buses) || len(b.Events) > queueSize ||
		len(b.Voices) > len(c.m.voices) || len(b.Released) > len(c.m.handles) {
		return errRemoteProtocol
	}
	delta := b.Applied - c.applied
	if delta > uint64(c.pending.Load()) {
		return errRemoteProtocol
	}
	clear(c.scratch)
	for _, id := range b.Released {
		if _, ok := c.live[id]; !ok || !c.m.Live(id) || c.scratch[id.Index]&1 != 0 {
			return errRemoteProtocol
		}
		c.scratch[id.Index] |= 1
	}
	for _, e := range b.Events {
		if e.Kind < EvFinished || e.Kind > EvStarved {
			return errRemoteProtocol
		}
		if _, ok := c.live[e.Voice]; !ok {
			return errRemoteProtocol
		}
		if e.Kind != EvStarved && c.scratch[e.Voice.Index]&1 == 0 {
			return errRemoteProtocol
		}
	}
	for _, v := range b.Voices {
		if _, ok := c.live[v.ID]; !ok || !c.m.Live(v.ID) || c.scratch[v.ID.Index] != 0 {
			return errRemoteProtocol
		}
		c.scratch[v.ID.Index] = 2
	}
	for _, e := range b.Events {
		c.m.events.push(e)
	}
	c.m.events.dropped.Add(b.EventsDropped - c.dropped)
	c.dropped = b.EventsDropped
	for _, id := range b.Released {
		c.m.release(id.Index)
		delete(c.live, id)
	}
	for _, v := range b.Voices {
		h := &c.m.handles[v.ID.Index]
		h.pos.Store(math.Float32bits(v.Position))
		h.gain.Store(math.Float32bits(v.Gain))
	}
	for i, b := range b.Buses {
		c.m.buses[i].level.Store(math.Float32bits(b.Gain))
		c.m.buses[i].peak.Store(math.Float32bits(b.Peak))
		c.m.buses[i].rms.Store(math.Float32bits(b.RMS))
	}
	c.m.active.Store(int32(b.Active))
	c.pending.Add(-int32(delta))
	c.applied = b.Applied
	c.serial = b.Serial
	return nil
}

// RemoteMixer owns the actual mixer on the consumer thread. Mix and Skip use
// the same implementation as native playback; this type does not provide a
// browser worker, source transfer, or audio output by itself.
type RemoteMixer struct {
	m        *Mixer
	received uint64
	applied  uint64
	serial   uint64
	releases []VoiceID
	overflow bool
	broken   bool
}

func NewRemoteMixer(cfg Config) (*RemoteMixer, error) {
	m, err := New(cfg)
	if err != nil {
		return nil, err
	}
	// Each reserved handle can be released before the producer receives the
	// next batch, even when the configured voice count exceeds queueSize*2.
	r := &RemoteMixer{m: m, releases: make([]VoiceID, 0, max(queueSize*2, len(m.handles)))}
	m.onRelease = func(id VoiceID) {
		if len(r.releases) == cap(r.releases) {
			r.overflow = true
			return
		}
		r.releases = append(r.releases, id)
	}
	return r, nil
}

// Accept enqueues an ordered command. A play reserves the same handle on both
// sides; a mismatch is an error rather than silently addressing another voice.
func (r *RemoteMixer) Accept(p RemoteCommand) error {
	if r == nil || r.m == nil || r.overflow || r.broken || p.Sequence != r.received+1 || r.m.cmds.Len() == len(r.m.cmds.q.buf) {
		return errRemoteProtocol
	}
	if p.Command.Kind == CmdPlay {
		id := r.m.Reserve()
		if !id.Valid() || id != p.Command.Voice {
			r.broken = true
			return errRemoteProtocol
		}
	}
	if !r.m.cmds.Push(p.Command) {
		r.broken = true
		return errRemoteProtocol
	}
	r.received = p.Sequence
	return nil
}

func (r *RemoteMixer) Mix(out []float32) error {
	if r == nil || r.m == nil || r.overflow || r.broken {
		return errRemoteProtocol
	}
	r.m.Mix(out)
	r.applied = r.received
	if r.overflow {
		return errRemoteProtocol
	}
	return nil
}

func (r *RemoteMixer) Skip(frames int) error {
	if r == nil || r.m == nil || r.overflow || r.broken {
		return errRemoteProtocol
	}
	r.m.Skip(frames)
	r.applied = r.received
	if r.overflow {
		return errRemoteProtocol
	}
	return nil
}

// SetSampleRate is serialized with commands and rendering by the transport.
// It keeps existing voices and their source positions when an output device is
// reopened at a different rate; it does not itself reconnect that device.
func (r *RemoteMixer) SetSampleRate(rate int) error {
	if r == nil || r.m == nil || r.overflow || r.broken || rate <= 0 || int64(rate) > int64(^uint32(0)>>1) {
		return errRemoteProtocol
	}
	r.m.SetSampleRate(rate)
	return nil
}

// Drain is called after Mix or Skip, not from the real-time audio callback.
// It copies the bounded release ledger so it can be transferred asynchronously.
func (r *RemoteMixer) Drain() (RemoteBatch, error) {
	if r == nil || r.m == nil || r.overflow || r.broken || r.serial == ^uint64(0) {
		return RemoteBatch{}, errRemoteProtocol
	}
	r.serial++
	b := RemoteBatch{
		Serial: r.serial, Applied: r.applied, EventsDropped: r.m.events.Dropped(), Active: r.m.Active(),
		Released: append([]VoiceID(nil), r.releases...), Buses: make([]RemoteBus, len(r.m.buses)),
	}
	r.releases = r.releases[:0]
	for {
		e, ok := r.m.events.Pop()
		if !ok {
			break
		}
		b.Events = append(b.Events, e)
	}
	for i := range r.m.voices {
		v := &r.m.voices[i]
		if v.state != voiceFree {
			b.Voices = append(b.Voices, RemoteVoice{ID: v.id, Position: r.m.Position(v.id), Gain: r.m.VoiceGain(v.id)})
		}
	}
	for i := range b.Buses {
		b.Buses[i].Gain = r.m.BusGain(i)
		b.Buses[i].Peak, b.Buses[i].RMS = r.m.Meters(i)
	}
	return b, nil
}
