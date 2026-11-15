package audio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"path"
	"strings"

	"aego/asset"
	"aego/asset/audiosidecar"
	"aego/asset/pipeline"
	"aego/audio/codec"
	"aego/audio/edit/codecsource"
)

// The current artifact API owns []byte. Limit private rendered candidates until
// the pipeline supports streaming artifact publication without whole-file copies.
const (
	maxEditedPreviewWAVBytes        int64 = 64 << 20
	maxEditedPreviewFrameOperations int64 = 64_000_000
)

func (i *Importer) GuardedMetadataCandidate(sourcePath string, metadata []byte) (bool, error) {
	switch strings.ToLower(path.Ext(sourcePath)) {
	case ".wav", ".ogg":
		return audiosidecar.NeedsGuardedPreview(metadata, sourcePath)
	default:
		return false, nil
	}
}

// PreviewGuardedMetadata renders a private edit candidate into the existing
// AEAU v1 format, or privately imports ordinary working settings unchanged.
// The normal memory and stream playback paths can consume either result.
// It neither changes the original source nor makes version-2 sidecars writable.
// The pipeline guards the source and metadata around this call; the caller must
// still guard asynchronous document/generation changes before using the result.
func (i *Importer) PreviewGuardedMetadata(request context.Context, ctx *pipeline.ImportContext, metadata []byte) (*pipeline.Result, error) {
	if request == nil || ctx == nil || ctx.ID.IsZero() {
		return nil, errors.New("edited preview needs an indexed audio source")
	}
	if err := request.Err(); err != nil {
		return nil, err
	}
	doc, settings, present, err := audiosidecar.DecodePreview(metadata, ctx.Path)
	if err != nil {
		return nil, fail(ctx, "invalid audio edit metadata", err)
	}
	if err := request.Err(); err != nil {
		return nil, err
	}
	if !present {
		meta, err := asset.ParseSourceMeta(metadata, ctx.Path+asset.MetaSuffix)
		if err != nil || meta.ID != ctx.ID || meta.Importer != i.ID() {
			return nil, fail(ctx, "audio preview metadata does not match the indexed asset", err)
		}
		for key, value := range i.Defaults() {
			if _, ok := meta.Settings[key]; !ok {
				meta.Settings[key] = value
			}
		}
		gain := float32(meta.Settings.Float("gain", 1))
		if math.IsNaN(float64(gain)) || math.IsInf(float64(gain), 0) || len(meta.Settings.String("bus", "sfx")) > 255 {
			return nil, fail(ctx, "invalid audio preview settings", nil)
		}
		private := *ctx
		private.Meta = meta
		result, err := i.Import(&private)
		if err != nil {
			return nil, err
		}
		if err := request.Err(); err != nil {
			return nil, err
		}
		return result, nil
	}
	if doc.Source.AssetID != ctx.ID.String() {
		return nil, fail(ctx, "audio edit does not match the indexed asset", nil)
	}
	if int64(len(ctx.Source)) > pipeline.MaxSourceFingerprintBytes ||
		doc.Trim.Length() > (maxEditedPreviewWAVBytes-44)/(int64(doc.Source.Channels)*4) ||
		doc.Trim.Length() > maxEditedPreviewFrameOperations/int64(max(len(doc.Operations), 1)) {
		return nil, fail(ctx, "audio edit exceeds the private preview budget", nil)
	}
	gain := float32(settings.Float("gain", 1))
	bus := settings.String("bus", "sfx")
	if math.IsNaN(float64(gain)) || math.IsInf(float64(gain), 0) || len(bus) > 255 {
		return nil, fail(ctx, "invalid audio preview settings", nil)
	}
	hash := sha256.New()
	for rest := ctx.Source; len(rest) > 0; {
		if err := request.Err(); err != nil {
			return nil, err
		}
		n := min(len(rest), 64<<10)
		_, _ = hash.Write(rest[:n])
		rest = rest[n:]
	}
	if hex.EncodeToString(hash.Sum(nil)) != doc.Source.SHA256 {
		return nil, fail(ctx, "audio source changed since the edit was prepared", nil)
	}
	if !editedSourceMatchesCodec(ctx.Source, doc.Source.Codec) {
		return nil, fail(ctx, "audio source format does not match the edit", nil)
	}
	decoder, err := codec.Open(bytes.NewReader(ctx.Source), ctx.Path)
	if err != nil {
		return nil, fail(ctx, "cannot decode the edited source", err)
	}
	// Preserve the existing importer precedence when no edit-specific loop is
	// specified, but refuse to silently crop a loop outside the edited range.
	var loopStart, loopEnd int64
	if loop, ok := decoder.(codec.Looper); ok {
		if start, end, ok := loop.Loop(); ok {
			loopStart, loopEnd = start, end
		}
	}
	if value := settings.Int("loopStart", -1); value >= 0 {
		loopStart = int64(value)
	}
	if value := settings.Int("loopEnd", -1); value >= 0 {
		loopEnd = int64(value)
	}
	if doc.Loop != nil {
		loopStart, loopEnd = doc.Loop.Start, doc.Loop.End
	}
	if loopStart != 0 || loopEnd != 0 {
		if loopStart < doc.Trim.Start || loopEnd > doc.Trim.End || loopStart >= loopEnd {
			decoder.Close()
			return nil, fail(ctx, "loop is outside the edited range", nil)
		}
		loopStart, loopEnd = loopStart-doc.Trim.Start, loopEnd-doc.Trim.Start
	}
	edited, err := codecsource.NewEditedDecoder(request, decoder, doc)
	if err != nil {
		decoder.Close()
		return nil, fail(ctx, "audio layout does not match the edit", err)
	}
	defer edited.Close()
	info := edited.Info()
	encoded, err := encodeEditedPreviewWAV(edited, info)
	if err != nil {
		return nil, fail(ctx, "cannot render the edited preview", err)
	}
	if err := request.Err(); err != nil {
		return nil, err
	}
	art := asset.EncodeAudio(&asset.AudioArtifact{
		Format: asset.AudioWAV, Rate: info.SampleRate, Channels: info.Channels, Frames: info.Frames,
		LoopStart: loopStart, LoopEnd: loopEnd, Gain: gain, Bus: bus,
		Stream: streamed(settings, len(encoded), info.Frames, info.SampleRate), Data: encoded,
	})
	return &pipeline.Result{Kind: asset.ArtAudio, Artifacts: []pipeline.Produced{{Kind: asset.ArtAudio, Data: art}}}, nil
}

