package engine

import (
	"strings"
	"testing"

	"aego/core/errs"
)

type recorder struct {
	id  string
	log *[]string
}

func (r recorder) Run(*Context) {
	*r.log = append(*r.log, r.id)
}

func TestTopologicalOrder(t *testing.T) {
	var seen []string
	r := newRegistry()
	s := r.Systems()
	for _, d := range []SystemDesc{
		{ID: "c", Phase: PhaseUpdate, After: []string{"b"}, System: recorder{id: "c", log: &seen}},
		{ID: "a", Phase: PhaseUpdate, Before: []string{"b"}, System: recorder{id: "a", log: &seen}},
		{ID: "b", Phase: PhaseUpdate, System: recorder{id: "b", log: &seen}},
		{ID: "r", Phase: PhaseRender, System: recorder{id: "r", log: &seen}},
	} {
		if err := s.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.Order(PhaseUpdate), ","); got != "a,b,c" {
		t.Fatalf("update order = %s", got)
	}
	if got := strings.Join(s.Order(PhaseRender), ","); got != "r" {
		t.Fatalf("render order = %s", got)
	}
}

func TestRegistryErrors(t *testing.T) {
	cases := []struct {
		name  string
		descs []SystemDesc
		code  errs.Code
	}{
		{"cycle", []SystemDesc{
			{ID: "a", Phase: PhaseUpdate, After: []string{"b"}, System: recorder{}},
			{ID: "b", Phase: PhaseUpdate, After: []string{"a"}, System: recorder{}},
		}, errs.EngineSystemCycle},
		{"self", []SystemDesc{
			{ID: "a", Phase: PhaseUpdate, After: []string{"a"}, System: recorder{}},
		}, errs.EngineSystemCycle},
		{"unknown", []SystemDesc{
			{ID: "a", Phase: PhaseUpdate, Before: []string{"ghost"}, System: recorder{}},
		}, errs.EngineSystemDep},
		{"cross phase", []SystemDesc{
			{ID: "a", Phase: PhaseUpdate, Before: []string{"b"}, System: recorder{}},
			{ID: "b", Phase: PhaseRender, System: recorder{}},
		}, errs.EngineSystemPhase},
	}
	for _, c := range cases {
		r := newRegistry()
		for _, d := range c.descs {
			if err := r.Systems().Register(d); err != nil {
				t.Fatalf("%s: register: %v", c.name, err)
			}
		}
		err := r.Freeze()
		if !errs.Has(err, c.code) {
			t.Fatalf("%s: got %v, want %s", c.name, err, c.code)
		}
	}
}

func TestRegisterValidation(t *testing.T) {
	r := newRegistry()
	s := r.Systems()
	if err := s.Register(SystemDesc{ID: "", Phase: PhaseUpdate, System: recorder{}}); !errs.Has(err, errs.EngineSystemBad) {
		t.Fatalf("empty id: %v", err)
	}
	if err := s.Register(SystemDesc{ID: "a", Phase: PhaseUpdate}); !errs.Has(err, errs.EngineSystemBad) {
		t.Fatalf("nil system: %v", err)
	}
	if err := s.Register(SystemDesc{ID: "a", Phase: Phase(200), System: recorder{}}); !errs.Has(err, errs.EngineSystemPhase) {
		t.Fatalf("bad phase: %v", err)
	}
	if err := s.Register(SystemDesc{ID: "a", Phase: PhaseUpdate, System: recorder{}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(SystemDesc{ID: "a", Phase: PhaseInput, System: recorder{}}); !errs.Has(err, errs.EngineSystemDup) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(SystemDesc{ID: "b", Phase: PhaseUpdate, System: recorder{}}); !errs.Has(err, errs.EngineFrozen) {
		t.Fatalf("frozen: %v", err)
	}
}
