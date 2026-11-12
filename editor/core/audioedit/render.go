package audioedit

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// maxRenderBytes includes the coordinate and gain arrays, the returned PCM,
// and a decoder result of the requested size. A Reader must independently
// bound any memory it allocates internally.
const maxRenderBytes int64 = 32 << 20

func validateAudioRange(ctx context.Context, reader Reader, doc Document, region Range) error {
	if ctx == nil || reader == nil {
		return errors.New("audio render needs a context and reader")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := doc.Validate(); err != nil {
		return err
	}
	if !region.Valid(doc.Source.Frames) || region.Start < doc.Trim.Start || region.End > doc.Trim.End {
		return errors.New("invalid audio render range")
	}
	return nil
}

func validateWindowBudget(region Range, channels, maxWindow int) error {
	if maxWindow < 1 || region.Length() > int64(maxWindow) || region.Length() > maxRenderBytes/int64(16+8*channels) {
		return errors.New("audio render window exceeds memory budget")
	}
	return nil
}

// RenderWindow applies all operations to bounded PCM and clamps the final
// output to [-1,1]. Coordinates are mapped in reverse operation order.
func RenderWindow(ctx context.Context, reader Reader, doc Document, region Range, maxWindow int) ([]float32, error) {
	if err := validateAudioRange(ctx, reader, doc, region); err != nil {
		return nil, err
	}
	if err := validateWindowBudget(region, doc.Source.Channels, maxWindow); err != nil {
		return nil, err
	}
	result := make([]float32, int(region.Length())*doc.Source.Channels)
	err := visitWindow(ctx, reader, doc, region, func(frame, channel int, sample float64) {
		result[frame*doc.Source.Channels+channel] = float32(math.Max(-1, math.Min(1, sample)))
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// visitWindow emits the effective signal before final output clipping. Callers
// validate the range and budget first; the document and Reader are not mutated.
func visitWindow(ctx context.Context, reader Reader, doc Document, region Range, visit func(int, int, float64)) error {
	n := int(region.Length())
	channels := doc.Source.Channels
	coordinates := make([]int64, n)
	gains := make([]float64, n)
	for i := range coordinates {
		if err := ctx.Err(); err != nil {
			return err
		}
		x, gain := region.Start+int64(i), 1.0
		for j := len(doc.Operations) - 1; j >= 0; j-- {
			op := doc.Operations[j]
			if x < op.Region.Start || x >= op.Region.End {
				continue
			}
			switch op.Kind {
			case Reverse:
				x = op.Region.Start + op.Region.End - 1 - x
			case Silence:
				gain = 0
			case Gain, Normalize:
				gain *= op.Value
			case FadeIn:
				if op.Region.Length() <= 1 {
					gain = 0
				} else {
					gain *= float64(x-op.Region.Start) / float64(op.Region.Length()-1)
				}
			case FadeOut:
				if op.Region.Length() <= 1 {
					gain = 0
				} else {
					gain *= float64(op.Region.End-1-x) / float64(op.Region.Length()-1)
				}
			}
			if math.IsNaN(gain) || math.IsInf(gain, 0) {
				return errors.New("audio operation gain overflow")
			}
		}
		coordinates[i], gains[i] = x, gain
	}
	for i := 0; i < n; {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Batch adjacent ascending or descending source positions into a
		// single bounded ReadFrames call; avoid a decoder call per sample.
		step := int64(1)
		if i+1 < n && coordinates[i+1] == coordinates[i]-1 {
			step = -1
		}
		end := i + 1
		for end < n && coordinates[end] == coordinates[end-1]+step {
			end++
		}
		lo := coordinates[i]
		if step < 0 {
			lo = coordinates[end-1]
		}
		count := end - i
		data, err := reader.ReadFrames(ctx, lo, count)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(data) != count*channels {
			return fmt.Errorf("decoder returned %d samples, expected %d", len(data), count*channels)
		}
		for k := i; k < end; k++ {
			if (k-i)%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			srcFrame := k - i
			if step < 0 {
				srcFrame = end - k - 1
			}
			for c := 0; c < channels; c++ {
				sample := float64(data[srcFrame*channels+c]) * gains[k]
				if math.IsInf(sample, 0) || math.IsNaN(sample) {
					return errors.New("non-finite edited PCM")
				}
				visit(k, c, sample)
			}
		}
		i = end
	}
	return nil
}

// MeasurePeak measures the effective, previously edited signal before final
// clipping. This lets a later Normalize operation attenuate over-range signals.
// It scans bounded chunks and never rewrites the encoded source.
func MeasurePeak(ctx context.Context, reader Reader, doc Document, region Range) (float64, error) {
	if err := validateAudioRange(ctx, reader, doc, region); err != nil {
		return 0, err
	}
	peak := 0.0
	for at := region.Start; at < region.End; {
		count := 8192
		if int64(count) > region.End-at {
			count = int(region.End - at)
		}
		window := Range{at, at + int64(count)}
		if err := validateWindowBudget(window, doc.Source.Channels, count); err != nil {
			return 0, err
		}
		if err := visitWindow(ctx, reader, doc, window, func(_, _ int, sample float64) {
			a := math.Abs(sample)
			if a > peak {
				peak = a
			}
		}); err != nil {
			return 0, err
		}
		at += int64(count)
	}
	return peak, nil
}

func NormalizeMultiplier(peak, target float64) (float64, error) {
	if math.IsNaN(peak) || math.IsInf(peak, 0) || peak < 0 || math.IsNaN(target) || math.IsInf(target, 0) || target <= 0 || target > 1 {
		return 0, errors.New("invalid normalization peaks")
	}
	if peak == 0 {
		return 1, nil
	}
	multiplier := target / peak
	if multiplier > 100 {
		multiplier = 100
	}
	return multiplier, nil
}

// NearestZeroCrossing returns the nearest sign change or minimum-amplitude
// frame in the first channel, limited to a small decoded neighborhood.
func NearestZeroCrossing(ctx context.Context, reader Reader, source Source, frame int64, radius int) (int64, error) {
	if ctx == nil || reader == nil {
		return 0, errors.New("snap needs a context and reader")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := source.Validate(); err != nil {
		return 0, err
	}
	if frame < 0 || frame >= source.Frames || radius < 0 || radius > 4096 {
		return 0, errors.New("invalid snap request")
	}
	start := frame - int64(radius)
	if start < 0 {
		start = 0
	}
	end := frame + int64(radius) + 1
	if end > source.Frames {
		end = source.Frames
	}
	pcm, err := reader.ReadFrames(ctx, start, int(end-start))
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(pcm) != int(end-start)*source.Channels {
		return 0, errors.New("invalid PCM from decoder")
	}
	for _, sample := range pcm {
		if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
			return 0, errors.New("invalid PCM sample")
		}
	}
	best := frame
	bestScore := math.MaxFloat64
	for i := 0; i < int(end-start); i++ {
		amp := math.Abs(float64(pcm[i*source.Channels]))
		crosses := i > 0 && ((pcm[(i-1)*source.Channels] < 0) != (pcm[i*source.Channels] < 0))
		score := amp + float64(abs64(start+int64(i)-frame))*0.0001
		if crosses {
			score -= 2
		}
		if score < bestScore {
			bestScore = score
			best = start + int64(i)
		}
	}
	return best, nil
}
func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// ExportWAV16 writes quantized 16-bit PCM in chunks to the supplied writer. It
// never opens Source.Path. Once writing begins, a decoder, cancellation or I/O
// error can leave a partial stream; the caller owns staging and publication.
func ExportWAV16(ctx context.Context, w io.Writer, reader Reader, doc Document, region Range) error {
	if w == nil {
		return errors.New("audio export needs a writer")
	}
	if err := validateAudioRange(ctx, reader, doc, region); err != nil {
		return err
	}
	bytesPerFrame := int64(doc.Source.Channels) * 2
	if region.Length() > (math.MaxUint32-36)/bytesPerFrame {
		return errors.New("audio too large for RIFF/WAV; split export")
	}
	size := region.Length() * bytesPerFrame
	var header [44]byte
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+size))
	copy(header[8:12], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(doc.Source.Channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(doc.Source.SampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(doc.Source.SampleRate)*uint32(bytesPerFrame))
	binary.LittleEndian.PutUint16(header[32:34], uint16(bytesPerFrame))
	binary.LittleEndian.PutUint16(header[34:36], 16)
	header[36], header[37], header[38], header[39] = 'd', 'a', 't', 'a'
	binary.LittleEndian.PutUint32(header[40:44], uint32(size))
	if err := writeAudioBytes(w, header[:]); err != nil {
		return err
	}
	const chunk = 4096
	for at := region.Start; at < region.End; {
		frames := chunk
		if region.End-at < int64(frames) {
			frames = int(region.End - at)
		}
		pcm, err := RenderWindow(ctx, reader, doc, Range{at, at + int64(frames)}, chunk)
		if err != nil {
			return err
		}
		encoded := make([]byte, len(pcm)*2)
		for i, v := range pcm {
			sample := int32(math.Round(float64(v) * 32767))
			if v <= -1 {
				sample = -32768
			}
			if sample > 32767 {
				sample = 32767
			}
			binary.LittleEndian.PutUint16(encoded[i*2:], uint16(int16(sample)))
		}
		if err := writeAudioBytes(w, encoded); err != nil {
			return err
		}
		at += int64(frames)
	}
	return nil
}

func writeAudioBytes(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}
