package codec

import (
	"errors"
	"io"
	"math"
	"path"
	"strings"

	"aego/audio/dsp"
	"aego/audio/mix"
)

type Info struct {
	SampleRate int
	Channels   int
	Frames     int64
}

type Decoder interface {
	Info() Info
	Read(dst []float32) (frames int, err error)
	SeekFrame(frame int64) error
	Close() error
}

type Looper interface {
	Loop() (start, end int64, ok bool)
}

type Format struct {
	Name  string
	Match func(head []byte) bool
	Open  func(r io.ReadSeeker) (Decoder, error)
}

type StreamOptions struct {
	Loop      bool
	LoopStart int64
	LoopEnd   int64
	Chunk     int
	// StartFrame starts a fresh decoder at this source frame before publishing
	// PCM. The returned Stream completion reports validation or seek errors.
	// The stream/mixer position remains relative to this new start.
	StartFrame int64
}

const (
	MaxMemory  = 64 << 20
	readFrames = 4096
	headBytes  = 16
)

var (
	ErrUnknown  = errors.New("codec: unknown audio format")
	ErrMP3      = errors.New("codec: mp3 is not supported; convert with ffmpeg")
	ErrTooLarge = errors.New("codec: the sound is larger than 64 MB decoded; stream it instead")
	ErrLayout   = errors.New("codec: only mono and stereo sounds with a positive sample rate are supported")
	formats     []Format
)

func Register(f Format) {
	formats = append(formats, f)
}

func Formats() []string {
	out := make([]string, len(formats))
	for i, f := range formats {
		out[i] = f.Name
	}
	return out
}

func Open(r io.ReadSeeker, hint string) (Decoder, error) {
	var head [headBytes]byte
	n, err := io.ReadFull(r, head[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	h := head[:n]
	for _, f := range formats {
		if f.Match(h) {
			return f.Open(r)
		}
	}
	if mp3(h) || strings.EqualFold(path.Ext(hint), ".mp3") {
		return nil, ErrMP3
	}
	return nil, ErrUnknown
}

func mp3(h []byte) bool {
	return len(h) >= 3 && string(h[:3]) == "ID3" || len(h) >= 2 && h[0] == 0xFF && h[1]&0xE0 == 0xE0
}

func DecodeAll(d Decoder, rate int) (*mix.Memory, error) {
	info := d.Info()
	ch := info.Channels
	if ch < 1 || ch > 2 || info.SampleRate <= 0 {
		return nil, ErrLayout
	}
	limit := MaxMemory / 4
	if info.Frames > 0 && info.Frames*int64(ch) > int64(limit) {
		return nil, ErrTooLarge
	}
	var pcm []float32
	if info.Frames > 0 {
		pcm = make([]float32, 0, info.Frames*int64(ch))
	}
	buf := make([]float32, readFrames*ch)
	for {
		n, err := d.Read(buf)
		pcm = append(pcm, buf[:n*ch]...)
		if len(pcm) > limit {
			return nil, ErrTooLarge
		}
		if errors.Is(err, io.EOF) || (err == nil && n == 0) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	mem := &mix.Memory{PCM: pcm, Rate: info.SampleRate, Channels: ch}
	if l, ok := d.(Looper); ok {
		if s, e, ok := l.Loop(); ok {
			mem.LoopStart, mem.LoopEnd = s, e
		}
	}
	if rate > 0 && rate != info.SampleRate {
		mem.PCM = dsp.Resample(pcm, ch, info.SampleRate, rate)
		mem.LoopStart = mem.LoopStart * int64(rate) / int64(info.SampleRate)
		mem.LoopEnd = mem.LoopEnd * int64(rate) / int64(info.SampleRate)
		mem.Rate = rate
	}
	return mem, nil
}

func Stream(d Decoder, st *mix.Stream, o StreamOptions, cancel <-chan struct{}) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- feed(d, st, o, cancel)
	}()
	return done
}

// skipTo wraps a skipped position into the loop; it reports false past the end of a stream that does not loop.
func skipTo(d Decoder, o StreamOptions, to int64) (int64, bool) {
	end := d.Info().Frames
	if o.Loop && o.LoopEnd > 0 {
		end = o.LoopEnd
	}
	if end <= 0 || to < end {
		return to, true
	}
	if !o.Loop || end <= o.LoopStart {
		return 0, false
	}
	return o.LoopStart + (to-o.LoopStart)%(end-o.LoopStart), true
}

func feed(d Decoder, st *mix.Stream, o StreamOptions, cancel <-chan struct{}) error {
	defer st.SetEOF()
	ch := st.Channels
	if d.Info().Channels != ch {
		return errors.New("codec: the stream and the decoder have different channel counts")
	}
	if o.Loop && o.LoopEnd == 0 {
		if l, ok := d.(Looper); ok {
			if s, e, ok := l.Loop(); ok {
				o.LoopStart, o.LoopEnd = s, e
			}
		}
	}
	if o.StartFrame < 0 {
		return errors.New("codec: the starting frame cannot be negative")
	}
	if o.StartFrame > 0 {
		info := d.Info()
		if info.Frames > 0 && o.StartFrame >= info.Frames || o.Loop && o.LoopEnd > 0 && o.StartFrame >= o.LoopEnd {
			return errors.New("codec: the starting frame is outside the playable range")
		}
	}
	chunk := o.Chunk
	if chunk <= 0 {
		chunk = readFrames
	}
	buf := make([]float32, chunk*ch)
	var pending []float32
	pos, seekTo := o.StartFrame, o.StartFrame
	needsSeek := o.StartFrame > 0
	empty := 0
	for {
		select {
		case <-cancel:
			return nil
		default:
		}
		if sk := st.TakeSkip(); sk > 0 {
			k := min(sk, int64(len(pending)/ch))
			pending = pending[int(k)*ch:]
			if sk -= k; sk > 0 {
				// A held stream can advance repeatedly while its output is muted.
				// Keep its logical target current, but defer expensive decoder
				// seeks until the mixer actually releases the stream.
				from := pos
				if needsSeek {
					from = seekTo
				}
				if from > math.MaxInt64-sk {
					return errors.New("codec: the skipped frame position overflows")
				}
				to, ok := skipTo(d, o, from+sk)
				if !ok {
					return nil
				}
				seekTo, needsSeek = to, true
			}
		}
		if st.Held() {
			select {
			case <-cancel:
				return nil
			case <-st.Wake():
			}
			continue
		}
		if needsSeek {
			if err := d.SeekFrame(seekTo); err != nil {
				return err
			}
			pos, needsSeek = seekTo, false
			// A new skip can arrive while the decoder is seeking. Handle it
			// before reading or publishing samples from the old position.
			continue
		}
		if len(pending) == 0 {
			want := chunk
			if o.Loop && o.LoopEnd > 0 {
				want = int(min(int64(chunk), o.LoopEnd-pos))
			}
			n, err := 0, error(nil)
			if want > 0 {
				n, err = d.Read(buf[:want*ch])
			}
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			if n == 0 {
				if !o.Loop {
					return nil
				}
				empty++
				if empty > 2 {
					return nil
				}
				if err := d.SeekFrame(o.LoopStart); err != nil {
					return err
				}
				pos = o.LoopStart
				continue
			}
			empty = 0
			pos += int64(n)
			pending = buf[:n*ch]
			// Decoding may also overlap a new hold/skip. Let the next
			// iteration discard superseded samples before writing the ring.
			continue
		}
		w := st.Ring.Write(pending)
		pending = pending[w*ch:]
		if len(pending) > 0 && !st.WaitSpace(cancel) {
			return nil
		}
	}
}
