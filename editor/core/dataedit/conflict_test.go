package dataedit

import (
	"bytes"
	"os"
	"testing"

	"aego/asset/pipeline"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/project"
)

func TestStructuredAssetSaveRequiresExplicitExternalConflictResolution(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Data", false)
	if err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, pipeline.NewImporterRegistry(), log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	ed := &context.Editor{Pipeline: pl}
	s := New(ed)
	const path = "assets/settings.aedata"
	expected, external := []byte("original"), []byte("external edit")
	if err := os.WriteFile(pl.Abs(path), external, 0644); err != nil {
		t.Fatal(err)
	}
	reloaded := 0
	keep := func(raw []byte) { expected = raw }
	reload := func() { reloaded++ }
	if err := s.guardSave(path, expected, keep, reload); err == nil {
		t.Fatal("structured save silently overwrote an external edit")
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Then == nil {
		t.Fatal("source conflict has no explicit resolution")
	}
	questions[0].Then(1) // Keep mine acknowledges the current disk baseline only.
	if !bytes.Equal(expected, external) || reloaded != 0 {
		t.Fatal("Keep mine discarded local edits instead of updating the baseline")
	}
	if actual, _ := os.ReadFile(pl.Abs(path)); !bytes.Equal(actual, external) {
		t.Fatal("conflict resolution silently saved a document")
	}
	if err := s.guardSave(path, expected, keep, reload); err != nil {
		t.Fatal("acknowledged baseline could not be saved", err)
	}
	if err := os.WriteFile(pl.Abs(path), []byte("newer external edit"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.guardSave(path, expected, keep, reload); err == nil {
		t.Fatal("a second disk change bypassed optimistic concurrency")
	}
	ed.TakeQuestions()[0].Then(0)
	if reloaded != 1 {
		t.Fatal("Reload did not route to the existing data model")
	}
}
