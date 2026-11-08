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

func TestScriptAttachmentActionIsExplicitAndWellFormed(t *testing.T) {
	c, d := fixture()
	c.ID, c.DocumentID, c.Scene, c.Fields = "engine.Script", "engine.Script", "engine.Script/try", nil
	d.ComponentID, d.DocumentID, d.Scene, d.EditableFields = c.ID, c.DocumentID, c.Scene, nil
	on := true
	d.Actions[0].Field, d.Actions[0].Value = "", nil
	d.Actions[0].ScriptToggle = &ScriptToggle{Script: "game.demo.Move", Enabled: &on}
	if r := Check(context.Background(), []Component{c}, []Contract{d}, &fakeRunner{}); !r.Passed() {
		t.Fatalf("valid attachment action rejected: %+v", r)
	}
	// Other component demos may toggle a script on their selected node, but
	// the real runner must still observe that component and its changed state.
	other, otherContract := c, d
	other.ID, other.DocumentID, other.Scene = "test.Counter", "test.Counter", "test.Counter/try"
	otherContract.ComponentID, otherContract.DocumentID, otherContract.Scene = other.ID, other.DocumentID, other.Scene
	if r := Check(context.Background(), []Component{other}, []Contract{otherContract}, &fakeRunner{}); !r.Passed() {
		t.Fatalf("valid selected-node script action rejected: %+v", r)
	}
	for _, fault := range []string{"no-operation", "empty-script", "missing-enabled", "mixed-field", "extra-value"} {
		t.Run(fault, func(t *testing.T) {
			component, contract := c, d
			contract.Actions = append([]Action(nil), d.Actions...)
			toggle := *d.Actions[0].ScriptToggle
			contract.Actions[0].ScriptToggle = &toggle
			switch fault {
			case "no-operation":
				contract.Actions[0].ScriptToggle = nil
			case "empty-script":
				toggle.Script = ""
			case "missing-enabled":
				toggle.Enabled = nil
			case "mixed-field":
				contract.Actions[0].Field = "engine.Script.fake"
			case "extra-value":
				contract.Actions[0].Value = json.RawMessage("1")
			}
			if r := Check(context.Background(), []Component{component}, []Contract{contract}, &fakeRunner{}); r.Passed() {
				t.Fatalf("accepted invalid attachment action: %+v", r)
			}
		})
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

type visualRunner struct{ fault string }
type visualInstance struct {
	fault, env string
	contract   Contract
	value      int
	actions    int
}

func (r visualRunner) Open(_ context.Context, c Contract, env string) (Instance, error) {
	return &visualInstance{contract: c, env: env, fault: r.fault}, nil
}
func (p *visualInstance) Observe(context.Context) (Observation, error) {
	raw, _ := json.Marshal(p.value)
	o := Observation{Identity: p.contract.Identity, Components: []string{p.contract.ComponentID}, Dependencies: p.contract.Dependencies, RuntimeExecuting: true, Observables: map[string]json.RawMessage{"count": raw}}
	if p.fault == "missing" {
		return o, nil
	}
	n := 1
	if p.env == "chromium" {
		n = 2
	}
	for i := 0; i < n; i++ {
		pixel := byte(0)
		if p.value == 2 || p.fault == "reset" && p.actions > 0 {
			pixel = 255
		}
		if p.fault == "static" || p.fault == "one-language" && i == 1 {
			pixel = 0
		}
		frame := Frame{Width: 1, Height: 1, Pixels: []byte{pixel, 0, 0, 255}}
		if p.fault == "wrong-size" && p.value == 2 {
			frame.Width = 2
		}
		o.Frames = append(o.Frames, frame)
	}
	return o, nil
}
func (p *visualInstance) Action(context.Context, Action) error { p.value = 2; p.actions++; return nil }
func (p *visualInstance) Reset(context.Context, string) error  { p.value = 0; return nil }
func (p *visualInstance) Close() error                         { return nil }

func TestVisualEvidenceRequiresChangedAndResetGameViews(t *testing.T) {
	c, d := fixture()
	d.Actions[0].Visual = &VisualExpectation{MinChangedPixels: 1, MaxResetDifferencePixels: 0}
	if r := Check(context.Background(), []Component{c}, []Contract{d}, visualRunner{}); !r.Passed() {
		t.Fatalf("real changes in all frames should pass: %+v", r)
	}
	for _, fault := range []string{"missing", "static", "one-language", "wrong-size", "reset"} {
		t.Run(fault, func(t *testing.T) {
			r := Check(context.Background(), []Component{c}, []Contract{d}, visualRunner{fault})
			if r.Passed() {
				t.Fatalf("accepted bad Game View evidence: %+v", r)
			}
		})
	}
	d.Actions[0].Visual.MinChangedPixels = 0
	if r := Check(context.Background(), []Component{c}, []Contract{d}, visualRunner{}); r.Passed() {
		t.Fatalf("accepted a zero-pixel visual threshold: %+v", r)
	}
}
