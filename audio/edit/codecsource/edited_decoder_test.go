package codecsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/edit"
	"aego/audio/mix"
)

func editedFixtureDocument(t *testing.T, raw []byte) audioedit.Document {
	t.Helper()
	d, err := codec.Open(bytes.NewReader(raw), "")
	if err != nil {
		t.Fatal(err)
	}
	info := d.Info()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if info.Frames < 32 {
		t.Fatalf("fixture is too short: %+v", info)
	}
	doc, err := audioedit.New(audioedit.Source{AssetID: "synthetic", Path: "assets/test.wav", SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)),
		Codec: "test", SampleRate: info.SampleRate, Channels: info.Channels, Frames: info.Frames})
	if err != nil {
		t.Fatal(err)
	}
	doc.Trim = audioedit.Range{Start: info.Frames / 8, End: info.Frames - info.Frames/6}
	doc.Loop = &audioedit.Range{Start: doc.Trim.Start + 2, End: doc.Trim.End - 3}
	doc.Operations = []audioedit.Operation{
		{Kind: audioedit.Gain, Region: audioedit.Range{Start: 0, End: info.Frames}, Value: .65},
		{Kind: audioedit.Reverse, Region: audioedit.Range{Start: doc.Trim.Start + 3, End: doc.Trim.End - 2}},
		{Kind: audioedit.FadeIn, Region: audioedit.Range{Start: doc.Trim.Start, End: doc.Trim.End}},
	}
	return doc
}

func editedExpected(t *testing.T, raw []byte, doc audioedit.Document) []float32 {
	t.Helper()
	d, err := codec.Open(bytes.NewReader(raw), "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	r, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	var expected []float32
	for start := doc.Trim.Start; start < doc.Trim.End; {
		end := min(start+4096, doc.Trim.End)
		data, err := audioedit.RenderWindow(context.Background(), r, doc, audioedit.Range{Start: start, End: end}, 4096)
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, data...)
		start = end
	}
	return expected
}

func TestEditedDecoderReadSeekAndImmutableDocument(t *testing.T) {
	pcm := make([]float32, 256*2)
	for i := range pcm {
		pcm[i] = float32(i%31-15) / 32
	}
	raw := wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})
	doc := editedFixtureDocument(t, raw)
	expected := editedExpected(t, raw, doc)
	d, err := codec.Open(bytes.NewReader(raw), "")
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewEditedDecoder(context.Background(), d, doc)
	if err != nil {
		d.Close()
		t.Fatal(err)
	}
	defer w.Close()
	info := w.Info()
	if info.Frames != doc.Trim.Length() || info.Channels != 2 || info.SampleRate != 48000 {
		t.Fatalf("edited info: %+v", info)
	}
	start, end, ok := w.Loop()
	if !ok || start != doc.Loop.Start-doc.Trim.Start || end != doc.Loop.End-doc.Trim.Start {
		t.Fatalf("loop not mapped to trimmed frames: %d %d %v", start, end, ok)
	}
	// The decoder owns an immutable snapshot, not the caller's retained slices.
	doc.Operations[0].Value = 0
	doc.Loop.Start++
	var got []float32
	buf := make([]float32, 13*info.Channels)
	for {
		n, err := w.Read(buf)
		got = append(got, buf[:n*info.Channels]...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || n == 0 {
			t.Fatalf("edited decoder stalled: %d %v", n, err)
		}
	}
	if !equalPCM(got, expected) {
		t.Fatal("edited sequential PCM differs from the bounded renderer")
	}
	if err := w.SeekFrame(7); err != nil {
		t.Fatal(err)
	}
	n, err := w.Read(buf)
	if err != nil || n != 13 || !equalPCM(buf[:n*info.Channels], expected[7*info.Channels:(7+n)*info.Channels]) {
		t.Fatalf("edited seek differs from sequential PCM: frames=%d err=%v", n, err)
	}
	if err := w.SeekFrame(-1); err == nil {
		t.Fatal("accepted a negative seek")
	}
	if err := w.SeekFrame(info.Frames + 1); err == nil {
		t.Fatal("accepted a seek past the edited end")
	}
	if err := w.SeekFrame(info.Frames); err != nil {
		t.Fatal(err)
	}
	if n, err := w.Read(buf); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF at edited end, got %d %v", n, err)
	}
}

