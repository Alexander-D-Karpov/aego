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
	e := &Entry{ID: "engine.Test", Component: &ComponentInfo{}}
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

func TestGuideCanReuseAComponentTryWithoutClaimingItsContract(t *testing.T) {
	src := module.DocSource{Owner: "test", FS: fstest.MapFS{
		"try/Test.demo.json": {Data: []byte(`{"componentID":"engine.Test","documentID":"engine.Test"}`)},
		"try/Test.aes":       {Data: []byte("package demo")},
	}}
	r := &Registry{}
	guide := &Entry{ID: "guide", Kind: Page}
	component := &Entry{ID: "engine.Test", Kind: Type, Component: &ComponentInfo{}}
	r.tryExtras(guide, src, "try/Test.aescene")
	r.tryExtras(component, src, "try/Test.aescene")
	if len(r.problems) != 0 || guide.Demo != nil || component.Demo == nil {
		t.Fatalf("shared Try did not keep its component owner: guide=%+v component=%+v problems=%+v", guide.Demo, component.Demo, r.problems)
	}
	if guide.TryScript != "package demo" || component.TryScript != guide.TryScript {
		t.Fatal("ignoring the guide's component sidecar also lost the shared Try script")
	}
}

func TestComponentContractStillRejectsWrongOwner(t *testing.T) {
	src := module.DocSource{Owner: "test", FS: fstest.MapFS{
		"try/Test.demo.json": {Data: []byte(`{"componentID":"engine.Other","documentID":"engine.Other"}`)},
	}}
	r := &Registry{}
	e := &Entry{ID: "engine.Test", Component: &ComponentInfo{}}
	r.tryExtras(e, src, "try/Test.aescene")
	if e.Demo != nil || len(r.problems) != 1 || !strings.Contains(r.problems[0].Message, "identity does not match") {
		t.Fatalf("wrong component owner was not rejected: demo=%+v problems=%+v", e.Demo, r.problems)
	}
}
