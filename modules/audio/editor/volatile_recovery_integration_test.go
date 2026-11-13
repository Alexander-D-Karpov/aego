package editor

import (
	"bytes"
	"context"
	"math"
	"testing"

	"aego/asset"
	"aego/editor/core/code"
	"aego/format/aetext"
)

// This is an in-memory simulation, not an implemented recovery workflow. The
// production owner must verify the open document, source and disk preimages,
// and obtain an explicit user decision before adopting any decoded journal.
func TestVolatileRecoveryAudioHistoryAndPrivatePreview(t *testing.T) {
	f := newWorkingAudioFixture(t, ".wav", false)
	candidate := volatileWorkingCandidate(t, f)
	edit, err := candidate.Encode()
	if err != nil {
		t.Fatal(err)
	}
	storeVolatileAudio(t, f, edit)
	journal := f.doc.Metadata.Code
	initial := bytes.Clone(journal.Bytes())
	meta, err := asset.ParseSourceMeta(initial, f.doc.Metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	next := meta.Settings.Clone()
	next["loopStart"], next["loopEnd"] = aetext.Int(100), aetext.Int(20000)
	f.plugin.write(f.ed, meta.Settings, next)
	withLoop := bytes.Clone(journal.Bytes())
	if !journal.Undo() || !bytes.Equal(journal.Bytes(), initial) || !journal.CanRedo() {
		t.Fatal("cannot prepare combined audio redo")
	}
	raw, err := journal.EncodeVolatileJournal(context.Background(), f.meta)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := code.DecodeVolatileJournal(context.Background(), raw, f.meta)
	if err != nil || restored == journal {
		t.Fatal("cannot prepare detached recovery candidate", err)
	}
	// Simulate adoption only in this temporary fixture. No real recovery file is
	// created and no v2 sidecar is placed in the shared text document.
	f.doc.Metadata.Code = restored
	if !restored.VolatileDirty() || bytes.Contains(restored.Bytes(), []byte("audioEdit")) {
		t.Fatal("candidate did not separate the private edit from v1 metadata")
	}
	if err := f.ed.UndoAsset(f.doc); err != nil || restored.VolatileDirty() || !bytes.Equal(restored.Bytes(), initial) {
		t.Fatal("recovered Undo did not restore the edit state", err)
	}
	if err := f.ed.RedoAsset(f.doc); err != nil || !restored.VolatileDirty() || !bytes.Equal(restored.Bytes(), initial) {
		t.Fatal("recovered Redo did not restore the edit state", err)
	}
	if err := f.ed.RedoAsset(f.doc); err != nil || !bytes.Equal(restored.Bytes(), withLoop) {
		t.Fatal("recovered Redo lost the loop settings", err)
	}
	if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, false); err != nil {
		t.Fatal(err)
	}
	waitWorkingAudio(t, f)
	if f.module.PreviewClip() != f.id {
		notice, _ := f.ed.LastNotice()
		t.Fatal("recovered private preview did not start", notice)
	}
	out := make([]float32, 2048)
	f.module.Mixer().Mix(out)
	var peak float64
	for _, sample := range out {
		peak = max(peak, math.Abs(float64(sample)))
	}
	if peak < .035 || peak > .19 {
		t.Fatalf("recovered preview played the wrong PCM: peak=%v", peak)
	}
	if err := f.ed.Docs.SaveAll(); err == nil {
		t.Fatal("recovered volatile edit was published as ordinary metadata")
	}
	f.checkFiles(t, f.source, f.meta)
}
