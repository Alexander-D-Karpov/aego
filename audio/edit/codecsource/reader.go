// Package codecsource adapts seekable decoders to bounded readers without opening files, verifying identities or saving edits.
package codecsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"aego/audio/codec"
	"aego/audio/edit"
)

// maxSamples bounds PCM requests to 8 MiB, excluding decoder buffers; the caller owns/closes the decoder and serializes access through this reader.
const maxSamples = 2 << 20

type Reader struct {
	mu         sync.Mutex
	decoder    codec.Decoder
	info       codec.Info
	next       int64
	positionOK bool
}

// New accepts finite seekable mono/stereo streams; the host guards encoded-source identity and digest.
func New(decoder codec.Decoder) (*Reader, error) {
	if decoder == nil {
		return nil, errors.New("audio edit decoder is missing")
	}
	info := decoder.Info()
	if info.SampleRate <= 0 || info.Channels < 1 || info.Channels > 2 || info.Frames <= 0 {
		return nil, errors.New("audio edit decoder has an unsupported layout or unknown length")
	}
	return &Reader{decoder: decoder, info: info}, nil
}

// ReadFrames serializes decode/seek without writing the source and returns exactly frames*channels or an error; cancellation cannot interrupt an active decoder call.
func (r *Reader) ReadFrames(ctx context.Context, start int64, frames int) ([]float32, error) {
	if r == nil || r.decoder == nil || r.info.Channels < 1 || r.info.Channels > 2 || ctx == nil {
		return nil, errors.New("audio edit read needs a reader and context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if start < 0 || start > r.info.Frames || frames < 0 || frames > maxSamples/r.info.Channels ||
		int64(frames) > r.info.Frames-start {
		return nil, errors.New("audio edit frame range exceeds source or memory budget")
	}
	if frames == 0 {
		return []float32{}, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !r.positionOK || r.next != start {
		r.positionOK = false
		if err := r.decoder.SeekFrame(start); err != nil {
			return nil, err
		}
	}
	// Any error or cancellation after this point invalidates the cached cursor.
	r.positionOK = false
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data := make([]float32, frames*r.info.Channels)
	for done := 0; done < frames; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		read, err := r.decoder.Read(data[done*r.info.Channels:])
		if read < 0 || read > frames-done {
			return nil, fmt.Errorf("audio edit decoder returned invalid frame count %d", read)
		}
		done += read
		if cancel := ctx.Err(); cancel != nil {
			return nil, cancel
		}
		if err != nil {
			if errors.Is(err, io.EOF) && done == frames {
				break
			}
			if errors.Is(err, io.EOF) {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		if read == 0 {
			return nil, io.ErrNoProgress
		}
	}
	r.next, r.positionOK = start+int64(frames), true
	return data, nil
}

var _ audioedit.Reader = (*Reader)(nil)
