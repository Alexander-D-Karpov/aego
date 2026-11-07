//go:build aego_editor

package docsview

import (
	"encoding/json"
	"fmt"
	"testing"

	"aego/app/playground"
)

func TestStringTryBridgeHistoryAndValidation(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("camera.CameraBrain2D")
	if !ok || e.Try == "" {
		t.Fatal("missing CameraBrain2D Try")
	}
	a := newHeadless(t)
	load, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", load); r.Error != "" {
		t.Fatal(r.Error)
	}
	step := func() {
		t.Helper()
		if err := a.Step(4); err != nil {
			t.Fatal(err)
		}
	}
	step()
	inspect := func(want string) {
		t.Helper()
		r := a.Call("inspect", []byte(`{"component":"camera.CameraBrain2D","fields":["camera.CameraBrain2D.Output"]}`))
		if r.Error != "" || r.Inspection == nil || r.Inspection.Observables["camera.CameraBrain2D.Output"] != want {
			t.Fatalf("want output %q: %+v", want, r)
		}
	}
	inspect("map")
	for _, value := range []string{`null`, `true`, `{}`, `[]`, `123`} {
		r := a.Call("set", []byte(fmt.Sprintf(`{"component":"camera.CameraBrain2D","field":"Output","value":%s}`, value)))
		if r.Error == "" {
			t.Fatalf("accepted %s for string field", value)
		}
	}
	for _, invalid := range []string{
		`{"component":"camera.CameraBrain2D","field":"Output"}`,
		`{"component":"engine.Camera2D","field":"Zoom","value":"40"}`,
	} {
		if r := a.Call("set", []byte(invalid)); r.Error == "" {
			t.Fatalf("accepted invalid set: %s", invalid)
		}
	}
	inspect("map")
	if a.session.State().CanUndo {
		t.Fatal("rejected edits changed history")
	}
	set := func(value string) {
		t.Helper()
		data, err := json.Marshal(map[string]any{"component": "camera.CameraBrain2D", "field": "Output", "value": value})
		if err != nil {
			t.Fatal(err)
		}
		if r := a.Call("set", data); r.Error != "" {
			t.Fatal(r.Error)
		}
		step()
	}
	set("")
	inspect("")
	if r := a.Call("undo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	step()
	inspect("map")
	if r := a.Call("redo", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	step()
	inspect("")
	set("карта \"A\"")
	inspect("карта \"A\"")
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	step()
	inspect("map")
	if a.session.State().CanUndo {
		t.Fatal("reset retained history")
	}
}
