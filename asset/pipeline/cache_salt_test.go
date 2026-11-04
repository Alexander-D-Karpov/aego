package pipeline

import (
	"errors"
	"testing"

	"aego/core/ids"
)

type saltedImporter struct {
	fakeImporter
	salt []byte
	err  error
}

func (s *saltedImporter) CacheSalt(*ImportContext) ([]byte, error) {
	return s.salt, s.err
}

func TestSupplementalInputsInvalidateBeforeCacheLookup(t *testing.T) {
	p := newProject(t)
	write(t, p, "assets/a.txt", "same source")
	imp := &saltedImporter{fakeImporter: fakeImporter{version: 1}, salt: []byte("include v1")}
	pl := openPipeline(t, p, imp)
	pl.Scan()
	pl.ImportAll()
	pl.ImportAll()
	if imp.calls["assets/a.txt"] != 1 {
		t.Fatal("unchanged salt bypassed cache")
	}
	imp.salt = []byte("include v2")
	pl.ImportAll()
	if imp.calls["assets/a.txt"] != 2 {
		t.Fatal("include change reused stale artifact")
	}
	imp.err = errors.New("missing transitive include")
	batch, err := pl.ImportAll()
	if err != nil || len(batch.Failed) != 1 {
		t.Fatalf("invalid source silently reused last-good: %v %+v", err, batch)
	}
	imp.err = nil
	pl.ImportAll()
	for _, info := range pl.Assets() {
		if info.Error != "" {
			t.Fatal("a repaired source retained its import error")
		}
	}
}

func TestPreviewUsesRegisteredImporterWithoutWriting(t *testing.T) {
	p := newProject(t)
	imp := &fakeImporter{version: 1}
	pl := openPipeline(t, p, imp)
	res, err := pl.Preview("assets/new.txt", []byte("unsaved buffer"), nil)
	if err != nil || string(res.Artifacts[0].Data) != "unsaved buffer" {
		t.Fatalf("preview: %+v %v", res, err)
	}
	if pl.Index().Len() != 0 {
		t.Fatal("preview changed the index")
	}
	if id, exists := pl.Lookup("assets/new.txt"); exists || id != (ids.AssetID{}) {
		t.Fatal("preview created a source identity")
	}
	if _, err := pl.Preview("../outside.txt", []byte("x"), nil); err == nil {
		t.Fatal("preview accepted a path outside the project")
	}
}
