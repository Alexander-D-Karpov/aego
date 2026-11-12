// Package codecsource adapts existing seekable decoders to audioedit's bounded
// frame reader. It does not open files, verify source identities, or save edits.
package codecsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"aego/audio/codec"
	"aego/editor/core/audioedit"
)

// maxSamples bounds the PCM allocated by one request to 8 MiB. Decoder-owned
// buffers are separate. The caller retains and closes the decoder, and must not
// read, seek, or close it concurrently except through this reader.
const maxSamples = 2 << 20

type Reader struct {
	mu         sync.Mutex
	decoder    codec.Decoder
	info       codec.Info
	next       int64
	positionOK bool
}

// New accepts only a finite, seekable mono or stereo stream. Identity and a
// digest guard for the encoded source remain the responsibility of the host.
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

// ReadFrames serializes seeking and decoding, returns exactly frames*channels
// samples or an error, and never changes or writes the encoded source. Context
// cancellation is cooperative; it cannot interrupt a decoder call in progress.
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
