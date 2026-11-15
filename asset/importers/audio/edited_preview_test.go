package audio

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"

	"aego/audio/codec"
	"aego/audio/codec/wav"
)

type previewTestDecoder struct {
	info       codec.Info
	pcm        []float32
	position   int
	maxRead    int
	err        error
	badCount   bool
	noProgress bool
	reads      int
}

func (d *previewTestDecoder) Info() codec.Info { return d.info }
func (d *previewTestDecoder) Close() error     { return nil }
func (d *previewTestDecoder) SeekFrame(int64) error {
	return errors.New("unexpected seek")
}
func (d *previewTestDecoder) Read(dst []float32) (int, error) {
	d.reads++
	if d.badCount {
		return len(dst)/d.info.Channels + 1, nil
	}
	if d.noProgress {
		return 0, nil
	}
	if d.err != nil {
		return 0, d.err
	}
	if d.position == len(d.pcm) {
		return 0, io.EOF
	}
	count := min(len(dst), len(d.pcm)-d.position)
	if d.maxRead > 0 {
		count = min(count, d.maxRead*d.info.Channels)
	}
	copy(dst, d.pcm[d.position:d.position+count])
	d.position += count
	return count / d.info.Channels, nil
}

func TestEditedPreviewWAVBoundsAndDecoderErrors(t *testing.T) {
	info := codec.Info{SampleRate: 48000, Channels: 2, Frames: 4}
	samples := []float32{.1, -.2, .3, -.4, .5, -.6, .7, -.8}
	d := &previewTestDecoder{info: info, pcm: samples, maxRead: 1}
	raw, err := encodeEditedPreviewWAV(d, info)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := wav.Open(bytes.NewReader(raw))
	if err != nil {
		t.Fatal("written WAV cannot be decoded", err)
	}
	defer decoded.Close()
	out := make([]float32, len(samples))
	if n, err := decoded.Read(out); n != 4 || err != nil {
		t.Fatalf("decoded WAV: frames=%d err=%v", n, err)
	}
	for i := range samples {
		if out[i] != samples[i] {
			t.Fatalf("float PCM lost precision at %d: %v != %v", i, out[i], samples[i])
		}
	}
	for _, reason := range []string{"oversize", "bad-rate", "invalid-count", "no-progress", "decoder-error", "short", "non-finite"} {
		t.Run(reason, func(t *testing.T) {
			candidate := &previewTestDecoder{info: info, pcm: append([]float32(nil), samples...)}
			layout := info
			switch reason {
			case "oversize":
				layout.Frames = (maxEditedPreviewWAVBytes-44)/8 + 1
			case "bad-rate":
				layout.SampleRate = math.MaxInt32
			case "invalid-count":
				candidate.badCount = true
			case "no-progress":
				candidate.noProgress = true
			case "decoder-error":
				candidate.err = io.ErrUnexpectedEOF
			case "short":
				candidate.pcm = candidate.pcm[:2]
			case "non-finite":
				candidate.pcm[1] = float32(math.NaN())
			}
			if output, err := encodeEditedPreviewWAV(candidate, layout); err == nil || output != nil {
				t.Fatalf("accepted %s: %d bytes, %v", reason, len(output), err)
			}
			if (reason == "oversize" || reason == "bad-rate") && candidate.reads != 0 {
				t.Fatal("decoder was read before rejecting the layout budget")
			}
		})
	}
}
