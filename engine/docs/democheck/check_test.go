package democheck

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func fixture() (Component, Contract) {
	component := Component{ID: "test.Counter", DocumentID: "test.Counter", Scene: "test.Counter/try", Identity: "exact-runtime-payload", Fields: []string{"test.Counter.Value"}}
	contract := Contract{Version: 1, ComponentID: component.ID, DemoID: "counter-increment", DocumentID: component.ID, Scene: component.Scene, Identity: component.Identity,
		Dependencies: []string{"counter-script"}, EditableFields: component.Fields,
		Actions: []Action{{ID: "increment", Field: component.Fields[0], Value: json.RawMessage("2"), Expected: []Expectation{{Observable: "count", Equals: json.RawMessage("2"), Changes: true}}}}, Reset: "reset-counter",
		Languages:    map[string]Text{"en": {"Counter", "Changes its counter", "Press increment"}, "ru": {"Счётчик", "Изменяет счётчик", "Нажмите «Увеличить»"}},
		Environments: map[string]Environment{"native": {Mode: "runtime"}, "chromium": {Mode: "runtime"}}}
	return component, contract
}

type fakeRunner struct {
	fault  string
	closed int
}

func (r *fakeRunner) Open(_ context.Context, c Contract, env string) (Instance, error) {
	return &fakeInstance{r: r, c: c, env: env}, nil
}

type fakeInstance struct {
	r       *fakeRunner
	c       Contract
	env     string
	value   int
	actions int
}

func (p *fakeInstance) Observe(context.Context) (Observation, error) {
	o := Observation{Identity: p.c.Identity, Components: []string{p.c.ComponentID}, Dependencies: p.c.Dependencies, RuntimeExecuting: p.c.Environments[p.env].Mode == "runtime", Observables: map[string]json.RawMessage{}}
	b, _ := json.Marshal(p.value)
	o.Observables["count"] = b
	switch p.r.fault {
	case "identity":
		o.Identity = "old-build"
	case "component":
		o.Components = nil
	case "dependency":
		o.Dependencies = nil
	case "fake-runtime":
		o.RuntimeExecuting = true
	case "stopped":
		o.RuntimeExecuting = false
	case "no-observable":
		o.Observables = nil
	}
	return o, nil
}
func (p *fakeInstance) Action(context.Context, Action) error {
	p.actions++
	if p.r.fault != "no-op" {
		p.value = 2
	}
	return nil
}
func (p *fakeInstance) Reset(context.Context, string) error {
	if p.r.fault != "reset" || p.actions == 0 {
		p.value = 0
	}
	return nil
}
func (p *fakeInstance) Close() error {
	p.r.closed++
	if p.r.fault == "close" {
		return errors.New("close failed")
	}
	return nil
}
func TestBehavioralCoverageRequiresActionsAndReset(t *testing.T) {
	c, d := fixture()
	runner := &fakeRunner{}
	report := Check(context.Background(), []Component{c}, []Contract{d}, runner)
	if !report.Passed() || report.Error() != nil || runner.closed != 2 {
		t.Fatalf("%+v closed=%d", report, runner.closed)
	}
	for _, fault := range []string{"identity", "component", "dependency", "no-op", "reset", "stopped", "no-observable", "close"} {
		t.Run(fault, func(t *testing.T) {
			runner := &fakeRunner{fault: fault}
			r := Check(context.Background(), []Component{c}, []Contract{d}, runner)
			if r.Passed() || r.Error() == nil || runner.closed != 2 {
				t.Fatalf("%+v, closed=%d", r, runner.closed)
			}
		})
	}
}
func TestCoverageCannotPassWithOnlyFilesOrNoComponents(t *testing.T) {
	c, d := fixture()
	for _, tc := range []struct {
		name       string
		components []Component
		contracts  []Contract
		runner     Runner
	}{
		{"no runner", []Component{c}, []Contract{d}, nil}, {"no contract", []Component{c}, nil, &fakeRunner{}},
		{"empty registry", nil, nil, &fakeRunner{}}, {"duplicate contract", []Component{c}, []Contract{d, d}, &fakeRunner{}},
		{"duplicate component", []Component{c, c}, []Contract{d}, &fakeRunner{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Check(context.Background(), tc.components, tc.contracts, tc.runner)
			if r.Passed() {
				t.Fatalf("false coverage: %+v", r)
			}
		})
	}
}
func TestCoverageRejectsStaleSchemaAndNoChangeAssertions(t *testing.T) {
	for _, fault := range []string{"field", "translation", "no-change", "unused-field", "identity", "only-native"} {
		t.Run(fault, func(t *testing.T) {
			c, d := fixture()
			switch fault {
			case "field":
				c.Fields = nil
			case "translation":
				delete(d.Languages, "ru")
			case "no-change":
				d.Actions[0].Expected[0].Changes = false
			case "unused-field":
				c.Fields = append(c.Fields, "test.Counter.Extra")
				d.EditableFields = append(d.EditableFields, "test.Counter.Extra")
			case "identity":
				d.Identity = "old"
			case "only-native":
				delete(d.Environments, "chromium")
			}
			if r := Check(context.Background(), []Component{c}, []Contract{d}, &fakeRunner{}); r.Passed() {
				t.Fatalf("false coverage: %+v", r)
			}
		})
	}
}
func TestAuthoringOnlyScenarioIsExplicitAndDoesNotExecuteRuntime(t *testing.T) {
	c, d := fixture()
	d.Environments["chromium"] = Environment{Mode: "authoring", Reason: map[string]string{"en": "Native-only output", "ru": "Вывод доступен только в нативном приложении"}}
	if r := Check(context.Background(), []Component{c}, []Contract{d}, &fakeRunner{}); !r.Passed() {
		t.Fatalf("%+v", r)
	}
	r := Check(context.Background(), []Component{c}, []Contract{d}, &fakeRunner{fault: "fake-runtime"})
	if r.Passed() || !strings.Contains(strings.Join(r.Results[0].Errors, " "), "falsely") {
		t.Fatalf("%+v", r)
	}
}
func TestJSONComparisonDoesNotRoundIntegerEvidence(t *testing.T) {
	if equalJSON(json.RawMessage("9007199254740992"), json.RawMessage("9007199254740993")) {
		t.Fatal("rounded integer equality")
	}
	if !equalJSON(json.RawMessage(`{"b":2,"a":1}`), json.RawMessage(`{"a":1,"b":2}`)) {
		t.Fatal("object order changes semantics")
	}
}
func TestCancelledCoverageDoesNotRunProbe(t *testing.T) {
	c, d := fixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeRunner{}
	if r := Check(ctx, []Component{c}, []Contract{d}, runner); r.Passed() || runner.closed != 0 {
		t.Fatalf("%+v", r)
	}
}
