package audioedit

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"
)

func TestRenderReverseGainAndFade(t *testing.T) {
	d, _ := New(Source{AssetID: "id", SHA256: testSourceDigest, Codec: "wav", SampleRate: 48000, Channels: 1, Frames: 8})
	d.Operations = []Operation{{Kind: Reverse, Region: Range{0, 8}}, {Kind: Gain, Region: Range{0, 8}, Value: .5}}
	reader := fakePCM{channels: 1, samples: []float32{0, .1, .2, .3, .4, .5, .6, .7}}
	out, err := RenderWindow(context.Background(), reader, d, Range{2, 5}, 10)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []float32{.25, .2, .15} {
		if math.Abs(float64(out[i]-want)) > 0.00001 {
			t.Fatalf("frame %d: %v want %v", i, out[i], want)
		}
	}
	d.Operations = []Operation{{Kind: FadeIn, Region: Range{0, 8}}}
	out, err = RenderWindow(context.Background(), reader, d, Range{0, 8}, 8)
	if err != nil || out[0] != 0 || math.Abs(float64(out[7]-.7)) > 0.00001 {
		t.Fatalf("fade %v: %v", err, out)
	}
}
func TestWAVExportSeparateSource(t *testing.T) {
	d, _ := New(Source{AssetID: "id", SHA256: testSourceDigest, Codec: "ogg", SampleRate: 48000, Channels: 2, Frames: 480})
	samples := make([]float32, 960)
	for i := range samples {
		samples[i] = .25
	}
	var b bytes.Buffer
	if err := ExportWAV16(context.Background(), &b, fakePCM{channels: 2, samples: samples}, d, Range{0, 480}); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	if got, want := len(raw), 44+480*2*2; got != want {
		t.Fatalf("wav length %d want %d", got, want)
	}
	if string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" || binary.LittleEndian.Uint32(raw[40:44]) != 1920 {
		t.Fatal("invalid WAV header")
	}
	if raw[44] == 0 && raw[45] == 0 {
		t.Fatal("silent WAV")
	}
	if _, err := NormalizeMultiplier(.5, .98); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeMultiplier(math.NaN(), .98); err == nil {
		t.Fatal("accepted NaN")
	}
}
func TestZeroSnap(t *testing.T) {
	d, _ := New(Source{AssetID: "id", SHA256: testSourceDigest, Codec: "wav", SampleRate: 48000, Channels: 1, Frames: 8})
	pcm := fakePCM{channels: 1, samples: []float32{-.2, -.2, -.1, .01, .2, .1, .1, .1}}
	got, err := NearestZeroCrossing(context.Background(), pcm, d.Source, 2, 3)
	if err != nil || got != 3 {
		t.Fatalf("snap %d %v", got, err)
	}
}