func editedSourceMatchesCodec(source []byte, name string) bool {
	switch strings.ToLower(name) {
	case "wav":
		return len(source) >= 12 && string(source[:4]) == "RIFF" && string(source[8:12]) == "WAVE"
	case "ogg":
		return len(source) >= 4 && string(source[:4]) == "OggS"
	default:
		return false
	}
}

// encodeEditedPreviewWAV writes float PCM in bounded chunks directly to one
// pre-sized slice. AEAU encoding makes one additional whole-payload copy.
func encodeEditedPreviewWAV(decoder codec.Decoder, info codec.Info) ([]byte, error) {
	if decoder == nil || info.Channels < 1 || info.Channels > 2 || info.SampleRate <= 0 || info.Frames <= 0 {
		return nil, errors.New("invalid edited WAV layout")
	}
	bytesPerFrame := int64(info.Channels) * 4
	if int64(info.SampleRate) > math.MaxUint32/bytesPerFrame {
		return nil, errors.New("edited WAV sample rate exceeds its header budget")
	}
	if info.Frames > (maxEditedPreviewWAVBytes-44)/bytesPerFrame {
		return nil, errors.New("edited preview exceeds the 64 MiB WAV budget")
	}
	size := info.Frames * bytesPerFrame
	out := make([]byte, 44+int(size))
	le := binary.LittleEndian
	copy(out[:4], "RIFF")
	le.PutUint32(out[4:8], uint32(36+size))
	copy(out[8:16], "WAVEfmt ")
	le.PutUint32(out[16:20], 16)
	le.PutUint16(out[20:22], 3) // IEEE float32 PCM
	le.PutUint16(out[22:24], uint16(info.Channels))
	le.PutUint32(out[24:28], uint32(info.SampleRate))
	le.PutUint32(out[28:32], uint32(int64(info.SampleRate)*bytesPerFrame))
	le.PutUint16(out[32:34], uint16(bytesPerFrame))
	le.PutUint16(out[34:36], 32)
	copy(out[36:40], "data")
	le.PutUint32(out[40:44], uint32(size))
	buf := make([]float32, 4096*info.Channels)
	for frame := int64(0); frame < info.Frames; {
		want := int(min(int64(len(buf)/info.Channels), info.Frames-frame))
		n, err := decoder.Read(buf[:want*info.Channels])
		if n < 0 || n > want {
			return nil, errors.New("decoder returned an invalid edited frame count")
		}
		for sample, value := range buf[:n*info.Channels] {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return nil, errors.New("decoder returned non-finite edited audio")
			}
			offset := 44 + int(frame*bytesPerFrame) + sample*4
			le.PutUint32(out[offset:offset+4], math.Float32bits(value))
		}
		frame += int64(n)
		if err != nil && !(errors.Is(err, io.EOF) && frame == info.Frames) {
			return nil, err
		}
		if n == 0 && frame < info.Frames {
			return nil, io.ErrNoProgress
		}
	}
	return out, nil
}

var _ pipeline.GuardedMetadataPreviewer = (*Importer)(nil)