func TestEditedDecoderKnownTrimReverseAndGain(t *testing.T) {
	base := &fakeDecoder{info: codec.Info{SampleRate: 48000, Channels: 1, Frames: 8},
		pcm: []float32{.1, .2, .3, .4, .5, .6, .7, .8}}
	doc, err := audioedit.New(audioedit.Source{AssetID: "source", SHA256: fmt.Sprintf("%064x", 1), SampleRate: 48000, Channels: 1, Frames: 8})
	if err != nil {
		t.Fatal(err)
	}
	doc.Trim = audioedit.Range{Start: 2, End: 7}
	doc.Operations = []audioedit.Operation{
		{Kind: audioedit.Gain, Region: audioedit.Range{Start: 0, End: 8}, Value: .5},
		{Kind: audioedit.Reverse, Region: audioedit.Range{Start: 3, End: 6}},
	}
	w, err := NewEditedDecoder(context.Background(), base, doc)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	got := make([]float32, 5)
	if n, err := w.Read(got); err != nil || n != 5 || !equalPCM(got, []float32{.15, .3, .25, .2, .35}) {
		t.Fatalf("wrong edited frames: %v, n=%d, err=%v", got, n, err)
	}
	if start, end, ok := w.Loop(); ok || start != 0 || end != 0 {
		t.Fatalf("invented an implicit loop: %d %d %v", start, end, ok)
	}
}

type countedDecoder struct {
	codec.Decoder
	closes   int
	closeErr error
}

func (d *countedDecoder) Close() error {
	d.closes++
	return d.closeErr
}

