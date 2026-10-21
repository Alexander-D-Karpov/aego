package codec_test

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"testing"

	"aego/audio/codec"
	_ "aego/audio/codec/vorbis"
	"aego/audio/codec/wav"
)

func ramp(frames, ch int) []float32 {
	pcm := make([]float32, frames*ch)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i)*0.013)) * 0.9
	}
	return pcm
}

func open(t testing.TB, data []byte, hint string) codec.Decoder {
	t.Helper()
	d, err := codec.Open(bytes.NewReader(data), hint)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestWAVFormats(t *testing.T) {
	type format struct {
		bits  int
		float bool
		tol   float64
	}
	formats := []format{{8, false, 1.0 / 64}, {16, false, 1e-4}, {24, false, 1e-6}, {32, false, 1e-6}, {32, true, 0}}
	for _, f := range formats {
		for _, ch := range []int{1, 2} {
			pcm := ramp(1000, ch)
			data := wav.Encode(pcm, 44100, ch, wav.EncodeOptions{Bits: f.bits, Float: f.float})
			d := open(t, data, "x.wav")
			info := d.Info()
			if info.SampleRate != 44100 || info.Channels != ch || info.Frames != 1000 {
				t.Fatalf("bits %d float %v ch %d: info %+v", f.bits, f.float, ch, info)
			}
			mem, err := codec.DecodeAll(d, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(mem.PCM) != len(pcm) {
				t.Fatalf("decoded %d samples, want %d", len(mem.PCM), len(pcm))
			}
			for i := range pcm {
				if d := math.Abs(float64(mem.PCM[i] - pcm[i])); d > f.tol {
					t.Fatalf("bits %d float %v ch %d: sample %d off by %v", f.bits, f.float, ch, i, d)
				}
			}
		}
	}
}

func TestWAVLoopAndSeek(t *testing.T) {
	pcm := ramp(2000, 1)
	data := wav.Encode(pcm, 48000, 1, wav.EncodeOptions{Bits: 32, Float: true, Loop: true, LoopStart: 100, LoopEnd: 200})
	d := open(t, data, "loop.wav")
	l, ok := d.(codec.Looper)
	if !ok {
		t.Fatal("wav decoder has no loop points")
	}
	if s, e, ok := l.Loop(); !ok || s != 100 || e != 200 {
		t.Fatalf("loop %d..%d %v", s, e, ok)
	}
	if err := d.SeekFrame(500); err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 10)
	if n, err := d.Read(buf); n != 10 || err != nil {
		t.Fatalf("read %d %v", n, err)
	}
	for i, v := range buf {
		if v != pcm[500+i] {
			t.Fatalf("after Seek sample %d = %v, want %v", i, v, pcm[500+i])
		}
	}
	mem, err := codec.DecodeAll(open(t, data, "loop.wav"), 96000)
	if err != nil {
		t.Fatal(err)
	}
	if mem.LoopStart != 200 || mem.LoopEnd != 400 || mem.Frames() != 4000 {
		t.Fatalf("resampled loop %d..%d, %d frames", mem.LoopStart, mem.LoopEnd, mem.Frames())
	}
}

func TestUnsupported(t *testing.T) {
	if _, err := codec.Open(bytes.NewReader([]byte("ID3\x04\x00\x00\x00\x00\x00\x00")), "a.mp3"); !errors.Is(err, codec.ErrMP3) {
		t.Fatalf("mp3: %v", err)
	}
	if _, err := codec.Open(bytes.NewReader([]byte("garbage")), "x.flac"); !errors.Is(err, codec.ErrUnknown) {
		t.Fatalf("unknown: %v", err)
	}
}

func readAll(t *testing.T, d codec.Decoder) []float32 {
	t.Helper()
	var out []float32
	buf := make([]float32, 1024*d.Info().Channels)
	for {
		n, err := d.Read(buf)
		out = append(out, buf[:n*d.Info().Channels]...)
		if errors.Is(err, io.EOF) || (err == nil && n == 0) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestVorbis(t *testing.T) {
	data, err := os.ReadFile("testdata/tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	d := open(t, data, "tone.ogg")
	info := d.Info()
	if info.SampleRate != 48000 || info.Channels != 1 {
		t.Fatalf("info %+v", info)
	}
	all := readAll(t, d)
	if len(all) < 22000 || len(all) > 26000 {
		t.Fatalf("decoded %d frames, want about 24000", len(all))
	}
	if info.Frames > 0 && math.Abs(float64(info.Frames-int64(len(all)))) > 2 {
		t.Fatalf("Info.Frames %d, decoded %d", info.Frames, len(all))
	}
	sum := 0.0
	for _, v := range all {
		sum += float64(v * v)
	}
	if rms := math.Sqrt(sum / float64(len(all))); rms < 0.1 {
		t.Fatalf("rms %v", rms)
	}
	mid := int64(len(all) / 2)
	if err := d.SeekFrame(mid); err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 256)
	n, err := d.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("read after seek: %d %v", n, err)
	}
	for i := 0; i < n; i++ {
		if math.Abs(float64(buf[i]-all[int(mid)+i])) > 1e-3 {
			t.Fatalf("after Seek sample %d = %v, want %v", i, buf[i], all[int(mid)+i])
		}
	}
}

func drain(d codec.Decoder, err error) {
	if err != nil {
		return
	}
	buf := make([]float32, 1024*d.Info().Channels)
	for i := 0; i < 1000; i++ {
		n, err := d.Read(buf)
		if err != nil || n == 0 {
			break
		}
	}
	_ = d.SeekFrame(d.Info().Frames / 2)
	_, _ = d.Read(buf)
	_ = d.Close()
}

func FuzzWAV(f *testing.F) {
	f.Add(wav.Encode(ramp(64, 1), 48000, 1, wav.EncodeOptions{Bits: 16}))
	f.Add(wav.Encode(ramp(64, 2), 22050, 2, wav.EncodeOptions{Bits: 24, Loop: true, LoopStart: 4, LoopEnd: 32}))
	f.Add(wav.Encode(ramp(64, 1), 48000, 1, wav.EncodeOptions{Float: true}))
	f.Add([]byte("RIFF\x00\x00\x00\x00WAVE"))
	f.Fuzz(func(t *testing.T, data []byte) {
		drain(codec.Open(bytes.NewReader(data), "fuzz.wav"))
	})
}

func FuzzVorbis(f *testing.F) {
	if data, err := os.ReadFile("testdata/tone.ogg"); err == nil {
		f.Add(data)
		f.Add(data[:len(data)/2])
		f.Add(data[:min(len(data), 200)])
	}
	f.Add([]byte("OggS"))
	f.Fuzz(func(t *testing.T, data []byte) {
		drain(codec.Open(bytes.NewReader(data), "fuzz.ogg"))
	})
}

func TestPeaks(t *testing.T) {
	pcm := make([]float32, 1000)
	for i := 500; i < len(pcm); i++ {
		pcm[i] = -0.5
	}
	p, err := codec.Peaks(open(t, wav.Encode(pcm, 48000, 1, wav.EncodeOptions{Float: true}), "p.wav"), 4)
	if err != nil {
		t.Fatal(err)
	}
	if p[0] != 0 || p[3] != 0.5 {
		t.Fatalf("peaks %v", p)
	}
}
