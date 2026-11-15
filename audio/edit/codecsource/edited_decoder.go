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

// editedChunkFrames bounds PCM and coordinate buffers per Read independently of source or destination size.
const editedChunkFrames = 4096

// EditedDecoder uses trim-relative cursor/loop and source-relative edits without inheriting source loops; the host verifies source identity/digest/lifetime and chooses loop precedence.
type EditedDecoder struct {
	mu       sync.Mutex
	ctx      context.Context
	decoder  codec.Decoder
	reader   *Reader
	doc      audioedit.Document
	info     codec.Info
	position int64
	closed   bool
	closeErr error
}

// NewEditedDecoder requires matching finite mono/stereo input; success owns and closes it once, errors leave ownership with the caller, and cancellation is cooperative.
func NewEditedDecoder(ctx context.Context, decoder codec.Decoder, doc audioedit.Document) (*EditedDecoder, error) {
	if ctx == nil || decoder == nil {
		return nil, errors.New("edited audio needs a context and source decoder")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	doc = doc.Clone()
	r, err := New(decoder)
	if err != nil {
		return nil, err
	}
	info := r.info
	if info.SampleRate != doc.Source.SampleRate || info.Channels != doc.Source.Channels || info.Frames != doc.Source.Frames {
		return nil, errors.New("edited audio source layout does not match the document")
	}
	info.Frames = doc.Trim.Length()
	return &EditedDecoder{ctx: ctx, decoder: decoder, reader: r, doc: doc, info: info}, nil
}

func (d *EditedDecoder) Info() codec.Info { return d.info }

func (d *EditedDecoder) Loop() (start, end int64, ok bool) {
	if d.doc.Loop == nil {
		return 0, 0, false
	}
	return d.doc.Loop.Start - d.doc.Trim.Start, d.doc.Loop.End - d.doc.Trim.Start, true
}

// Read returns bounded complete frames and leaves dst and cursor unchanged on render or cancellation error.
func (d *EditedDecoder) Read(dst []float32) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return 0, errors.New("edited audio decoder is closed")
	}
	if err := d.ctx.Err(); err != nil {
		return 0, err
	}
	if len(dst)%d.info.Channels != 0 {
		return 0, errors.New("edited audio destination must contain complete frames")
	}
	if len(dst) == 0 {
		return 0, nil
	}
	if d.position == d.info.Frames {
		return 0, io.EOF
	}
	frames := min(len(dst)/d.info.Channels, editedChunkFrames)
	frames = int(min(int64(frames), d.info.Frames-d.position))
	start := d.doc.Trim.Start + d.position
	pcm, err := audioedit.RenderWindow(d.ctx, d.reader, d.doc, audioedit.Range{Start: start, End: start + int64(frames)}, editedChunkFrames)
	if err != nil {
		return 0, err
	}
	if err := d.ctx.Err(); err != nil {
		return 0, err
	}
	copy(dst, pcm)
	d.position += int64(frames)
	return frames, nil
}

// SeekFrame moves the edited cursor without immediately seeking the source; invalid seeks leave it unchanged.
func (d *EditedDecoder) SeekFrame(frame int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("edited audio decoder is closed")
	}
	if err := d.ctx.Err(); err != nil {
		return err
	}
	if frame < 0 || frame > d.info.Frames {
		return fmt.Errorf("edited audio seek %d is outside [0,%d]", frame, d.info.Frames)
	}
	d.position = frame
	return nil
}

func (d *EditedDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		d.closeErr = d.decoder.Close()
	}
	return d.closeErr
}

var (
	_ codec.Decoder = (*EditedDecoder)(nil)
	_ codec.Looper  = (*EditedDecoder)(nil)
)
