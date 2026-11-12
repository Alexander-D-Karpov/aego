package audio

import (
	"bytes"
	"errors"
	"math"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/core/ids"
)

// maxPrivatePreviewBytes allows the bounded private 64 MiB WAV and its AEAU
// header, without permitting a caller to allocate an unbounded snapshot.
const maxPrivatePreviewBytes = 64<<20 + 512

// PreviewArtifact plays an owned snapshot of a private WAV artifact without
// inserting it into any shared asset or clip cache. The caller must not mutate
// raw until this call returns, and must separately check the authoring
// generation, disk inputs and cancellation before starting playback. This is
// not asset publication, persistence, or protection from later external edits.
// Like Preview, it is called on the audio module's owner thread.
func (m *Module) PreviewArtifact(id ids.AssetID, raw []byte, loop bool) error {
	if id.IsZero() {
		return errNoClip
	}
	if m == nil || m.mixer == nil {
		return errNoOutput
	}
	if len(raw) > maxPrivatePreviewBytes {
		return errors.New("audio: private preview artifact exceeds its size budget")
	}
	owned := bytes.Clone(raw)
	a, err := asset.DecodeAudio(owned)
	if err != nil {
		return err
	}
	if a.Format != asset.AudioWAV || !wav.Match(a.Data) || a.Rate < 8000 || a.Rate > 384000 || a.Frames <= 0 ||
		math.IsNaN(float64(a.Gain)) || math.IsInf(float64(a.Gain), 0) ||
		(a.LoopStart != 0 || a.LoopEnd != 0) && (a.LoopStart < 0 || a.LoopEnd <= a.LoopStart || a.LoopEnd > a.Frames) {
		return errors.New("audio: invalid private preview artifact")
	}
	d, err := codec.Open(bytes.NewReader(a.Data), "preview.wav")
	if err != nil {
		return err
	}
	info := d.Info()
	closeErr := d.Close()
	if closeErr != nil {
		return closeErr
	}
	if info.SampleRate != a.Rate || info.Channels != a.Channels || info.Frames != a.Frames {
		return errors.New("audio: private preview layout does not match its payload")
	}
	rate := m.mixer.SampleRate()
	if rate < 8000 || rate > 384000 {
		return errors.New("audio: invalid preview output sample rate")
	}
	if a.Stream && float64(a.Rate) > mix.MaxRateRatio*float64(rate) {
		return errors.New("audio: private preview sample rate exceeds the mixer limit")
	}
	if !a.Stream {
		maxFrames := int64(codec.MaxMemory/4) / int64(a.Channels)
		if a.Frames > maxFrames || a.Frames > maxFrames*int64(a.Rate)/int64(rate) {
			return errors.New("audio: resampled private preview exceeds the memory budget")
		}
	}
	c, err := decodeClip(id, owned, rate)
	if err != nil {
		return err
	}
	if c.Mem != nil && c.Mem.Frames() != a.Frames*int64(rate)/int64(a.Rate) {
		return errors.New("audio: private preview payload ended before its declared length")
	}
	return m.previewClip(c, loop)
}
