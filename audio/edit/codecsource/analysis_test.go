package codecsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"aego/audio/codec"
	_ "aego/audio/codec/vorbis"
	"aego/audio/codec/wav"
	"aego/audio/edit"
)

// Compare the bounded random-access analysis to independently decoded PCM,
// not to RenderWindow, on actual WAV and Vorbis decoders.
func TestAnalyzeRealWAVAndOgg(t *testing.T) {
	pcm := make([]float32, 2048*2)
	for i := range pcm {
		pcm[i] = float32(i%41-20) / 24
	}
	ogg, err := os.ReadFile(filepath.Join("..", "..", "codec", "testdata", "tone.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
	}{{"synthetic.wav", wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})}, {"fixture.ogg", ogg}} {
		t.Run(tc.name, func(t *testing.T) {
			before := sha256.Sum256(tc.raw)
			full, err := codec.Open(bytes.NewReader(tc.raw), tc.name)
			if err != nil {
				t.Fatal(err)
			}
			info := full.Info()
			decoded, err := codec.DecodeAll(full, 0)
			closeErr := full.Close()
			if err != nil || closeErr != nil || info.Frames < 32 || int64(len(decoded.PCM)) != info.Frames*int64(info.Channels) {
				t.Fatalf("cannot sequentially decode source: info=%+v decode=%v close=%v", info, err, closeErr)
			}
			doc, err := audioedit.New(audioedit.Source{AssetID: "synthetic", Path: tc.name,
				SHA256: fmt.Sprintf("%x", before), Codec: "fixture", SampleRate: info.SampleRate,
				Channels: info.Channels, Frames: info.Frames})
			if err != nil {
				t.Fatal(err)
			}
			region := audioedit.Range{Start: info.Frames / 3, End: info.Frames * 4 / 5}
			doc.Trim = region
			doc.Operations = []audioedit.Operation{
				{Kind: audioedit.Gain, Region: audioedit.Range{Start: 0, End: info.Frames}, Value: .5},
				{Kind: audioedit.Reverse, Region: audioedit.Range{Start: 0, End: info.Frames}},
				{Kind: audioedit.FadeIn, Region: region},
			}
			dec, err := codec.Open(bytes.NewReader(tc.raw), tc.name)
			if err != nil {
				t.Fatal(err)
			}
			defer dec.Close()
			reader, err := New(dec)
			if err != nil {
				t.Fatal(err)
			}
			got, err := audioedit.Analyze(context.Background(), reader, doc, region)
			if err != nil || got.Frames != region.Length() || len(got.Channels) != info.Channels {
				t.Fatalf("analysis dimensions: %+v, %v", got, err)
			}
			for c := range info.Channels {
				peak, sum := 0.0, 0.0
				clipped := int64(0)
				for frame := region.Start; frame < region.End; frame++ {
					original := info.Frames - 1 - frame
					fade := float64(frame-region.Start) / float64(region.Length()-1)
					value := float64(decoded.PCM[original*int64(info.Channels)+int64(c)]) * .5 * fade
					peak = max(peak, math.Abs(value))
					sum += value * value
					if math.Abs(value) >= 1 {
						clipped++
					}
				}
				wantRMS := math.Sqrt(sum / float64(region.Length()))
				channel := got.Channels[c]
				if math.Abs(channel.Peak-peak) > 1e-6 || math.Abs(channel.RMS-wantRMS) > 1e-6 || channel.ClippedFrames != clipped {
					t.Fatalf("channel %d: got %+v, want peak=%g RMS=%g clipped=%d", c, channel, peak, wantRMS, clipped)
				}
			}
			if sha256.Sum256(tc.raw) != before {
				t.Fatal("analysis changed the encoded source")
			}
		})
	}
}
