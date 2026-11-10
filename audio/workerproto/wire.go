// Package workerproto defines the ordered messages between the Go mixer and
// the WASI audio worker. It is transport-independent: delivery, source
// lifetime, startup and recovery remain the responsibility of the caller.
package workerproto

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"

	"aego/audio/mix"
)

const (
	Init int32 = iota + 1
	BeginMemory
	CommitMemory
	Stream
	StreamWrite
	StreamEnd
	StreamStatus
	Command
	Drop
	Drain
	Skip
	StreamControlAck
	SetSampleRate
	TryStreamEnd
)

// Version changes whenever the JS transport and reactor cease to be wire/ABI
// compatible. It is checked before a worker can be selected for playback.
const Version = 2

const MaxJSON = 1 << 20

var ErrMessage = errors.New("audio worker: invalid message")

// Wide integers are decimal strings on the wire: passing them as JavaScript
// numbers would lose precision. Floating point values use their raw bits so
// NaNs, infinities and signed zero also survive a round trip.
type CommandMessage struct {
	Sequence  uint64          `json:"sequence,string,omitempty"`
	Kind      mix.CommandKind `json:"kind,omitempty"`
	Index     uint32          `json:"index,omitempty"`
	Gen       uint32          `json:"gen,omitempty"`
	Bus       int32           `json:"bus,omitempty"`
	Layer     uint8           `json:"layer,omitempty"`
	Source    uint64          `json:"source,string,omitempty"`
	GainBits  uint32          `json:"gainBits,omitempty"`
	PanBits   uint32          `json:"panBits,omitempty"`
	PitchBits uint32          `json:"pitchBits,omitempty"`
	FadeBits  uint32          `json:"fadeBits,omitempty"`
	Priority  int32           `json:"priority,omitempty"`
	Flag      bool            `json:"flag,omitempty"`
}

type Request struct {
	Source    uint64         `json:"source,string,omitempty"`
	Rate      int            `json:"rate,omitempty"`
	Channels  int            `json:"channels,omitempty"`
	Frames    int            `json:"frames,omitempty"`
	Voices    int            `json:"voices,omitempty"`
	Samples   int            `json:"samples,omitempty"`
	Budget    int64          `json:"budget,omitempty"`
	LoopStart int64          `json:"loopStart,string,omitempty"`
	LoopEnd   int64          `json:"loopEnd,string,omitempty"`
	Loop      bool           `json:"loop,omitempty"`
	Control   uint32         `json:"control,omitempty"`
	Buses     []mix.BusDesc  `json:"buses,omitempty"`
	Command   CommandMessage `json:"command,omitempty"`
}

type VoiceID struct {
	Index uint32 `json:"index"`
	Gen   uint32 `json:"gen"`
}

type Event struct {
	Kind  mix.EventKind `json:"kind"`
	Voice VoiceID       `json:"voice"`
}

type Voice struct {
	ID           VoiceID `json:"id"`
	PositionBits uint32  `json:"positionBits"`
	GainBits     uint32  `json:"gainBits"`
}

type Bus struct {
	GainBits uint32 `json:"gainBits"`
	PeakBits uint32 `json:"peakBits"`
	RMSBits  uint32 `json:"rmsBits"`
}

type Batch struct {
	Serial        uint64    `json:"serial,string"`
	Applied       uint64    `json:"applied,string"`
	Events        []Event   `json:"events"`
	EventsDropped uint64    `json:"eventsDropped,string"`
	Released      []VoiceID `json:"released"`
	Active        int       `json:"active"`
	Voices        []Voice   `json:"voices"`
	Buses         []Bus     `json:"buses"`
}

// Decode rejects oversized messages, unknown fields, non-object JSON and
// trailing data. It does not validate mixer state or acknowledge the message.
func decode(data []byte, out any) error {
	if len(data) > MaxJSON {
		return ErrMessage
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return ErrMessage
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return ErrMessage
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return ErrMessage
	}
	return nil
}

func DecodeRequest(data []byte) (Request, error) {
	var r Request
	err := decode(data, &r)
	return r, err
}

func DecodeBatch(data []byte) (Batch, error) {
	var b Batch
	err := decode(data, &b)
	return b, err
}

func CommandFromMix(p mix.RemoteSourceCommand) CommandMessage {
	c := p.Command
	return CommandMessage{Sequence: p.Sequence, Kind: c.Kind, Index: c.Voice.Index, Gen: c.Voice.Gen,
		Bus: c.Bus, Layer: c.Layer, Source: p.Source, GainBits: math.Float32bits(c.Gain),
		PanBits: math.Float32bits(c.Pan), PitchBits: math.Float32bits(c.Pitch), FadeBits: math.Float32bits(c.Fade),
		Priority: c.Priority, Flag: c.Flag}
}

func (c CommandMessage) Mix() mix.RemoteSourceCommand {
	return mix.RemoteSourceCommand{RemoteCommand: mix.RemoteCommand{Sequence: c.Sequence, Command: mix.Command{
		Kind: c.Kind, Voice: mix.VoiceID{Index: c.Index, Gen: c.Gen}, Bus: c.Bus, Layer: c.Layer,
		Gain: math.Float32frombits(c.GainBits), Pan: math.Float32frombits(c.PanBits),
		Pitch: math.Float32frombits(c.PitchBits), Fade: math.Float32frombits(c.FadeBits),
		Priority: c.Priority, Flag: c.Flag}}, Source: c.Source}
}

func BatchFromMix(b mix.RemoteBatch) Batch {
	out := Batch{Serial: b.Serial, Applied: b.Applied, EventsDropped: b.EventsDropped, Active: b.Active}
	for _, e := range b.Events {
		out.Events = append(out.Events, Event{Kind: e.Kind, Voice: VoiceID(e.Voice)})
	}
	for _, id := range b.Released {
		out.Released = append(out.Released, VoiceID(id))
	}
	for _, v := range b.Voices {
		out.Voices = append(out.Voices, Voice{ID: VoiceID(v.ID),
			PositionBits: math.Float32bits(v.Position), GainBits: math.Float32bits(v.Gain)})
	}
	for _, bus := range b.Buses {
		out.Buses = append(out.Buses, Bus{GainBits: math.Float32bits(bus.Gain),
			PeakBits: math.Float32bits(bus.Peak), RMSBits: math.Float32bits(bus.RMS)})
	}
	return out
}

// Mix only decodes the batch; callers must still pass it through
// RemoteClient.Apply, which validates sequencing and handle lifetimes.
func (b Batch) Mix() mix.RemoteBatch {
	out := mix.RemoteBatch{Serial: b.Serial, Applied: b.Applied, EventsDropped: b.EventsDropped, Active: b.Active}
	for _, e := range b.Events {
		out.Events = append(out.Events, mix.Event{Kind: e.Kind, Voice: mix.VoiceID(e.Voice)})
	}
	for _, id := range b.Released {
		out.Released = append(out.Released, mix.VoiceID(id))
	}
	for _, v := range b.Voices {
		out.Voices = append(out.Voices, mix.RemoteVoice{ID: mix.VoiceID(v.ID),
			Position: math.Float32frombits(v.PositionBits), Gain: math.Float32frombits(v.GainBits)})
	}
	for _, bus := range b.Buses {
		out.Buses = append(out.Buses, mix.RemoteBus{Gain: math.Float32frombits(bus.GainBits),
			Peak: math.Float32frombits(bus.PeakBits), RMS: math.Float32frombits(bus.RMSBits)})
	}
	return out
}
