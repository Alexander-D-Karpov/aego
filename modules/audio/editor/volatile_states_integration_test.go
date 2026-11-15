package editor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"aego/asset/audiosidecar"
	"aego/audio/edit"
	"aego/editor/core/code"
)

// Validate a detached journal without restoring a document or persisting EDL.
func TestVolatileAudioRecoveryValidatesEveryReachableState(t *testing.T) {
	for _, scenario := range []string{"valid", "wrong source in redo", "invalid redo", "invalid undo", "invalid historical metadata"} {
		t.Run(scenario, func(t *testing.T) {
			f := newWorkingAudioFixture(t, ".wav", false)
			edit := volatileWorkingCandidate(t, f)
			valid, err := edit.Encode()
			if err != nil {
				t.Fatal(err)
			}
			journal := f.doc.Metadata.Code
			if err := journal.AttachVolatile(); err != nil {
				t.Fatal(err)
			}
			replace := func(raw []byte) {
				t.Helper()
				if changed, err := journal.ReplaceVolatile(journal.Rev(), raw); err != nil || !changed {
					t.Fatal("cannot prepare structured history", err)
				}
			}
			switch scenario {
			case "wrong source in redo":
				replace(valid)
				other := edit.Clone()
				other.Source.SHA256 = strings.Repeat("f", 64)
				if other.Source.SHA256 == edit.Source.SHA256 {
					other.Source.SHA256 = strings.Repeat("e", 64)
				}
				bad, err := other.Encode()
				if err != nil {
					t.Fatal(err)
				}
				replace(bad)
				if !journal.Undo() {
					t.Fatal("cannot hide wrong-source state in redo")
				}
			case "invalid redo":
				replace(valid)
				replace([]byte("not an audio document"))
				if !journal.Undo() {
					t.Fatal("cannot hide invalid state in redo")
				}
			case "invalid undo":
				replace([]byte("not an audio document"))
				replace(valid)
			case "invalid historical metadata":
				replace(valid)
				journal.ReplaceSnapshot([]byte("not metadata"), false)
				journal.ReplaceSnapshot(f.meta, false)
			default:
				replace(valid)
			}
			raw, err := journal.EncodeVolatileJournal(context.Background(), f.meta)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := code.DecodeVolatileJournal(context.Background(), raw, f.meta)
			if err != nil {
				t.Fatal("generic codec refused a structurally valid journal", err)
			}
			before, err := restored.EncodeVolatileJournal(context.Background(), f.meta)
			if err != nil {
				t.Fatal(err)
			}
			invalid := errors.New("invalid audio recovery state")
			err = restored.ValidateVolatileStates(context.Background(), func(metadata, value []byte) error {
				_, present, err := audiosidecar.Decode(metadata, f.doc.Path)
				if err != nil || present {
					return invalid
				}
				if len(value) == 0 {
					return nil
				}
				candidate, err := audioedit.Decode(value)
				if err != nil || candidate.Source != edit.Source {
					return invalid
				}
				if _, err := audiosidecar.Encode(metadata, f.doc.Path, candidate); err != nil {
					return invalid
				}
				return nil
			})
			if scenario == "valid" && err != nil {
				t.Fatal("valid audio journal was rejected", err)
			}
			if scenario != "valid" && !errors.Is(err, invalid) {
				t.Fatalf("%s was not rejected: %v", scenario, err)
			}
			after, err := restored.EncodeVolatileJournal(context.Background(), f.meta)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("domain validation changed the detached journal", err)
			}
			f.checkFiles(t, f.source, f.meta)
		})
	}
}
