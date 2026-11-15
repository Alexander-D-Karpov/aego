package audio

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/mix"
)

// preparedStreamSeek owns a fresh feeder until it is either cancelled or
// transferred to the preview owner. It never mutates the currently playing
// stream. Closing the cancellation channel is nonblocking; a decoder already
// inside SeekFrame or Read cannot necessarily be interrupted.
type preparedStreamSeek struct {
	stream             *mix.Stream
	stop               chan struct{}
	done               chan struct{}
	once               sync.Once
	err                error // read only after done closes
	origin, frame      int64
	loopStart, loopEnd int64
	loop               bool
	initial            int64
}

func (p *preparedStreamSeek) cancel() {
	if p != nil {
		p.once.Do(func() { close(p.stop) })
	}
}

// prepareStreamSeek runs away from the module owner thread. The artifact and
// payload must be immutable and owned by the caller until preparation returns;
// the resulting feeder continues to read the payload until it stops. The
// caller must check its preview generation, voice and output again before
// queueing a replacement, and must cancel any result it does not adopt.
func prepareStreamSeek(ctx context.Context, a *asset.AudioArtifact, frame int64, loop bool, outputRate int) (*preparedStreamSeek, error) {
	if ctx == nil {
		return nil, errors.New("audio: stream seek needs a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || !a.Stream || len(a.Data) > MaxPrivatePreviewPayloadBytes ||
		(a.Format != asset.AudioWAV || !wav.Match(a.Data)) &&
			(a.Format != asset.AudioOgg || len(a.Data) < 4 || string(a.Data[:4]) != "OggS") {
		return nil, errors.New("audio: stream seek needs a bounded WAV or OGG artifact")
	}
	d, err := codec.Open(bytes.NewReader(a.Data), "")
	if err != nil {
		return nil, err
	}
	return prepareStreamSeekDecoder(ctx, a, frame, loop, outputRate, d)
}

// prepareStreamSeekDecoder takes ownership of d, including on failure.
func prepareStreamSeekDecoder(ctx context.Context, a *asset.AudioArtifact, frame int64, loop bool, outputRate int, d codec.Decoder) (*preparedStreamSeek, error) {
	if d == nil {
		return nil, errors.New("audio: stream seek needs a decoder and context")
	}
	owned := true
	defer func() {
		if owned {
			_ = d.Close()
		}
	}()
	if ctx == nil || a == nil {
		return nil, errors.New("audio: stream seek needs a decoder and context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info := d.Info()
	if a.Rate < 8000 || a.Rate > 384000 || outputRate < 8000 || outputRate > 384000 ||
		float64(a.Rate) > mix.MaxRateRatio*float64(outputRate) || a.Frames <= 0 || a.Frames > 1<<53 ||
		(a.Channels != 1 && a.Channels != 2) || info.SampleRate != a.Rate || info.Channels != a.Channels || info.Frames != a.Frames ||
		frame < 0 || frame >= a.Frames {
		return nil, errors.New("audio: stream seek frame or decoder layout is invalid")
	}
	start, end := int64(0), a.Frames
	validLoop := func(s, e int64) bool { return s >= 0 && e > s && e <= a.Frames }
	if loop {
		if validLoop(a.LoopStart, a.LoopEnd) {
			start, end = a.LoopStart, a.LoopEnd
		} else if embedded, ok := d.(codec.Looper); ok {
			if s, e, valid := embedded.Loop(); valid && validLoop(s, e) {
				start, end = s, e
			}
		}
		if frame >= end {
			return nil, errors.New("audio: stream seek is beyond the loop end")
		}
	}
	origin, initial := frame, int64(0)
	if frame > 0 {
		// Hermite interpolation needs the real preceding sample when source and
		// output rates differ. The mixer starts at frame 1 of this new stream.
		origin, initial = frame-1, 1
	}
	p := &preparedStreamSeek{stream: mix.NewStream(a.Rate, a.Channels, a.Rate), stop: make(chan struct{}),
		done: make(chan struct{}), origin: origin, frame: frame, initial: initial,
		loop: loop, loopStart: start, loopEnd: end}
	opts := codec.StreamOptions{Loop: loop, StartFrame: origin}
	if loop {
		opts.LoopStart, opts.LoopEnd = start, end
	}
	feeder := codec.Stream(d, p.stream, opts, p.stop)
	owned = false
	go func() {
		p.err = errors.Join(<-feeder, d.Close())
		close(p.done)
	}()
	// At least the target and both following interpolation samples must be
	// ready, except at a genuine EOF where the mixer can render the tail.
	need := int(initial) + 3
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := ctx.Err(); err != nil {
			p.cancel()
			// Keep this worker occupied until the decoder really stops. Otherwise
			// repeated cancelled seeks could accumulate blocked decoder goroutines.
			<-p.done
			return nil, err
		}
		select {
		case <-p.done:
			if p.err != nil {
				p.cancel()
				return nil, p.err
			}
			if p.stream.Ring.Len() < int(initial)+1 {
				p.cancel()
				return nil, errors.New("audio: stream ended before the requested frame")
			}
			return p, nil
		default:
		}
		if p.stream.Ring.Len() >= need {
			return p, nil
		}
		select {
		case <-ctx.Done():
			p.cancel()
			<-p.done
			return nil, ctx.Err()
		case <-p.done:
			// Check the result and remaining buffered frames above.
		case <-tick.C:
		}
	}
}
