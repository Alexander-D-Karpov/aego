//go:build aego_editor

package docsview

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"aego/app/playground"
)

// Exercises actual component worlds with controlled headless input. This is
// not a substitute for the native SDL and browser/pixel acceptance gates.
func TestComponentContractRuntimeObservables(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range p.Docs().ComponentDemos() {
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
				if failures := errorsOf(a); len(failures) != 0 {
					t.Fatal(failures)
				}
			}
			warmup := contract.WarmupFrames
			if warmup == 0 {
				warmup = 4
			}
			reset := func() {
				t.Helper()
				if reply := a.Call("reset", nil); reply.Error != "" {
					t.Fatal(reply.Error)
				}
				advance(warmup, nil)
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
