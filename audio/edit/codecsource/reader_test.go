package codecsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"aego/audio/codec"
	_ "aego/audio/codec/vorbis"
	"aego/audio/codec/wav"
)

type fakeDecoder struct {
	info      codec.Info
	pcm       []float32
	position  int
	maxRead   int
	readError error
	seekError error
	badCount  bool
	onRead    func()
	seeks     []int64
}

func (d *fakeDecoder) Info() codec.Info { return d.info }
func (d *fakeDecoder) Close() error     { return nil }
func (d *fakeDecoder) SeekFrame(frame int64) error {
	if d.seekError != nil {
		return d.seekError
	}
	d.seeks = append(d.seeks, frame)
	d.position = int(frame)
	return nil
}
func (d *fakeDecoder) Read(dst []float32) (int, error) {
	if d.onRead != nil {
		d.onRead()
	}
	if d.badCount {
		return len(dst)/d.info.Channels + 1, nil
	}
	if d.readError != nil {
		return 0, d.readError
	}
	want := len(dst) / d.info.Channels
	if d.maxRead > 0 {
		want = min(want, d.maxRead)
	}
	want = min(want, len(d.pcm)/d.info.Channels-d.position)
	if want <= 0 {
		return 0, io.EOF
	}
	copy(dst, d.pcm[d.position*d.info.Channels:(d.position+want)*d.info.Channels])
	d.position += want
	return want, nil
}

func TestReadFramesSeeksAndHandlesPartialReads(t *testing.T) {
	d := &fakeDecoder{info: codec.Info{SampleRate: 48000, Channels: 2, Frames: 4},
		pcm: []float32{0, 1, 2, 3, 4, 5, 6, 7}, maxRead: 1}
	r, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		start int64
		want  []float32
	}{{2, []float32{4, 5, 6, 7}}, {0, []float32{0, 1, 2, 3}}} {
		got, err := r.ReadFrames(context.Background(), tt.start, 2)
		if err != nil || !equalPCM(got, tt.want) {
			t.Fatalf("read at %d: %v, %v", tt.start, got, err)
		}
	}
	if got, err := r.ReadFrames(context.Background(), 4, 0); err != nil || len(got) != 0 {
		t.Fatalf("empty range: %v, %v", got, err)
	}
	if _, err := r.ReadFrames(context.Background(), 2, 2); err != nil || len(d.seeks) != 2 {
		t.Fatalf("sequential read sought again: %v, seeks %v", err, d.seeks)
	}
}

func TestReadFramesRejectsBadRangesAndDecoderResults(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("accepted a missing decoder")
	}
	if _, err := (&Reader{}).ReadFrames(context.Background(), 0, 1); err == nil {
		t.Fatal("accepted an uninitialized reader")
	}
	for _, info := range []codec.Info{{}, {SampleRate: 48000, Channels: 2, Frames: -1},
		{SampleRate: 48000, Channels: 3, Frames: 4}} {
		if _, err := New(&fakeDecoder{info: info}); err == nil {
			t.Fatalf("accepted invalid info %+v", info)
		}
	}
	d := &fakeDecoder{info: codec.Info{SampleRate: 48000, Channels: 1, Frames: int64(maxSamples) + 2}, pcm: []float32{1}}
	r, _ := New(d)
	for _, tc := range []struct {
		start  int64
		frames int
	}{{-1, 1}, {0, -1}, {d.info.Frames, 1}, {0, maxSamples + 1}} {
		if _, err := r.ReadFrames(context.Background(), tc.start, tc.frames); err == nil {
			t.Fatalf("accepted bad range %+v", tc)
		}
	}
	if _, err := r.ReadFrames(nil, 0, 1); err == nil {
		t.Fatal("accepted a nil context")
	}
	if _, err := r.ReadFrames(context.Background(), 0, 2); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short decoder read: %v", err)
	}
	d.badCount = true
	if _, err := r.ReadFrames(context.Background(), 0, 1); err == nil {
		t.Fatal("accepted an invalid decoder frame count")
	}
	d.badCount = false
	d.readError = io.ErrNoProgress
	if _, err := r.ReadFrames(context.Background(), 0, 1); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("decoder error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.readError, d.onRead = nil, cancel
	if _, err := r.ReadFrames(ctx, 0, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation after read: %v", err)
	}
}

