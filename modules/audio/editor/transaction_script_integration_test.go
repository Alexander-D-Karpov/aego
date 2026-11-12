package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	audioimport "aego/asset/importers/audio"
	"aego/asset/pipeline"
	"aego/audio/codec/wav"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/format/aetext"
	"aego/project"
)

type audioTransactionFixture struct {
	project *project.Project
	pipe    *pipeline.Pipeline
	ed      *context.Editor
	doc     *document.AssetDocument
	source  []byte
	before  []byte
	tx      *document.DirtyScriptTransaction
}

func newAudioTransactionFixture(t *testing.T) audioTransactionFixture {
	t.Helper()
	pj, err := project.Create(t.TempDir(), "AudioTransaction", false)
	if err != nil {
		t.Fatal(err)
	}
	const path = "assets/tone.wav"
	source := wav.Encode([]float32{.1, -.2, .3, -.4, .5, -.6, .7, -.8}, 8000, 1, wav.EncodeOptions{Float: true})
	if err := os.WriteFile(filepath.Join(pj.Root, path), source, 0o644); err != nil {
		t.Fatal(err)
	}
	imp := audioimport.New()
	meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
	meta.Settings["extension_note"] = aetext.String("preserve")
	before := asset.MarshalSourceMeta(meta)
	if err := os.WriteFile(filepath.Join(pj.Root, path+".meta"), before, 0o644); err != nil {
		t.Fatal(err)
	}
	registry := pipeline.NewImporterRegistry()
	if err := registry.Register(imp); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(pj, registry, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("audio fixture import: batch=%+v error=%v", batch, err)
	}
	ed := &context.Editor{Pipeline: pl, Docs: document.NewManager(document.Options{Root: pj.Root})}
	if err := ed.Docs.Handlers().Register(document.Handler{ID: "audio.clip", Kinds: []asset.ArtifactKind{asset.ArtAudio}, Capabilities: document.View | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	doc, err := ed.DocumentForAsset(meta.ID)
	if err != nil || doc.Metadata == nil {
		t.Fatal("cannot open audio metadata", err)
	}
	p := &plugin{ed: ed, clip: clipEdit{id: meta.ID, drag: -1}}
	next := meta.Settings.Clone()
	next["gain"] = aetext.Float64(2)
	p.write(ed, meta.Settings, next)
	if !doc.Dirty() || !doc.CanUndo() {
		t.Fatal("fixture did not create a shared dirty metadata edit")
	}
	tx, err := ed.Docs.PrepareDirtyScriptTransaction(doc.Metadata, before)
	if err != nil {
		t.Fatal(err)
	}
	return audioTransactionFixture{project: pj, pipe: pl, ed: ed, doc: doc, source: source, before: before, tx: tx}
}

func TestDirtyAudioMetadataAdoptsActualGuardedPipelineTransaction(t *testing.T) {
	f := newAudioTransactionFixture(t)
	const path = "assets/tone.wav"
	before, after, err := f.tx.Images()
	if err != nil {
		t.Fatal(err)
	}
	edits := []pipeline.SourceEdit{
		{Path: path, Before: bytes.Clone(f.source), After: bytes.Clone(f.source), ReadOnlyGuard: true},
		{Path: path + ".meta", Before: before, After: after},
	}
	batch, err := f.pipe.ApplySources(edits, false)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("guarded pipeline transaction: batch=%+v error=%v", batch, err)
	}
	if !bytes.Equal(edits[1].After, after) {
		t.Fatal("pipeline unexpectedly canonicalized the prepared metadata image")
	}
	if err := f.ed.Docs.AcceptDirtyScriptTransaction(f.tx); err != nil {
		t.Fatal(err)
	}
	if f.doc.Dirty() {
		t.Fatal("adopted audio metadata remains dirty")
	}
	entry, ok := f.pipe.Entry(f.doc.Asset)
	if !ok {
		t.Fatal("transaction lost the indexed audio")
	}
	artifact, ok := entry.Artifact(asset.ArtAudio)
	if !ok {
		t.Fatal("transaction lost the audio artifact")
	}
	raw, err := f.pipe.Artifacts().ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := asset.DecodeAudio(raw)
	if err != nil || decoded.Gain != 2 || !bytes.Equal(decoded.Data, f.source) {
		t.Fatalf("published artifact does not match the metadata edit: %+v %v", decoded, err)
	}
	if err := f.ed.UndoAsset(f.doc); err != nil || !f.doc.Dirty() {
		t.Fatal("Undo after transaction did not leave a dirty working document", err)
	}
	if err := f.ed.RedoAsset(f.doc); err != nil || f.doc.Dirty() {
		t.Fatal("Redo did not restore the saved checkpoint", err)
	}
	if got, err := os.ReadFile(filepath.Join(f.project.Root, path)); err != nil || !bytes.Equal(got, f.source) {
		t.Fatal("guarded transaction changed encoded audio", err)
	}
	if got, err := os.ReadFile(filepath.Join(f.project.Root, path+".meta")); err != nil || !bytes.Equal(got, after) {
		t.Fatal("Undo/Redo changed the committed metadata on disk", err)
	}
}

func TestDirtyAudioMetadataRejectsChangedGuardedSource(t *testing.T) {
	f := newAudioTransactionFixture(t)
	const path = "assets/tone.wav"
	before, after, err := f.tx.Images()
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Clone(f.source)
	changed[len(changed)-1] ^= 1 // Same size, different owner bytes.
	if err := os.WriteFile(filepath.Join(f.project.Root, path), changed, 0o644); err != nil {
		t.Fatal(err)
	}
	edits := []pipeline.SourceEdit{
		{Path: path, Before: bytes.Clone(f.source), After: bytes.Clone(f.source), ReadOnlyGuard: true},
		{Path: path + ".meta", Before: before, After: after},
	}
	if batch, err := f.pipe.ApplySources(edits, false); err == nil {
		t.Fatalf("accepted an externally changed source: %+v", batch)
	}
	if err := f.ed.Docs.AcceptDirtyScriptTransaction(f.tx); err == nil {
		t.Fatal("adopted metadata that the transaction did not publish")
	}
	if !f.doc.Dirty() || !f.doc.CanUndo() || !bytes.Equal(f.doc.Metadata.Code.Bytes(), after) {
		t.Fatal("failed transaction lost the shared dirty metadata or history")
	}
	if got, err := os.ReadFile(filepath.Join(f.project.Root, path)); err != nil || !bytes.Equal(got, changed) {
		t.Fatal("failed transaction changed the external source", err)
	}
	if got, err := os.ReadFile(filepath.Join(f.project.Root, path+".meta")); err != nil || !bytes.Equal(got, f.before) {
		t.Fatal("failed transaction published metadata", err)
	}
}

func TestDirtyAudioMetadataKeepsWorkWhenAcceptanceFailsAfterPublication(t *testing.T) {
	for _, reason := range []string{"new owner edit", "external metadata"} {
		t.Run(reason, func(t *testing.T) {
			f := newAudioTransactionFixture(t)
			const path = "assets/tone.wav"
			before, after, err := f.tx.Images()
			if err != nil {
				t.Fatal(err)
			}
			edits := []pipeline.SourceEdit{
				{Path: path, Before: bytes.Clone(f.source), After: bytes.Clone(f.source), ReadOnlyGuard: true},
				{Path: path + ".meta", Before: before, After: after},
			}
			batch, err := f.pipe.ApplySources(edits, false)
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("guarded pipeline transaction: batch=%+v error=%v", batch, err)
			}
			switch reason {
			case "new owner edit":
				meta, err := asset.ParseSourceMeta(f.doc.Metadata.Code.Bytes(), f.doc.Metadata.Path)
				if err != nil {
					t.Fatal(err)
				}
				meta.Settings["gain"] = aetext.Float64(3)
				f.doc.Metadata.Code.ReplaceSnapshot(asset.MarshalSourceMeta(meta), false)
			case "external metadata":
				external := bytes.Repeat([]byte{'X'}, len(after))
				if err := os.WriteFile(filepath.Join(f.project.Root, path+".meta"), external, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			working := bytes.Clone(f.doc.Metadata.Code.Bytes())
			onDisk, err := os.ReadFile(filepath.Join(f.project.Root, path+".meta"))
			if err != nil {
				t.Fatal(err)
			}
			if err := f.ed.Docs.AcceptDirtyScriptTransaction(f.tx); err == nil {
				t.Fatal("accepted a changed owner document or disk postimage")
			}
			if !f.doc.Dirty() || !f.doc.CanUndo() || f.doc.Metadata.SavedEdits != 0 || !bytes.Equal(f.doc.Metadata.Code.Bytes(), working) {
				t.Fatal("failed acceptance discarded the owner's dirty journal")
			}
			// A later ordinary save must not overwrite the published or external
			// metadata against the old preimage; it reports the conflict instead.
			if err := f.ed.Docs.SaveScript(f.doc.Metadata); err == nil || !f.doc.Metadata.Conflict {
				t.Fatal("ordinary Save ignored the unadopted disk generation")
			}
			if got, err := os.ReadFile(filepath.Join(f.project.Root, path+".meta")); err != nil || !bytes.Equal(got, onDisk) {
				t.Fatal("failed acceptance or Save overwrote metadata", err)
			}
			if got, err := os.ReadFile(filepath.Join(f.project.Root, path)); err != nil || !bytes.Equal(got, f.source) {
				t.Fatal("failed acceptance changed encoded audio", err)
			}
		})
	}
}
