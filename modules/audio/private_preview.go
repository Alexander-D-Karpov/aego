package audio

import (
	"bytes"
	"context"
	"errors"
	"math"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/core/ids"
)

// MaxPrivatePreviewPayloadBytes bounds a private WAV or OGG payload before its
// AEAU header, without permitting an unbounded snapshot allocation.
const MaxPrivatePreviewPayloadBytes = 64 << 20

const maxPrivatePreviewBytes = MaxPrivatePreviewPayloadBytes + 512

// PreparedPreview owns one private decoded generation and its intended mixer
// rate. Its contents are not exposed through the shared asset cache.
type PreparedPreview struct {
	clip *Clip
	rate int
}

// PreviewArtifact plays an owned snapshot of a private WAV or OGG artifact without
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
	prepared, err := PreparePreviewArtifact(context.Background(), id, raw, m.mixer.SampleRate())
	if err != nil {
		return err
	}
	return m.PreviewPrepared(prepared, loop)
}

// PreparePreviewArtifact copies and decodes a bounded artifact without touching
// a Module, mixer or shared cache, so a worker may prepare it away from the UI
// thread. The caller must not mutate raw until this call returns. Cancellation
// is cooperative: a decoder, allocation or resample already in progress cannot
// be interrupted. A caller must check authoring and source generations before
// asking a Module to play the result.
func PreparePreviewArtifact(ctx context.Context, id ids.AssetID, raw []byte, rate int) (*PreparedPreview, error) {
	if ctx == nil {
		return nil, errors.New("audio: private preview needs a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id.IsZero() {
		return nil, errNoClip
	}
	if rate < 8000 || rate > 384000 {
		return nil, errors.New("audio: invalid preview output sample rate")
	}
	if len(raw) > maxPrivatePreviewBytes {
		return nil, errors.New("audio: private preview artifact exceeds its size budget")
	}
	owned := bytes.Clone(raw)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a, err := asset.DecodeAudio(owned)
	if err != nil {
		return nil, err
	}
	validFormat := a.Format == asset.AudioWAV && wav.Match(a.Data) ||
		a.Format == asset.AudioOgg && len(a.Data) >= 4 && string(a.Data[:4]) == "OggS"
	if !validFormat || a.Rate < 8000 || a.Rate > 384000 || a.Frames <= 0 ||
		math.IsNaN(float64(a.Gain)) || math.IsInf(float64(a.Gain), 0) ||
		(a.LoopStart != 0 || a.LoopEnd != 0) && (a.LoopStart < 0 || a.LoopEnd <= a.LoopStart || a.LoopEnd > a.Frames) {
		return nil, errors.New("audio: invalid private preview artifact")
	}
	d, err := codec.Open(bytes.NewReader(a.Data), "")
	if err != nil {
		return nil, err
	}
	info := d.Info()
	closeErr := d.Close()
	if closeErr != nil {
		return nil, closeErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if info.SampleRate != a.Rate || info.Channels != a.Channels || info.Frames != a.Frames {
		return nil, errors.New("audio: private preview layout does not match its payload")
	}
	if a.Stream && float64(a.Rate) > mix.MaxRateRatio*float64(rate) {
		return nil, errors.New("audio: private preview sample rate exceeds the mixer limit")
	}
	if !a.Stream {
		maxFrames := int64(codec.MaxMemory/4) / int64(a.Channels)
		if a.Frames > maxFrames || a.Frames > maxFrames*int64(a.Rate)/int64(rate) {
			return nil, errors.New("audio: resampled private preview exceeds the memory budget")
		}
	}
	c, err := decodeClip(id, owned, rate)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Mem != nil && c.Mem.Frames() != a.Frames*int64(rate)/int64(a.Rate) {
		return nil, errors.New("audio: private preview payload ended before its declared length")
	}
	return &PreparedPreview{clip: c, rate: rate}, nil
}

// PreviewPrepared queues an already prepared private clip on the module owner
// thread. A reopened output with a different sample rate rejects the old
// generation before stopping any existing voice.
func (m *Module) PreviewPrepared(prepared *PreparedPreview, loop bool) error {
	if m == nil || m.mixer == nil {
		return errNoOutput
	}
	if prepared == nil || prepared.clip == nil || prepared.rate != m.mixer.SampleRate() {
		return errors.New("audio: private preview is missing or uses a different output sample rate")
	}
	return m.previewClip(prepared.clip, loop)
}
