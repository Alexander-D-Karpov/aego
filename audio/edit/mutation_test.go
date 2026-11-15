package audioedit

import (
	"errors"
	"reflect"
	"testing"
)

func TestPrepareMutationOwnsInputsAndCandidate(t *testing.T) {
	current, err := New(testSource(48000))
	if err != nil {
		t.Fatal(err)
	}
	current.Revision = 41
	current.Loop = &Range{Start: 100, End: 1000}
	current.Markers = []Marker{{ID: "start", Frame: 200}}
	current.Operations = []Operation{{Kind: Gain, Region: Range{Start: 100, End: 1000}, Value: .5}}
	before := current.Clone()
	var retained *Document
	candidate, changed, err := PrepareMutation(current, 41, func(next *Document) error {
		retained = next
		next.Loop.End = 900
		next.Markers[0].Label = "edited"
		AddOperation(next, Operation{Kind: Reverse, Region: Range{Start: 100, End: 900}})
		next.Revision = 900 // caller does not control generation
		return nil
	})
	if err != nil || !changed || candidate.Revision != 42 {
		t.Fatalf("candidate: %+v changed=%v err=%v", candidate, changed, err)
	}
	if !reflect.DeepEqual(current, before) || candidate.Loop.End != 900 || len(candidate.Operations) != 2 || candidate.Markers[0].Label != "edited" {
		t.Fatal("preparation changed the original or lost the candidate")
	}
	retained.Loop.End = 300
	retained.Markers[0].Label = "retained"
	retained.Operations[0].Value = 3
	if candidate.Loop.End != 900 || candidate.Markers[0].Label != "edited" || candidate.Operations[0].Value != .5 {
		t.Fatal("retained callback argument mutated the candidate")
	}
	candidate.Markers[0].Label = "candidate"
	if !reflect.DeepEqual(current, before) {
		t.Fatal("candidate retained an alias to the original")
	}
}

func TestPrepareMutationRejectsStaleAndInvalidCandidates(t *testing.T) {
	current, _ := New(testSource(100))
	current.Revision = 7
	current.Markers = []Marker{{ID: "one", Frame: 10}}
	before := current.Clone()
	callbackErr := errors.New("mutation failed")
	for _, tc := range []struct {
		name     string
		expected uint64
		fn       func(*Document) error
	}{
		{"nil", 7, nil},
		{"stale", 6, func(*Document) error { t.Fatal("stale callback was invoked"); return nil }},
		{"callback error", 7, func(d *Document) error { d.Markers[0].Label = "discarded"; return callbackErr }},
		{"source identity", 7, func(d *Document) error { d.Source.Path = "assets/other.ogg"; return nil }},
		{"invalid trim", 7, func(d *Document) error { d.Trim.End = 101; return nil }},
		{"invalid operation", 7, func(d *Document) error {
			AddOperation(d, Operation{Kind: Reverse, Region: Range{Start: 99, End: 101}})
			return nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate, changed, err := PrepareMutation(current, tc.expected, tc.fn)
			if err == nil || changed || !reflect.DeepEqual(candidate, Document{}) || !reflect.DeepEqual(current, before) {
				t.Fatalf("rejected mutation returned %+v, %v, %v or changed the original", candidate, changed, err)
			}
			if tc.name == "callback error" && !errors.Is(err, callbackErr) {
				t.Fatalf("callback error was lost: %v", err)
			}
		})
	}
}

func TestPrepareMutationNoopAndRevisionExhaustion(t *testing.T) {
	current, _ := New(testSource(100))
	current.Revision = ^uint64(0)
	current.Loop = &Range{Start: 1, End: 10}
	candidate, changed, err := PrepareMutation(current, current.Revision, func(d *Document) error {
		d.Revision = 3
		return nil
	})
	if err != nil || changed || candidate.Revision != current.Revision || !reflect.DeepEqual(candidate, current) {
		t.Fatalf("no-op changed the generation: %+v, %v, %v", candidate, changed, err)
	}
	candidate.Loop.End = 9
	if current.Loop.End != 10 {
		t.Fatal("no-op candidate retained an alias to the original")
	}
	candidate, changed, err = PrepareMutation(current, current.Revision, func(d *Document) error { d.BPM = 120; return nil })
	if err == nil || changed || !reflect.DeepEqual(candidate, Document{}) || current.BPM != 0 {
		t.Fatal("revision exhaustion was not rejected without changing the original")
	}
}

func TestPrepareMutationRejectsInvalidInitialDocument(t *testing.T) {
	for _, makeInvalid := range []func(*Document){
		func(d *Document) { d.Trim.End = 101 },
		func(d *Document) { d.Operations = make([]Operation, maxDocumentOperations+1) },
	} {
		current, _ := New(testSource(100))
		makeInvalid(&current)
		called := false
		_, changed, err := PrepareMutation(current, 0, func(*Document) error { called = true; return nil })
		if err == nil || changed || called {
			t.Fatal("invalid current document reached the callback")
		}
	}
}
