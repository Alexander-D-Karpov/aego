package editor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"testing"

	"aego/asset"
	"aego/asset/audiosidecar"
	"aego/audio/codec"
	"aego/audio/edit"
	"aego/format/aetext"
)

// This exercises the shared journal and private preview with a synthetic WAV.
// It deliberately does not attach volatile editing to the user interface or
// authorize saving an audio edit envelope to disk.
func TestVolatileAudioHistoryAndPrivatePreview(t *testing.T) {
	f := newWorkingAudioFixture(t, ".wav", false)
	clip, err := f.module.Clip(f.ed.Loader, f.id)
	if err != nil || clip == nil || clip.Art == nil {
		t.Fatal("cannot load the synthetic source", err)
	}
	journal := f.doc.Metadata.Code
	initial := bytes.Clone(journal.Bytes()) // existing unsaved gain change
	if err := journal.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	base, err := audioedit.New(audioedit.Source{AssetID: f.id.String(), Path: f.doc.Path,
		SHA256: fmt.Sprintf("%x", sha256.Sum256(f.source)), Codec: "wav",
		SampleRate: clip.Art.Rate, Channels: clip.Art.Channels, Frames: clip.Art.Frames})
	if err != nil {
		t.Fatal(err)
	}
	next, changed, err := audioedit.PrepareMutation(base, base.Revision, func(d *audioedit.Document) error {
		audioedit.AddOperation(d, audioedit.Operation{Kind: audioedit.Gain, Region: d.Trim, Value: .5})
		return nil
	})
	if err != nil || !changed {
		t.Fatal("cannot prepare an audio edit", err)
	}
	encoded, err := next.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := journal.ReplaceVolatile(journal.Rev(), encoded); err != nil || !changed {
		t.Fatal("cannot record the audio edit in the metadata journal", err)
	}
	meta, err := asset.ParseSourceMeta(journal.Bytes(), f.doc.Metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	updated := meta.Settings.Clone()
	updated["loopStart"], updated["loopEnd"] = aetext.Int(100), aetext.Int(20000)
	f.plugin.write(f.ed, meta.Settings, updated)
	withLoop := bytes.Clone(journal.Bytes())
	if bytes.Equal(withLoop, initial) || bytes.Contains(withLoop, []byte("audioEdit")) || !f.doc.Dirty() {
		t.Fatal("the shared text journal lost the loop edit or contains the private EDL")
	}
	if err := f.ed.SaveAsset(f.doc); err == nil {
		t.Fatal("ordinary Save published a partial audio document")
	}
	if err := f.ed.Docs.SaveAll(); err == nil {
		t.Fatal("Save All published a partial audio document")
	}
	if _, err := f.ed.Docs.WriteRecovery(); err == nil {
		t.Fatal("text-only recovery claimed to preserve the audio edit")
	}
	f.checkFiles(t, f.source, f.meta)
	if err := f.ed.UndoAsset(f.doc); err != nil || !bytes.Equal(journal.Bytes(), initial) || !journal.VolatileDirty() {
		t.Fatal("first Undo did not restore only the loop settings", err)
	}
	if err := f.ed.UndoAsset(f.doc); err != nil || !bytes.Equal(journal.Bytes(), initial) || journal.VolatileDirty() {
		t.Fatal("second Undo did not restore only the audio edit", err)
	}
	if err := f.ed.UndoAsset(f.doc); err != nil || f.doc.Dirty() || !bytes.Equal(journal.Bytes(), f.meta) {
		t.Fatal("third Undo did not restore the saved metadata", err)
	}
	for range 3 {
		if err := f.ed.RedoAsset(f.doc); err != nil {
			t.Fatal("cannot redo shared audio history", err)
		}
	}
	state, attached := journal.Volatile()
	if !attached || !journal.VolatileDirty() || !bytes.Equal(state, encoded) || !bytes.Equal(journal.Bytes(), withLoop) {
		t.Fatal("Redo did not restore the entire shared audio document")
	}
	decoded, err := audioedit.Decode(state)
	if err != nil || decoded.Revision != next.Revision || len(decoded.Operations) != 1 {
		t.Fatal("volatile edit is not the prepared audio candidate", err)
	}
	// The version-2 envelope is built only in memory for the guarded preview.
	candidate, err := audiosidecar.Encode(journal.Bytes(), f.doc.Path, decoded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.ed.Pipeline.PreviewGuardedWithMetadataContext(context.Background(), f.doc.Path, f.source, candidate)
	if err != nil || result == nil || result.Kind != asset.ArtAudio || len(result.Artifacts) != 1 {
		t.Fatalf("private edited preview failed: %+v %v", result, err)
	}
	artifact, err := asset.DecodeAudio(result.Artifacts[0].Data)
	if err != nil || artifact.Format != asset.AudioWAV || artifact.Gain != 2 || artifact.LoopStart != 100 || artifact.LoopEnd != 20000 {
		t.Fatalf("private preview lost audio or metadata settings: %+v %v", artifact, err)
	}
	decoder, err := codec.Open(bytes.NewReader(artifact.Data), "preview.wav")
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := codec.DecodeAll(decoder, 0)
	closeErr := decoder.Close()
	if err != nil || closeErr != nil || len(pcm.PCM) != 48000 {
		t.Fatal("private preview could not be decoded", err, closeErr)
	}
	for _, frame := range []int{0, 100, 10000, 47999} {
		if math.Abs(float64(pcm.PCM[frame]-.125)) > 1e-6 {
			t.Fatalf("frame %d is not the edited signal: %v", frame, pcm.PCM[frame])
		}
	}
	f.checkFiles(t, f.source, f.meta)
}
