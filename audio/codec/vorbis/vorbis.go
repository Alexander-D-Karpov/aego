package vorbis

import (
	"errors"
	"fmt"
	"io"

	"aego/audio/codec"
	"aego/audio/codec/vorbis/internal/oggvorbis"
)

func init() {
	codec.Register(codec.Format{Name: "vorbis", Match: Match, Open: Open})
}

func Match(h []byte) bool {
	return len(h) >= 4 && string(h[:4]) == "OggS"
}

type Decoder struct {
	r    *oggvorbis.Reader
	src  io.ReadSeeker
	info codec.Info
}

func catch(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("vorbis: broken stream: %v", r)
	}
}

func Open(rs io.ReadSeeker) (dec codec.Decoder, err error) {
	defer catch(&err)
	r, err := oggvorbis.NewReader(rs)
	if err != nil {
		return nil, fmt.Errorf("vorbis: %w", err)
	}
	ch := r.Channels()
	if ch < 1 || ch > 2 || r.SampleRate() <= 0 {
		return nil, errors.New("vorbis: only mono and stereo streams with a positive sample rate are supported")
	}
	frames := r.Length()
	if frames <= 0 {
		frames = -1
	}
	return &Decoder{r: r, src: rs, info: codec.Info{SampleRate: r.SampleRate(), Channels: ch, Frames: frames}}, nil
}

func (d *Decoder) Info() codec.Info {
	return d.info
}

func (d *Decoder) Read(dst []float32) (n int, err error) {
	defer catch(&err)
	ch := d.info.Channels
	want := len(dst) / ch * ch
	if want == 0 {
		return 0, nil
	}
	got, err := d.r.Read(dst[:want])
	return got / ch, err
}

func (d *Decoder) SeekFrame(frame int64) (err error) {
	defer catch(&err)
	return d.r.SetPosition(max(frame, 0))
}

func (d *Decoder) Close() error {
	if c, ok := d.src.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
