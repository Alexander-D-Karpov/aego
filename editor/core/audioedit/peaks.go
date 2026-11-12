package audioedit

import (
	"context"
	"errors"
	"fmt"
	"math"
)

// Reader provides decoded interleaved samples in bounded frame ranges.
// The implementation can use existing Aego streaming WAV/Vorbis decoders.
type Reader interface {
	ReadFrames(ctx context.Context, start int64, frames int) ([]float32, error)
}

type Peak struct {
	Min     float32 `json:"min"`
	Max     float32 `json:"max"`
	RMS     float32 `json:"rms"`
	Clipped uint32  `json:"clipped"`
	Frames  int64   `json:"frames"`
}

type Level struct {
	FramesPerBucket int64 `json:"frames_per_bucket"`
	// Samples are interleaved channel tiles: bucket*channels+channel.
	Samples []Peak `json:"samples"`
}
type Pyramid struct {
	Channels    int     `json:"channels"`
	TotalFrames int64   `json:"total_frames"`
	Levels      []Level `json:"levels"`
}

type accum struct {
	min, max float32
	sumSq    float64
	clipped  uint32
	n        int
}

func newAccum() accum { return accum{min: float32(math.Inf(1)), max: float32(math.Inf(-1))} }
func (a *accum) add(v float32) {
	if v < a.min {
		a.min = v
	}
	if v > a.max {
		a.max = v
	}
	a.sumSq += float64(v) * float64(v)
	if v <= -1 || v >= 1 {
		a.clipped++
	}
	a.n++
}
func (a accum) peak() Peak {
	if a.n == 0 {
		return Peak{}
	}
	return Peak{a.min, a.max, float32(math.Sqrt(a.sumSq / float64(a.n))), a.clipped, int64(a.n)}
}

const (
	maxPeakBuckets = 1_000_000
	// Includes every channel and every level: at most about 48 MiB of Peak
	// records on 64-bit platforms, plus bounded decoder and slice overhead.
	maxPeakSamples = 2_000_000
)

// BuildPyramid creates a multiresolution waveform without retaining the full
// decoded song. Its combined number of channel buckets across all levels is
// bounded before any allocation or decoder call.
func BuildPyramid(ctx context.Context, r Reader, frames int64, channels, baseBucket int) (Pyramid, error) {
	if ctx == nil || r == nil || frames <= 0 || channels < 1 || channels > 32 || baseBucket < 32 || baseBucket > 65536 {
		return Pyramid{}, errors.New("invalid peak source")
	}
	if err := ctx.Err(); err != nil {
		return Pyramid{}, err
	}
	// Subtract before dividing instead of adding baseBucket-1 to frames.
	buckets := (frames-1)/int64(baseBucket) + 1
	if buckets > maxPeakBuckets {
		return Pyramid{}, errors.New("too many waveform buckets")
	}
	var totalSamples int64
	for n := buckets; ; n = (n + 3) / 4 {
		if n > (maxPeakSamples-totalSamples)/int64(channels) {
			return Pyramid{}, errors.New("waveform pyramid exceeds memory budget")
		}
		totalSamples += n * int64(channels)
		if n == 1 {
			break
		}
	}
	out := Pyramid{Channels: channels, TotalFrames: frames}
	level := Level{FramesPerBucket: int64(baseBucket), Samples: make([]Peak, 0, int(buckets)*channels)}
	window := make([]accum, channels)
	for c := range window {
		window[c] = newAccum()
	}
	perBucket := 0
	const chunk = 8192
	for pos := int64(0); pos < frames; {
		if err := ctx.Err(); err != nil {
			return Pyramid{}, err
		}
		count := chunk
		if rem := frames - pos; rem < int64(count) {
			count = int(rem)
		}
		pcm, err := r.ReadFrames(ctx, pos, count)
		if err != nil {
			return Pyramid{}, err
		}
		if err := ctx.Err(); err != nil {
			return Pyramid{}, err
		}
		if len(pcm) != count*channels {
			return Pyramid{}, fmt.Errorf("decoder returned %d samples, want %d", len(pcm), count*channels)
		}
		for frame := 0; frame < count; frame++ {
			for c := 0; c < channels; c++ {
				v := pcm[frame*channels+c]
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					return Pyramid{}, errors.New("non-finite PCM sample")
				}
				window[c].add(v)
			}
			perBucket++
			if perBucket == baseBucket {
				for c := 0; c < channels; c++ {
					level.Samples = append(level.Samples, window[c].peak())
					window[c] = newAccum()
				}
				perBucket = 0
			}
		}
		pos += int64(count)
	}
	if perBucket > 0 {
		for c := 0; c < channels; c++ {
			level.Samples = append(level.Samples, window[c].peak())
		}
	}
	out.Levels = append(out.Levels, level)
	for len(level.Samples)/channels > 1 {
		n := (len(level.Samples)/channels + 3) / 4
		up := Level{FramesPerBucket: level.FramesPerBucket * 4, Samples: make([]Peak, 0, n*channels)}
		for bucket := 0; bucket < n; bucket++ {
			if bucket%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return Pyramid{}, err
				}
			}
			for c := 0; c < channels; c++ {
				a := newAccum()
				sum := 0.
				var total int64
				clips := uint64(0)
				for k := 0; k < 4; k++ {
					idx := (bucket*4+k)*channels + c
					if idx >= len(level.Samples) {
						break
					}
					p := level.Samples[idx]
					if p.Frames == 0 {
						continue
					}
					if p.Min < a.min {
						a.min = p.Min
					}
					if p.Max > a.max {
						a.max = p.Max
					}
					sum += float64(p.RMS) * float64(p.RMS) * float64(p.Frames)
					total += p.Frames
					clips += uint64(p.Clipped)
				}
				if total == 0 {
					up.Samples = append(up.Samples, Peak{})
					continue
				}
				up.Samples = append(up.Samples, Peak{Min: a.min, Max: a.max, RMS: float32(math.Sqrt(sum / float64(total))), Frames: total, Clipped: uint32(min(clips, math.MaxUint32))})
			}
		}
		out.Levels = append(out.Levels, up)
		level = up
	}
	return out, nil
}

// Visible returns the coarsest appropriate level for a requested pixel width.
// The caller can then draw just [first:last] buckets at O(view width) cost.
func (p Pyramid) Visible(start, end int64, pixels int) (Level, int, int, error) {
	if pixels < 1 || p.Channels < 1 || p.Channels > 32 || p.TotalFrames <= 0 || start < 0 || start >= end || end > p.TotalFrames || len(p.Levels) == 0 {
		return Level{}, 0, 0, errors.New("invalid waveform viewport")
	}
	choice := p.Levels[0]
	target := float64(end-start) / float64(pixels)
	var previous int64
	for _, lvl := range p.Levels {
		if lvl.FramesPerBucket <= previous || len(lvl.Samples)%p.Channels != 0 || int64(len(lvl.Samples)/p.Channels) != (p.TotalFrames-1)/lvl.FramesPerBucket+1 {
			return Level{}, 0, 0, errors.New("invalid waveform pyramid")
		}
		previous = lvl.FramesPerBucket
		if float64(lvl.FramesPerBucket) <= target*2 {
			choice = lvl
		}
	}
	first := start / choice.FramesPerBucket
	last := (end-1)/choice.FramesPerBucket + 1
	if first < 0 || last <= first || last > int64(len(choice.Samples)/p.Channels) {
		return Level{}, 0, 0, errors.New("invalid waveform bucket range")
	}
	return choice, int(first), int(last), nil
}
