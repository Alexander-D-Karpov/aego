package shader_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"aego/asset"
	shimp "aego/asset/importers/shader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
)

// Emit real pipeline watcher events without depending on a host filesystem's
// polling interval. The pipeline still performs scan/cache/import/notification.
type sourceWatcher struct{ emit func([]pipeline.Change) }

func (w *sourceWatcher) Start(_ string, _ []string, _ time.Duration, emit func([]pipeline.Change)) (func(), error) {
	w.emit = emit
	return func() {}, nil
}

func TestMissingTransitiveIncludeCreationRemovalAndRepair(t *testing.T) {
	p, err := project.Create(t.TempDir(), "IncludeWatch", false)
	if err != nil {
		t.Fatal(err)
	}
	write(t, p.Root, "assets/source.aeshader", "#pragma aego abi 1\n#include \"parent.glsl\"\nvoid fragment(){aego_out=vec4(child());}\n")
	write(t, p.Root, "assets/shaders/include/parent.glsl", "#include \"child.glsl\"\n")
	reg := pipeline.NewImporterRegistry()
	i := shimp.New()
	i.Require, i.Validator = false, "aego-test-no-validator"
	for _, imp := range []pipeline.Importer{i, shimp.NewInclude()} {
		if err := reg.Register(imp); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	id, _ := pl.Lookup("assets/source.aeshader")
	if batch, err := pl.ImportAll(); err != nil || batch.Failed[id] == nil {
		t.Fatalf("missing initial include did not fail: %v %+v", err, batch)
	}
	if ds, _ := i.Diagnostics(); len(ds) == 0 || ds[0].AssetID != id.String() {
		t.Fatalf("cache-salt diagnostic lost shader identity: %+v", ds)
	}
	w := &sourceWatcher{}
	pl.SetWatcher(w)
	updates := make(chan *pipeline.Batch, 8)
	stop, err := pl.Watch(time.Millisecond, func(b *pipeline.Batch) { updates <- b })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	change := func(path string, kind pipeline.ChangeKind) *pipeline.Batch {
		t.Helper()
		w.emit([]pipeline.Change{{Path: path, Kind: kind}})
		select {
		case b := <-updates:
			return b
		case <-time.After(3 * time.Second):
			t.Fatal("pipeline did not publish an include change")
			return nil
		}
	}
	shaderBytes := func() []byte {
		t.Helper()
		e, ok := pl.Index().Lookup(id)
		if !ok {
			t.Fatal("shader entry disappeared")
		}
		a, ok := e.Artifact(asset.ArtShader)
		if !ok {
			t.Fatal("shader has no prepared artifact")
		}
		data, err := pl.Artifacts().ReadFile(a.Path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	child := "assets/shaders/include/child.glsl"
	write(t, p.Root, child, "float child(){return 1.;}\n")
	first := change(child, pipeline.ChangeWrite)
	if len(first.Failed) != 0 || !slices.ContainsFunc(first.Imported, func(e asset.Entry) bool { return e.ID == id }) {
		t.Fatalf("creating an include with no previous dependency failed to repair shader: %+v", first)
	}
	childID, _ := pl.Lookup(child)
	if !slices.Contains(pl.Dependents(childID), id) {
		t.Fatal("successful import omitted a transitive dependency")
	}
	before := shaderBytes()
	if err := os.Remove(filepath.Join(p.Root, child)); err != nil {
		t.Fatal(err)
	}
	removed := change(child, pipeline.ChangeRemove)
	if removed.Failed[id] == nil || !slices.Contains(removed.Removed, childID) {
		t.Fatalf("include removal failed to invalidate its consumer: %+v", removed)
	}
	if after := shaderBytes(); !bytes.Equal(before, after) {
		t.Fatal("include removal replaced last-good shader bytes")
	}
	write(t, p.Root, child, "float child(){return 2.;}\n")
	repaired := change(child, pipeline.ChangeWrite)
	if len(repaired.Failed) != 0 {
		t.Fatalf("include repair failed: %+v", repaired)
	}
	a, err := asset.DecodeShader(shaderBytes())
	if err != nil || !strings.Contains(a.Variants[0].Sources[1], "return 2.") {
		t.Fatalf("recreated include did not reach shader artifact: %v", err)
	}
	if ds, _ := i.Diagnostics(); len(ds) != 0 {
		t.Fatalf("repaired include kept stale diagnostics: %+v", ds)
	}
	if got, _ := pl.Lookup(child); got != childID && childID != (ids.AssetID{}) {
		t.Fatal("recreation with retained .meta changed include identity")
	}
}
