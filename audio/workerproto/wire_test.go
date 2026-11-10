package workerproto

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"aego/audio/mix"
)

func TestCommandWireRoundTripPreservesBitsAndWideIdentifiers(t *testing.T) {
	for _, kind := range []mix.CommandKind{mix.CmdPlay, mix.CmdStop, mix.CmdSetGain, mix.CmdSetPan,
		mix.CmdSetPitch, mix.CmdPause, mix.CmdResume, mix.CmdBusGain, mix.CmdBusMute, mix.CmdBusSolo,
		mix.CmdPauseAll, mix.CmdResumeAll, mix.CmdBusLayer, mix.CmdKill} {
		packet := mix.RemoteSourceCommand{Source: 1<<53 + 19, RemoteCommand: mix.RemoteCommand{
			Sequence: math.MaxUint64, Command: mix.Command{Kind: kind, Voice: mix.VoiceID{Index: math.MaxUint32, Gen: 9},
				Bus: -3, Layer: 2, Memory: &mix.Memory{}, Gain: math.Float32frombits(0x7fc01234),
				Pan: math.Float32frombits(0x80000000), Pitch: math.Float32frombits(0x7f800000),
				Fade: math.Float32frombits(0xff800000), Priority: -7, Flag: true}}}
		want := CommandFromMix(packet)
		raw, err := json.Marshal(Request{Command: want})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(raw, []byte(`"18446744073709551615"`)) || !bytes.Contains(raw, []byte(`"9007199254741011"`)) {
			t.Fatalf("wide integers lost their decimal string encoding: %s", raw)
		}
		got, err := DecodeRequest(raw)
		if err != nil || got.Command != want {
			t.Fatalf("kind %d: decoded %+v, want %+v, error %v", kind, got.Command, want, err)
		}
		decoded := got.Command.Mix()
		if decoded.Command.Memory != nil || decoded.Command.Stream != nil {
			t.Fatal("wire decoding must never invent Go source pointers")
		}
		if roundTrip := CommandFromMix(decoded); roundTrip != want {
			t.Fatalf("kind %d: round trip %+v, want %+v", kind, roundTrip, want)
		}
	}
}

func TestBatchWireRoundTripPreservesOrderAndBits(t *testing.T) {
	id1, id2 := mix.VoiceID{Index: 2, Gen: 3}, mix.VoiceID{Index: 1, Gen: 4}
	batch := mix.RemoteBatch{Serial: 1<<53 + 1, Applied: 1<<53 + 2, EventsDropped: 1<<53 + 3, Active: 1,
		Events:   []mix.Event{{Kind: mix.EvStarved, Voice: id1}, {Kind: mix.EvFinished, Voice: id2}},
		Released: []mix.VoiceID{id2},
		Voices:   []mix.RemoteVoice{{ID: id1, Position: math.Float32frombits(0x80000000), Gain: math.Float32frombits(0x7fc01234)}},
		Buses:    []mix.RemoteBus{{Gain: math.Float32frombits(0x7f800000), Peak: math.Float32frombits(0xff800000), RMS: .25}}}
	want := BatchFromMix(batch)
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeBatch(raw)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded %+v, want %+v, error %v", got, want, err)
	}
	if roundTrip := BatchFromMix(got.Mix()); !reflect.DeepEqual(roundTrip, want) {
		t.Fatalf("round trip %+v, want %+v", roundTrip, want)
	}
}

func TestDecodeRejectsInvalidOrImpreciseJSON(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(``), []byte(`null`), []byte(`[]`), []byte(`{} {}`), []byte(`{"source":7}`),
		[]byte(`{"command":{"sequence":9007199254740993}}`), []byte(`{"command":{"bad":1}}`),
		[]byte(`{"unknown":1}`), []byte(`{"source":"18446744073709551616"}`),
		bytes.Repeat([]byte{' '}, MaxJSON+1),
	} {
		if _, err := DecodeRequest(raw); err == nil {
			t.Fatalf("accepted invalid request %.100q", raw)
		}
	}
	for _, raw := range [][]byte{
		[]byte(`null`), []byte(`{} {}`), []byte(`{"serial":9007199254740993}`),
		[]byte(`{"released":[{"index":0,"gen":1,"unknown":0}]}`), []byte(`{"extra":1}`),
	} {
		if _, err := DecodeBatch(raw); err == nil {
			t.Fatalf("accepted invalid batch %q", raw)
		}
	}
}

func TestDecodeAllowsBoundaryWhitespaceButNotOversizedObject(t *testing.T) {
	if _, err := DecodeRequest([]byte(" \n {}\t")); err != nil {
		t.Fatal(err)
	}
	object := append([]byte(`{"x":"`), bytes.Repeat([]byte{'a'}, MaxJSON)...)
	object = append(object, []byte(`"}`)...)
	if _, err := DecodeRequest(object); err == nil {
		t.Fatal("accepted an oversized object")
	}
	object = append([]byte(`{}`), bytes.Repeat([]byte{' '}, MaxJSON)...)
	if _, err := DecodeRequest(object); err == nil {
		t.Fatal("accepted oversized whitespace around an object")
	}
}
