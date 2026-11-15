package audio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/edit"
	"aego/audio/edit/codecsource"
	"aego/core/ids"
)

// This verifies the shared kernel can consume a real imported artifact from
// the importer layer. It does not claim that Import applies or saves an EDL.
func TestSharedEditKernelCanRenderImportedWAVWithoutChangingSource(t *testing.T) {
	const frames = 128
	pcm := make([]float32, frames*2)
	for i := 0; i < len(pcm); i += 2 {
		pcm[i], pcm[i+1] = .5, -.25
	}
	source := wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})
	before := sha256.Sum256(source)
	id := ids.NewAssetID()
	imp := New()
	meta := pipeline.NewMeta(imp.ID(), imp.Version(), imp.Defaults())
	meta.ID = id
	result, err := imp.Import(&pipeline.ImportContext{ID: id, Path: "tone.wav", Source: source, Meta: meta})
	if err != nil || result == nil || len(result.Artifacts) != 1 {
		t.Fatalf("cannot import synthetic WAV: %+v, %v", result, err)
	}
	artifact, err := asset.DecodeAudio(result.Artifacts[0].Data)
	if err != nil || !bytes.Equal(artifact.Data, source) {
		t.Fatal("import changed the encoded source", err)
	}
	decoder, err := codec.Open(bytes.NewReader(artifact.Data), "tone.wav")
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	reader, err := codecsource.New(decoder)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := audioedit.New(audioedit.Source{AssetID: id.String(), Path: "tone.wav", SHA256: fmt.Sprintf("%x", before),
		Codec: "wav", SampleRate: 48000, Channels: 2, Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	doc.Operations = []audioedit.Operation{{Kind: audioedit.Gain, Region: audioedit.Range{Start: 0, End: frames}, Value: .5}}
	edited, err := audioedit.RenderWindow(context.Background(), reader, doc, doc.Trim, frames)
	if err != nil || len(edited) != len(pcm) {
		t.Fatalf("shared render failed: samples=%d error=%v", len(edited), err)
	}
	for i, sample := range edited {
		if sample != pcm[i]*.5 {
			t.Fatalf("sample %d = %v, want %v", i, sample, pcm[i]*.5)
		}
	}
	if sha256.Sum256(source) != before {
		t.Fatal("import or rendering changed source bytes")
	}
}
