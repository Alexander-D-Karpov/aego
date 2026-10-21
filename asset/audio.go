package asset

import (
	"encoding/binary"
	"math"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
)

const (
	magicAudio      = "AEAU"
	audioVersion    = 1
	audioHeaderSize = 46
	audioStream     = 1
	audioLoop       = 2
	AudioWAV        = uint8(1)
	AudioOgg        = uint8(2)
)

type AudioSource interface {
	Audio(id ids.AssetID) ([]byte, bool)
}

type AudioArtifact struct {
	Format    uint8
	Rate      int
	Channels  int
	Frames    int64
	LoopStart int64
	LoopEnd   int64
	Gain      float32
	Bus       string
	Stream    bool
	Loop      bool
	Data      []byte
}

func EncodeAudio(a *AudioArtifact) []byte {
	bus := a.Bus
	if len(bus) > 255 {
		bus = bus[:255]
	}
	out := make([]byte, audioHeaderSize+len(bus)+len(a.Data))
	le := binary.LittleEndian
	copy(out[0:4], magicAudio)
	le.PutUint32(out[4:8], audioVersion)
	out[8] = a.Format
	var flags byte
	if a.Stream {
		flags |= audioStream
	}
	if a.Loop {
		flags |= audioLoop
	}
	out[9] = flags
	le.PutUint16(out[10:12], uint16(a.Channels))
	le.PutUint32(out[12:16], uint32(a.Rate))
	le.PutUint64(out[16:24], uint64(a.Frames))
	le.PutUint64(out[24:32], uint64(a.LoopStart))
	le.PutUint64(out[32:40], uint64(a.LoopEnd))
	le.PutUint32(out[40:44], math.Float32bits(a.Gain))
	le.PutUint16(out[44:46], uint16(len(bus)))
	copy(out[audioHeaderSize:], bus)
	copy(out[audioHeaderSize+len(bus):], a.Data)
	return out
}

func DecodeAudio(data []byte) (*AudioArtifact, error) {
	if len(data) < audioHeaderSize || string(data[0:4]) != magicAudio {
		return nil, errs.New(errs.AssetBadArtifact, "not an audio artifact")
	}
	le := binary.LittleEndian
	if v := le.Uint32(data[4:8]); v != audioVersion {
		return nil, errs.New(errs.AssetBadArtifact, "unsupported audio artifact version", log.F("version", v))
	}
	n := int(le.Uint16(data[44:46]))
	if len(data) < audioHeaderSize+n {
		return nil, errs.New(errs.AssetBadArtifact, "audio artifact is truncated")
	}
	a := &AudioArtifact{
		Format:    data[8],
		Stream:    data[9]&audioStream != 0,
		Loop:      data[9]&audioLoop != 0,
		Channels:  int(le.Uint16(data[10:12])),
		Rate:      int(le.Uint32(data[12:16])),
		Frames:    int64(le.Uint64(data[16:24])),
		LoopStart: int64(le.Uint64(data[24:32])),
		LoopEnd:   int64(le.Uint64(data[32:40])),
		Gain:      math.Float32frombits(le.Uint32(data[40:44])),
		Bus:       string(data[audioHeaderSize : audioHeaderSize+n]),
		Data:      data[audioHeaderSize+n:],
	}
	if a.Channels < 1 || a.Channels > 2 || a.Rate <= 0 {
		return nil, errs.New(errs.AssetBadArtifact, "audio artifact has an invalid layout", log.F("channels", a.Channels), log.F("rate", a.Rate))
	}
	return a, nil
}
