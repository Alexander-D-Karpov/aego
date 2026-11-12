package audiosidecar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"testing"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/edit"
	"aego/audio/edit/codecsource"
)

// In-memory integration only: the production importer and Save paths still
// reject the version-2 metadata rather than applying this edit.
func TestSidecarAndEditedDecoderSyntheticWAV(t *testing.T) {
	samples := []float32{.1, .2, .3, .4, .5, .6, .7, .8}
	raw := wav.Encode(samples, 8000, 1, wav.EncodeOptions{Float: true})
	digest := sha256.Sum256(raw)
	meta := asset.NewSourceMeta("audio", 1, nil)
	const path = "assets/tone.wav"
	doc, err := audioedit.New(audioedit.Source{AssetID: meta.ID.String(), Path: path, SHA256: fmt.Sprintf("%x", digest),
		Codec: "wav", SampleRate: 8000, Channels: 1, Frames: int64(len(samples))})
	if err != nil {
		t.Fatal(err)
	}
	doc.Trim = audioedit.Range{Start: 1, End: 7}
	doc.Operations = []audioedit.Operation{
		{Kind: audioedit.Gain, Region: audioedit.Range{Start: 0, End: 8}, Value: .5},
		{Kind: audioedit.Reverse, Region: audioedit.Range{Start: 2, End: 6}},
	}
	encoded, err := Encode(asset.MarshalSourceMeta(meta), path, doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := asset.ParseSourceMeta(encoded, path+asset.MetaSuffix); err == nil {
		t.Fatal("legacy metadata reader silently accepted the synthetic edit")
	}
	decoded, present, err := Decode(encoded, path)
	if err != nil || !present || decoded.Source.SHA256 != fmt.Sprintf("%x", sha256.Sum256(raw)) {
		t.Fatal("edit did not retain the encoded source identity", err)
	}
	source, err := codec.Open(bytes.NewReader(raw), path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := codecsource.NewEditedDecoder(context.Background(), source, decoded)
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	defer reader.Close()
	memory, err := codec.DecodeAll(reader, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []float32{.1, .3, .25, .2, .15, .35}
	if len(memory.PCM) != len(want) {
		t.Fatalf("edited frame count %d, want %d", len(memory.PCM), len(want))
	}
	for i := range want {
		if math.Abs(float64(memory.PCM[i]-want[i])) > 1e-6 {
			t.Fatalf("edited frame %d = %v, want %v", i, memory.PCM[i], want[i])
		}
	}
	if sha256.Sum256(raw) != digest {
		t.Fatal("in-memory processing modified the source")
	}
}
