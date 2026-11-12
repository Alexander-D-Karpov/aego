package audio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/asset/audiosidecar"
	audioimport "aego/asset/importers/audio"
	"aego/asset/pipeline"
	"aego/audio/codec/wav"
	"aego/audio/edit"
	"aego/core/log"
	"aego/project"
)

func TestUnsupportedEditedSidecarCannotPublishThroughCurrentImporter(t *testing.T) {
	for _, initial := range []bool{false, true} {
		name := "guarded-publication"
		if initial {
			name = "fresh-project-scan"
		}
		t.Run(name, func(t *testing.T) {
			pj, err := project.Create(t.TempDir(), "UnsupportedAudioEdit", false)
			if err != nil {
				t.Fatal(err)
			}
			const rel = "assets/tone.wav"
			file := filepath.Join(pj.Root, filepath.FromSlash(rel))
			source := wav.Encode([]float32{.1, .2, .3, .4}, 8000, 1, wav.EncodeOptions{Float: true})
			digest := sha256.Sum256(source)
			imp := audioimport.New()
			meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
			before := asset.MarshalSourceMeta(meta)
			doc, err := audioedit.New(audioedit.Source{AssetID: meta.ID.String(), Path: rel, SHA256: fmt.Sprintf("%x", digest),
				Codec: "wav", SampleRate: 8000, Channels: 1, Frames: 4})
			if err != nil {
				t.Fatal(err)
			}
			doc.Operations = []audioedit.Operation{{Kind: audioedit.Gain, Region: audioedit.Range{Start: 0, End: 4}, Value: .5}}
			after, err := audiosidecar.Encode(before, rel, doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, source, 0o644); err != nil {
				t.Fatal(err)
			}
			onDisk := before
			if initial {
				onDisk = after
			}
			if err := os.WriteFile(file+asset.MetaSuffix, onDisk, 0o644); err != nil {
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
			defer pl.Close()
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			batch, err := pl.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("initial scan/import: %+v %v", batch, err)
			}
			if initial {
				if len(batch.Imported) != 0 {
					t.Fatal("fresh scan imported an unsupported edited source")
				}
				if _, ok := pl.Lookup(rel); ok || pl.VerifyCurrent(meta.ID) == nil {
					t.Fatal("fresh scan exposed an unsupported edit as a current asset")
				}
			} else {
				if len(batch.Imported) != 1 {
					t.Fatal("fixture did not import its original source")
				}
				guard, err := pl.FingerprintSource(context.Background(), rel, pipeline.MaxSourceFingerprintBytes)
				if err != nil {
					t.Fatal(err)
				}
				edits := []pipeline.SourceEdit{{Path: rel, ReadOnlyGuard: true, Fingerprint: &guard},
					{Path: rel + asset.MetaSuffix, Before: before, After: after}}
				if batch, err := pl.ApplySources(edits, false); err == nil {
					t.Fatalf("published an edit without importer support: %+v", batch)
				}
				if status, err := pipeline.SourceRecoveryStatus(pj); err != nil || status != nil {
					t.Fatalf("fully rolled-back candidate left recovery: %+v %v", status, err)
				}
			}
			if data, err := os.ReadFile(file); err != nil || sha256.Sum256(data) != digest {
				t.Fatal("safety check changed source bytes", err)
			}
			if data, err := os.ReadFile(file + asset.MetaSuffix); err != nil || !bytes.Equal(data, onDisk) {
				t.Fatal("safety check changed the metadata preimage", err)
			}
		})
	}
}