func TestReadFramesReseeksAfterIncompleteOrCanceledRead(t *testing.T) {
	d := &fakeDecoder{info: codec.Info{SampleRate: 48000, Channels: 1, Frames: 4}, pcm: []float32{10}}
	r, _ := New(d)
	if _, err := r.ReadFrames(context.Background(), 0, 2); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected an incomplete read: %v", err)
	}
	d.pcm = []float32{10, 20, 30, 40}
	got, err := r.ReadFrames(context.Background(), 0, 1)
	if err != nil || !equalPCM(got, []float32{10}) || len(d.seeks) != 2 {
		t.Fatalf("did not re-seek after partial data: %v %v, seeks %v", got, err, d.seeks)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.onRead = cancel
	if _, err := r.ReadFrames(ctx, 1, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
	d.onRead = nil
	got, err = r.ReadFrames(context.Background(), 1, 1)
	if err != nil || !equalPCM(got, []float32{20}) || len(d.seeks) != 3 || d.seeks[2] != 1 {
		t.Fatalf("did not re-seek after cancellation: %v %v, seeks %v", got, err, d.seeks)
	}
}

func equalPCM(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > 1e-6 {
			return false
		}
	}
	return true
}

func TestReadFramesSerializesConcurrentSeeks(t *testing.T) {
	pcm := make([]float32, 1024)
	for i := range pcm {
		pcm[i] = float32(i)
	}
	r, _ := New(&fakeDecoder{info: codec.Info{SampleRate: 48000, Channels: 1, Frames: int64(len(pcm))}, pcm: pcm, maxRead: 7})
	var wg sync.WaitGroup
	for n := range 8 {
		wg.Go(func() {
			for i := range 40 {
				start := (n*83 + i*3) % (len(pcm) - 20)
				got, err := r.ReadFrames(context.Background(), int64(start), 20)
				if err != nil || !equalPCM(got, pcm[start:start+20]) {
					t.Errorf("concurrent read %d: %v", start, err)
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestReadFramesWithActualWAVAndOggDecoders(t *testing.T) {
	pcm := make([]float32, 2048)
	for i := range pcm {
		pcm[i] = float32(i%37-18) / 32
	}
	ogg, err := os.ReadFile(filepath.Join("..", "..", "codec", "testdata", "tone.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		raw  []byte
	}{{"fixture.wav", wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})}, {"fixture.ogg", ogg}} {
		t.Run(tt.name, func(t *testing.T) {
			before := sha256.Sum256(tt.raw)
			full, err := codec.Open(bytes.NewReader(tt.raw), tt.name)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := codec.DecodeAll(full, 0)
			full.Close()
			if err != nil {
				t.Fatal(err)
			}
			dec, err := codec.Open(bytes.NewReader(tt.raw), tt.name)
			if err != nil {
				t.Fatal(err)
			}
			defer dec.Close()
			reader, err := New(dec)
			if err != nil {
				t.Fatal(err)
			}
			for _, start := range []int64{0, reader.info.Frames / 2, reader.info.Frames - 16} {
				got, err := reader.ReadFrames(context.Background(), start, 16)
				at := int(start) * reader.info.Channels
				if err != nil || !equalPCM(got, decoded.PCM[at:at+16*reader.info.Channels]) {
					t.Fatalf("seek %d did not match sequential decode: %v", start, err)
				}
			}
			if sha256.Sum256(tt.raw) != before {
				t.Fatal("decoding changed encoded source bytes")
			}
		})
	}
}
