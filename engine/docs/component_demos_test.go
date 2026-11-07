//go:build !aego_runtime

package docs

import (
	"strings"
	"testing"
	"testing/fstest"

	"aego/engine/module"
)

func TestTrySidecarsDeduplicateOnlyWithinTheSameSourceAndPath(t *testing.T) {
	contract := []byte(`{"componentID":"engine.Test","documentID":"engine.Test"}`)
	src := module.DocSource{Owner: "test", FS: fstest.MapFS{
		"try/Test.demo.json":  {Data: contract},
		"try/Other.demo.json": {Data: contract},
	}}
	r := &Registry{}
	e := &Entry{ID: "engine.Test"}
	r.tryExtras(e, src, "try/Test.aescene")
	r.tryExtras(e, src, "try/./Test.aescene")
	if e.Demo == nil || len(r.problems) != 0 {
		t.Fatalf("the same page and auto-discovered scene should share one contract: %+v", r.problems)
	}
	r.tryExtras(e, src, "try/Other.aescene")
	if len(r.problems) != 1 || !strings.Contains(r.problems[0].Message, "duplicate component contract") {
		t.Fatalf("a second path was not rejected: %+v", r.problems)
	}
	r.problems = nil
	r.trySeen = make(map[string]bool) // Assemble starts a new source.
	r.tryExtras(e, src, "try/Test.aescene")
	if len(r.problems) != 1 || !strings.Contains(r.problems[0].Message, "duplicate component contract") {
		t.Fatalf("a second source was not rejected: %+v", r.problems)
	}
}
