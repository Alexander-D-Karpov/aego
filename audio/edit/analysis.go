package audioedit

import (
	"context"
	"math"
)

// ChannelAnalysis describes one channel of the effective signal before the
// final [-1,1] output clamp. ClippedFrames counts samples at or beyond full
// scale, like the waveform pyramid. Full scale may indicate clipping but does
// not by itself prove that a signal was clipped.
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

// Analyze measures the edited signal in bounded chunks without writing source,
// metadata, history or artifacts. Regions use the same source-frame coordinates
// as RenderWindow and must lie inside Trim. On cancellation or decoder failure
// it returns an error and no partial analysis.
//
// The caller owns the source reader and must guard its identity and lifetime;
// it must not mutate the supplied document concurrently with this call. The
// scan may be expensive for long ranges or many operations, so interactive
// callers must impose their own work budget and run it off the owner thread.
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
			// Scaling by the largest observed magnitude avoids overflowing the
			// sum of squares for valid finite, heavily amplified PCM.
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
