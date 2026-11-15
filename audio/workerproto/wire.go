// Package workerproto defines ordered mixer/Worker messages; callers own transport, source lifetime, startup and recovery.
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

// Version changes on transport/reactor wire or ABI incompatibility and is checked before playback.
const Version = 4

const MaxJSON = 1 << 20

var ErrMessage = errors.New("audio worker: invalid message")

// Wide integers use decimal strings to avoid JS precision loss; raw float bits preserve NaNs, infinities and signed zero.
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
	Frame     int64           `json:"frame,string,omitempty"`
	PrevIndex uint32          `json:"prevIndex,omitempty"`
	PrevGen   uint32          `json:"prevGen,omitempty"`
	Paused    bool            `json:"paused,omitempty"`
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

// Decode rejects oversized, non-object, unknown-field or trailing JSON; it neither validates mixer state nor acknowledges messages.
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
		Priority: c.Priority, Flag: c.Flag, Frame: c.Frame,
		PrevIndex: c.Previous.Index, PrevGen: c.Previous.Gen, Paused: c.Paused}
}

func (c CommandMessage) Mix() mix.RemoteSourceCommand {
	return mix.RemoteSourceCommand{RemoteCommand: mix.RemoteCommand{Sequence: c.Sequence, Command: mix.Command{
		Kind: c.Kind, Voice: mix.VoiceID{Index: c.Index, Gen: c.Gen}, Bus: c.Bus, Layer: c.Layer,
		Gain: math.Float32frombits(c.GainBits), Pan: math.Float32frombits(c.PanBits),
		Pitch: math.Float32frombits(c.PitchBits), Fade: math.Float32frombits(c.FadeBits),
		Priority: c.Priority, Flag: c.Flag, Frame: c.Frame,
		Previous: mix.VoiceID{Index: c.PrevIndex, Gen: c.PrevGen}, Paused: c.Paused}}, Source: c.Source}
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

// Mix only decodes the batch; RemoteClient.Apply must validate sequencing and handle lifetimes.
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