func TestEditedDecoderBoundsErrorsAndOwnership(t *testing.T) {
	const frames = editedChunkFrames + 8
	pcm := make([]float32, frames*2)
	for i := range pcm {
		pcm[i] = float32(i%19-9) / 16
	}
	base := &fakeDecoder{info: codec.Info{SampleRate: 48000, Channels: 2, Frames: frames}, pcm: pcm}
	closeFailure := errors.New("close failure")
	owned := &countedDecoder{Decoder: base, closeErr: closeFailure}
	doc, err := audioedit.New(audioedit.Source{AssetID: "source", SHA256: fmt.Sprintf("%064x", 1), SampleRate: 48000, Channels: 2, Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	bad := doc.Clone()
	bad.Source.SampleRate++
	if _, err := NewEditedDecoder(context.Background(), owned, bad); err == nil || owned.closes != 0 {
		t.Fatal("rejected constructor took ownership of the decoder")
	}
	w, err := NewEditedDecoder(context.Background(), owned, doc)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := w.Read(nil); n != 0 || err != nil {
		t.Fatalf("zero-length read: %d %v", n, err)
	}
	if n, err := w.Read([]float32{1}); n != 0 || err == nil {
		t.Fatalf("accepted a partial stereo frame: %d %v", n, err)
	}
	dst := make([]float32, len(pcm))
	base.readError = io.ErrNoProgress
	dst[0] = .875
	if n, err := w.Read(dst); n != 0 || !errors.Is(err, io.ErrNoProgress) || dst[0] != .875 {
		t.Fatalf("decoder failure modified output: %d %v %v", n, err, dst[0])
	}
	base.readError = nil
	if n, err := w.Read(dst); err != nil || n != editedChunkFrames || !equalPCM(dst[:n*2], pcm[:n*2]) {
		t.Fatalf("retry did not restart or respect the chunk bound: %d %v", n, err)
	}
	if err := w.SeekFrame(10); err != nil {
		t.Fatal(err)
	}
	base.maxRead = 1
	reads := 0
	base.onRead = func() {
		reads++
		if reads == 2 {
			base.readError = io.ErrNoProgress
		}
	}
	dst[0] = .875
	if n, err := w.Read(dst[:8]); n != 0 || !errors.Is(err, io.ErrNoProgress) || dst[0] != .875 {
		t.Fatalf("partial decoder failure exposed samples: %d %v %v", n, err, dst[0])
	}
	base.onRead, base.readError = nil, nil
	if n, err := w.Read(dst[:8]); err != nil || n != 4 || !equalPCM(dst[:8], pcm[20:28]) {
		t.Fatalf("retry after a partial read did not restore position: %d %v", n, err)
	}
	if err := w.Close(); !errors.Is(err, closeFailure) {
		t.Fatal("did not propagate decoder close failure", err)
	}
	if err := w.Close(); !errors.Is(err, closeFailure) || owned.closes != 1 {
		t.Fatalf("close was not idempotent: calls=%d err=%v", owned.closes, err)
	}
	if n, err := w.Read(dst); n != 0 || err == nil {
		t.Fatal("read from a closed decoder", n, err)
	}
	if err := w.SeekFrame(0); err == nil {
		t.Fatal("sought a closed decoder")
	}
}

func TestEditedDecoderSerializesConcurrentReadsAndSeeks(t *testing.T) {
	const frames = 1024
	pcm := make([]float32, frames)
	for i := range pcm {
		pcm[i] = float32(i) / 1024
	}
	base := &fakeDecoder{info: codec.Info{SampleRate: 48000, Channels: 1, Frames: frames}, pcm: pcm, maxRead: 3}
	doc, err := audioedit.New(audioedit.Source{AssetID: "source", SHA256: fmt.Sprintf("%064x", 1), SampleRate: 48000, Channels: 1, Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewEditedDecoder(context.Background(), base, doc)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var wg sync.WaitGroup
	for worker := range 6 {
		wg.Go(func() {
			buf := make([]float32, 8)
			for i := range 50 {
				if err := w.SeekFrame(int64((worker*101 + i*7) % (frames - 128))); err != nil {
					t.Error(err)
					return
				}
				n, err := w.Read(buf)
				if err != nil || n != len(buf) {
					t.Errorf("concurrent read: frames=%d error=%v", n, err)
					return
				}
				at := int(buf[0] * 1024)
				if at < 0 || at+n > len(pcm) || !equalPCM(buf, pcm[at:at+n]) {
					t.Errorf("concurrent read returned a torn window at %d", at)
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestEditedDecoderActualWAVAndOggMemoryAndStream(t *testing.T) {
	pcm := make([]float32, 4800*2)
	for i := range pcm {
		pcm[i] = float32(i%41-20) / 32
	}
	ogg, err := os.ReadFile(filepath.Join("..", "..", "codec", "testdata", "tone.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
	}{{"wav", wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})}, {"ogg", ogg}} {
		t.Run(tc.name, func(t *testing.T) {
			original := sha256.Sum256(tc.raw)
			doc := editedFixtureDocument(t, tc.raw)
			expected := editedExpected(t, tc.raw, doc)
			open := func() *EditedDecoder {
				d, err := codec.Open(bytes.NewReader(tc.raw), "")
				if err != nil {
					t.Fatal(err)
				}
				w, err := NewEditedDecoder(context.Background(), d, doc)
				if err != nil {
					d.Close()
					t.Fatal(err)
				}
				return w
			}
			memory := open()
			mem, err := codec.DecodeAll(memory, 0)
			if err != nil || !equalPCM(mem.PCM, expected) {
				t.Fatalf("memory decode differs from edited PCM: %v", err)
			}
			if mem.LoopStart != doc.Loop.Start-doc.Trim.Start || mem.LoopEnd != doc.Loop.End-doc.Trim.Start {
				t.Fatalf("memory loop does not use trimmed coordinates: %+v", mem)
			}
			if err := memory.Close(); err != nil {
				t.Fatal(err)
			}
			stream := open()
			defer stream.Close()
			st := mix.NewStream(doc.Source.SampleRate, doc.Source.Channels, int(doc.Trim.Length())+1)
			cancel := make(chan struct{})
			done := codec.Stream(stream, st, codec.StreamOptions{Chunk: 127}, cancel)
			select {
			case err := <-done:
				if err != nil {
					t.Fatal("stream decode failed", err)
				}
			case <-time.After(10 * time.Second):
				close(cancel)
				<-done
				t.Fatal("stream decode did not finish")
			}
			got := make([]float32, len(expected))
			if n := st.Ring.Read(got); n != int(doc.Trim.Length()) || !st.EOF() || !equalPCM(got, expected) {
				t.Fatalf("stream differs from memory: frames=%d eof=%v", n, st.EOF())
			}
			if sha256.Sum256(tc.raw) != original {
				t.Fatal("decoding edited audio changed the encoded source")
			}
		})
	}
}

func TestEditedDecoderStreamUsesTrimRelativeLoop(t *testing.T) {
	pcm := []float32{.05, .1, .15, .2, .25, .3, .35, .4}
	base := &fakeDecoder{info: codec.Info{SampleRate: 48000, Channels: 1, Frames: int64(len(pcm))}, pcm: pcm}
	doc, err := audioedit.New(audioedit.Source{AssetID: "source", SHA256: fmt.Sprintf("%064x", 1), SampleRate: 48000, Channels: 1, Frames: int64(len(pcm))})
	if err != nil {
		t.Fatal(err)
	}
	doc.Trim = audioedit.Range{Start: 1, End: 8}
	doc.Loop = &audioedit.Range{Start: 3, End: 6}
	w, err := NewEditedDecoder(context.Background(), base, doc)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	st := mix.NewStream(48000, 1, 32)
	cancel := make(chan struct{})
	done := codec.Stream(w, st, codec.StreamOptions{Loop: true, Chunk: 4}, cancel)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for st.Ring.Len() < 20 {
		select {
		case err := <-done:
			t.Fatalf("looping stream stopped early: %v", err)
		case <-timer.C:
			close(cancel)
			<-done
			t.Fatal("looping stream did not fill its bounded ring")
		case <-ticker.C:
		}
	}
	close(cancel)
	if err := <-done; err != nil {
		t.Fatal("could not cancel looping stream", err)
	}
	got := make([]float32, st.Ring.Len())
	n := st.Ring.Read(got)
	if n < 20 {
		t.Fatalf("not enough loop samples: %d", n)
	}
	for i, sample := range got[:n] {
		source := i + 1
		if i >= 5 { // Source loop [3,6) maps to output [2,5).
			source = 3 + (i-5)%3
		}
		if sample != pcm[source] {
			t.Fatalf("loop sample %d = %v, want %v", i, sample, pcm[source])
		}
	}
}

func TestEditedDecoderRejectsInvalidInputAndPropagatesCancellation(t *testing.T) {
	info := codec.Info{SampleRate: 48000, Channels: 1, Frames: 4}
	base := &fakeDecoder{info: info, pcm: []float32{.1, .2, .3, .4}}
	doc, err := audioedit.New(audioedit.Source{AssetID: "source", SHA256: fmt.Sprintf("%064x", 1), SampleRate: 48000, Channels: 1, Frames: 4})
	if err != nil {
		t.Fatal(err)
	}
	var absent context.Context // Exercise the API's explicit nil-context rejection.
	if _, err := NewEditedDecoder(absent, base, doc); err == nil {
		t.Fatal("accepted a nil context")
	}
	if _, err := NewEditedDecoder(context.Background(), nil, doc); err == nil {
		t.Fatal("accepted a nil source decoder")
	}
	wrong := doc.Clone()
	wrong.Source.Frames++
	if _, err := NewEditedDecoder(context.Background(), base, wrong); err == nil {
		t.Fatal("accepted a decoder with a different source length")
	}
	wrong = doc.Clone()
	wrong.Source.Channels = 2
	if _, err := NewEditedDecoder(context.Background(), base, wrong); err == nil {
		t.Fatal("accepted a decoder with different channels")
	}
	wrong = doc.Clone()
	wrong.Operations = make([]audioedit.Operation, 4097)
	if _, err := NewEditedDecoder(context.Background(), base, wrong); err == nil {
		t.Fatal("accepted an edit document over its operation budget")
	}
	ctx, cancel := context.WithCancel(context.Background())
	w, err := NewEditedDecoder(ctx, base, doc)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	dst := []float32{.9, .9}
	if n, err := w.Read(dst); n != 0 || !errors.Is(err, context.Canceled) || dst[0] != .9 || dst[1] != .9 {
		t.Fatalf("cancellation lost or modified output: %d %v %v", n, err, dst)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	base.onRead = cancel
	w, err = NewEditedDecoder(ctx, base, doc)
	if err != nil {
		t.Fatal(err)
	}
	dst = []float32{.9, .9}
	if n, err := w.Read(dst); n != 0 || !errors.Is(err, context.Canceled) || dst[0] != .9 || dst[1] != .9 {
		t.Fatalf("cancellation during decoding lost or modified output: %d %v %v", n, err, dst)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}
