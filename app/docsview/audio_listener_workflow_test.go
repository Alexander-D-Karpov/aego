//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/modules/audio"
)

func TestAudioListenerTryDisplaysEffectivePositionAndReset(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("audio.AudioListener2D")
	if !ok || e.Demo == nil {
		t.Fatal("audio listener contract is absent")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", req); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(12); err != nil {
		t.Fatal(err)
	}
	check := func(want float32) {
		t.Helper()
		sc := a.session.Controller().Scene()
		marker := sc.Find(sc.Root(), "Effective listener")
		if !sc.IsAlive(marker) {
			t.Fatal("listener indicator is absent")
		}
		service := audio.Of(a.session.Controller().World())
		if service == nil || service.Listener().X != want || sc.WorldPosition(marker).X != want {
			t.Fatalf("marker does not reflect actual audio listener position %v", want)
		}
		r := a.Call("inspect", []byte(`{"component":"audio.AudioListener2D","fields":["audio.AudioListener2D.ListenerX"]}`))
		if r.Error != "" || r.Inspection == nil || r.Inspection.Observables["audio.AudioListener2D.ListenerX"] != float64(want) {
			t.Fatalf("effective listener position is not observable: %+v", r)
		}
	}
	check(2)
	if r := a.Call("set", []byte(`{"component":"audio.AudioListener2D","field":"Active","value":0}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(12); err != nil {
		t.Fatal(err)
	}
	check(0)
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(12); err != nil {
		t.Fatal(err)
	}
	check(2)
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("listener Try runtime errors: %v", messages)
	}
}
