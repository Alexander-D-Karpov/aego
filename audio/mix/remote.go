package mix

import (
	"errors"
	"math"
	"sync/atomic"
)

var errRemoteProtocol = errors.New("mix: invalid remote mixer protocol")

// RemoteCommand is ordered; transport must copy or explicitly transfer Memory and Stream data instead of Go pointers.
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

// RemoteBatch acknowledges commands and records releases independently of the bounded, potentially lossy event queue.
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

// RemoteClient must be installed before render, skip or concurrent producers; it solely consumes commands while the remote mixer owns playback.
type RemoteClient struct {
	m       *Mixer
	pending atomic.Int32
	sent    uint64
	applied uint64
	serial  uint64
	dropped uint64
	live    map[VoiceID]uint64
	scratch []uint8
}

func NewRemoteClient(m *Mixer) (*RemoteClient, error) {
	if m == nil || m.remote || m.applied != 0 || m.clock != 0 {
		return nil, errRemoteProtocol
	}
	c := &RemoteClient{m: m, live: make(map[VoiceID]uint64), scratch: make([]uint8, len(m.handles))}
	c.pending.Store(int32(m.cmds.Len()))
	m.cmds.remote = &c.pending
	m.remote = true
	return c, nil
}

// CancelUnsent restores local ownership before any command/batch crosses, after stopping the worker and excluding concurrent access; queued commands and handles remain.
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

// Queued snapshots untransferred commands for the sole consumer; concurrent producers may append after the boundary.
func (c *RemoteClient) Queued() int {
	if c == nil || c.m == nil {
		return -1
	}
	return c.m.cmds.q.len()
}

// Next transfers one command, retaining its queue slot until actual application; deliver in order without fabricated acknowledgements.
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
	if cmd.Kind == CmdPlay || cmd.Kind == CmdReplaceStream {
		if !c.m.Live(cmd.Voice) {
			return RemoteCommand{}, false, errRemoteProtocol
		}
		if _, exists := c.live[cmd.Voice]; exists {
			return RemoteCommand{}, false, errRemoteProtocol
		}
		c.live[cmd.Voice] = c.sent + 1
	}
	c.sent++
	return RemoteCommand{Sequence: c.sent, Command: cmd}, true, nil
}

// Apply validates the whole batch before mutation; transport failures must be reported without guessing playback state.
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
		if sequence, ok := c.live[id]; !ok || sequence > b.Applied || !c.m.Live(id) || c.scratch[id.Index]&1 != 0 {
			return errRemoteProtocol
		}
		c.scratch[id.Index] |= 1
	}
	for _, e := range b.Events {
		if e.Kind < EvFinished || e.Kind > EvStarved {
			return errRemoteProtocol
		}
		if sequence, ok := c.live[e.Voice]; !ok || sequence > b.Applied {
			return errRemoteProtocol
		}
		if e.Kind != EvStarved && c.scratch[e.Voice.Index]&1 == 0 {
			return errRemoteProtocol
		}
	}
	for _, v := range b.Voices {
		if sequence, ok := c.live[v.ID]; !ok || sequence > b.Applied || !c.m.Live(v.ID) || c.scratch[v.ID.Index] != 0 {
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
		h.started.Store(true)
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

// RemoteMixer owns the consumer-thread mixer and uses native Mix/Skip; worker, source transfer and output are separate.
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
	// Reserve release capacity for every handle, even when voices exceed queueSize*2.
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

// Accept enqueues an ordered command and rejects mismatched play handles across the two mixers.
func (r *RemoteMixer) Accept(p RemoteCommand) error {
	if r == nil || r.m == nil || r.overflow || r.broken || p.Sequence != r.received+1 || r.m.cmds.Len() == len(r.m.cmds.q.buf) {
		return errRemoteProtocol
	}
	if p.Command.Kind == CmdPlay || p.Command.Kind == CmdReplaceStream {
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

// SetSampleRate is transport-serialized and preserves voice positions across rate changes without reconnecting the device.
func (r *RemoteMixer) SetSampleRate(rate int) error {
	if r == nil || r.m == nil || r.overflow || r.broken || rate <= 0 || int64(rate) > int64(^uint32(0)>>1) {
		return errRemoteProtocol
	}
	r.m.SetSampleRate(rate)
	return nil
}

// Drain copies the bounded release ledger after Mix/Skip, outside the real-time callback.
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
