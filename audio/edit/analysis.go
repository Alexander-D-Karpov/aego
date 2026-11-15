package audioedit

import (
	"context"
	"math"
)

// ChannelAnalysis counts pre-clamp samples at or beyond full scale, which may indicate but do not prove clipping.
type ChannelAnalysis struct {
	Peak          float64
	RMS           float64
	ClippedFrames int64
}

// SignalAnalysis covers a source-frame range, with channels in source order.
type SignalAnalysis struct {
	Frames   int64
	Channels []ChannelAnalysis
}

type analysisAccumulator struct {
	peak, scaledSquares float64
	clipped             int64
}

// Analyze scans source-frame ranges within Trim without writes or partial error results; callers guard source/lifetime, avoid concurrent mutation and budget bounded work off-thread.
func Analyze(ctx context.Context, reader Reader, doc Document, region Range) (SignalAnalysis, error) {
	if err := validateAudioRange(ctx, reader, doc, region); err != nil {
		return SignalAnalysis{}, err
	}
	doc = doc.Clone()
	acc := make([]analysisAccumulator, doc.Source.Channels)
	const chunk = 8192
	for at := region.Start; at < region.End; {
		frames := int(min(int64(chunk), region.End-at))
		window := Range{Start: at, End: at + int64(frames)}
		if err := validateWindowBudget(window, doc.Source.Channels, chunk); err != nil {
			return SignalAnalysis{}, err
		}
		if err := visitWindow(ctx, reader, doc, window, func(_, channel int, sample float64) {
			a := &acc[channel]
			magnitude := math.Abs(sample)
			if magnitude >= 1 {
				a.clipped++
			}
			// Scale by peak magnitude to avoid sum-of-squares overflow.
			if magnitude > a.peak {
				ratio := a.peak / magnitude
				a.scaledSquares = a.scaledSquares*ratio*ratio + 1
				a.peak = magnitude
			} else if a.peak != 0 {
				ratio := magnitude / a.peak
				a.scaledSquares += ratio * ratio
			}
		}); err != nil {
			return SignalAnalysis{}, err
		}
		at += int64(frames)
	}
	if err := ctx.Err(); err != nil {
		return SignalAnalysis{}, err
	}
	out := SignalAnalysis{Frames: region.Length(), Channels: make([]ChannelAnalysis, len(acc))}
	for i, a := range acc {
		mean := min(1, a.scaledSquares/float64(out.Frames))
		out.Channels[i] = ChannelAnalysis{Peak: a.peak, RMS: a.peak * math.Sqrt(mean), ClippedFrames: a.clipped}
	}
	return out, nil
}
