//go:build aego_editor

package docsview

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/app/playground"
	"aego/contracts/persistence"
	"aego/engine"
	"aego/modules/savegame"
	"aego/save"
)

type componentRuntimeHooks struct {
	only        string
	beforeCycle func(*testing.T, *App, string)
	onWait      func(*testing.T, *App, string, persistCycleEvents)
}

type persistCycleEvents struct {
	saved  bool
	loaded bool
}

func (e *persistCycleEvents) record(lines []Line) {
	for _, line := range lines {
		e.saved = e.saved || line.Text == "game saved"
		e.loaded = e.loaded || line.Text == "game loaded"
	}
}

func (e persistCycleEvents) complete(slotRemoved bool) bool {
	return e.saved && e.loaded && slotRemoved
}

// Exercises actual component worlds with controlled headless input. This is
// not a substitute for the native SDL and browser/pixel acceptance gates.
func TestComponentContractRuntimeObservables(t *testing.T) {
	runComponentContractRuntimeObservables(t, componentRuntimeHooks{})
}

func runComponentContractRuntimeObservables(t *testing.T, hooks componentRuntimeHooks) {
	t.Helper()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range p.Docs().ComponentDemos() {
		if hooks.only != "" && component.ID != hooks.only {
			continue
		}
		t.Run(component.ID, func(t *testing.T) {
			entry, ok := p.Docs().Get(component.ID)
			if !ok || entry.Demo == nil {
				t.Fatal("component has no behavioral contract")
			}
			contract := *entry.Demo
			a := newHeadless(t)
			if reply := a.Call("control", []byte(`{"enabled":true}`)); reply.Error != "" {
				t.Fatal(reply.Error)
			}
			code := ""
			if entry.TryScript != "" {
				var err error
				code, err = p.TryCode(entry.TryScript)
				if err != nil {
					t.Fatal(err)
				}
			}
			load, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "prefabs": entry.TryPrefabs, "artifacts": entry.TryArtifacts})
			if err != nil {
				t.Fatal(err)
			}
			if reply := a.Call("load", load); reply.Error != "" {
				t.Fatal(reply.Error)
			}
			var cycle persistCycleEvents
			collect := func() {
				t.Helper()
				if component.ID == "save.Persist" {
					cycle.record(a.lines)
				}
				if failures := errorsOf(a); len(failures) != 0 {
					t.Fatal(failures)
				}
			}
			advance := func(frames int, keys []string) {
				t.Helper()
				req, err := json.Marshal(map[string]any{"frames": frames, "keys": keys})
				if err != nil {
					t.Fatal(err)
				}
				if reply := a.Call("advance", req); reply.Error != "" {
					t.Fatal(reply.Error)
				}
				if err := a.Step(frames); err != nil || a.control.Remaining != 0 {
					t.Fatalf("controlled input failed: %v", err)
				}
				collect()
			}
			beginCycle := func(phase string) {
				t.Helper()
				collect()
				cycle = persistCycleEvents{}
				if hooks.beforeCycle != nil {
					hooks.beforeCycle(t, a, phase)
				}
			}
			waitCycle := func(phase string) {
				t.Helper()
				if component.ID != "save.Persist" {
					return
				}
				deadline := time.Now().Add(10 * time.Second)
				waitNotified := false
				for {
					world := a.session.Controller().World()
					removed := false
					if world != nil {
						service := savegame.Of(world)
						store, ok := engine.Service[*save.Store](a.e, persistence.StoreID)
						removed = service != nil && ok && store != nil && !store.Exists("persist-demo")
					}
					if cycle.complete(removed) {
						return
					}
					if !time.Now().Before(deadline) {
						t.Fatalf("%s: real save/load did not complete: saved=%v loaded=%v slot removed=%v", phase, cycle.saved, cycle.loaded, removed)
					}
					if !waitNotified && hooks.onWait != nil {
						waitNotified = true
						hooks.onWait(t, a, phase, cycle)
					}
					advance(4, nil)
					runtime.Gosched()
				}
			}
			warmup := contract.WarmupFrames
			if warmup == 0 {
				warmup = 4
			}
			reset := func() {
				t.Helper()
				beginCycle("reset")
				if reply := a.Call("reset", nil); reply.Error != "" {
					t.Fatal(reply.Error)
				}
				advance(warmup, nil)
				waitCycle("reset")
			}
			for _, action := range contract.Actions {
				reset()
				fields := make([]string, 0, len(action.Expected))
				for _, expect := range action.Expected {
					fields = append(fields, expect.Observable)
				}
				inspect := func() map[string]any {
					t.Helper()
					result, err := a.inspectTry(component.ID, fields, contract.Dependencies)
					if err != nil {
						t.Fatal(err)
					}
					if !result.RuntimeExecuting {
						t.Fatal("Try world is not executing")
					}
					return result.Observables
				}
				before := inspect()
				beginCycle("action")
				if action.ScriptToggle != nil {
					req, err := json.Marshal(action.ScriptToggle)
					if err != nil {
						t.Fatal(err)
					}
					if reply := a.Call("scriptEnabled", req); reply.Error != "" {
						t.Fatal(reply.Error)
					}
				} else if action.Field != "" {
					field, ok := strings.CutPrefix(action.Field, component.ID+".")
					if !ok || field == "" {
						t.Fatalf("invalid action field %q", action.Field)
					}
					req, err := json.Marshal(map[string]any{"component": component.ID, "field": field, "value": action.Value})
					if err != nil {
						t.Fatal(err)
					}
					if reply := a.Call("set", req); reply.Error != "" {
						t.Fatal(reply.Error)
					}
				}
				if (action.Field != "" || action.ScriptToggle != nil) && (len(action.Keys) != 0 || len(action.Inputs) != 0) {
					advance(warmup, nil)
				}
				for _, step := range action.Inputs {
					advance(step.Frames, step.Keys)
				}
				if len(action.Inputs) == 0 || action.Frames != 0 || len(action.Keys) != 0 {
					frames := action.Frames
					if frames == 0 {
						frames = warmup
					}
					advance(frames, action.Keys)
				}
				waitCycle("action")
				after := inspect()
				for _, expect := range action.Expected {
					got, gotOK := after[expect.Observable]
					old, oldOK := before[expect.Observable]
					actualJSON, _ := json.Marshal(got)
					baselineJSON, _ := json.Marshal(old)
					if !gotOK || !oldOK || !runtimeEqual(actualJSON, expect.Equals, expect.Tolerance) {
						t.Errorf("%s: got %s, want %s (tolerance %g)", expect.Observable, actualJSON, expect.Equals, expect.Tolerance)
					}
					if expect.Changes && runtimeEqual(actualJSON, baselineJSON, expect.Tolerance) {
						t.Errorf("%s did not change", expect.Observable)
					}
				}
				reset()
				restored := inspect()
				for _, expect := range action.Expected {
					got, ok := restored[expect.Observable]
					actualJSON, _ := json.Marshal(got)
					baselineJSON, _ := json.Marshal(before[expect.Observable])
					if !ok || !runtimeEqual(actualJSON, baselineJSON, expect.Tolerance) {
						t.Errorf("Reset did not restore %s: %s != %s", expect.Observable, actualJSON, baselineJSON)
					}
				}
			}
		})
	}
}

func runtimeEqual(a, b json.RawMessage, tolerance float64) bool {
	if !json.Valid(a) || !json.Valid(b) {
		return false
	}
	var x, y any
	left, right := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	left.UseNumber()
	right.UseNumber()
	if left.Decode(&x) != nil || right.Decode(&y) != nil {
		return false
	}
	if xn, ok := x.(json.Number); ok && tolerance > 0 {
		if yn, ok := y.(json.Number); ok {
			xf, xe := xn.Float64()
			yf, ye := yn.Float64()
			return xe == nil && ye == nil && math.Abs(xf-yf) <= tolerance
		}
	}
	return reflect.DeepEqual(x, y)
}
