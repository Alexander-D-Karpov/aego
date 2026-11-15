//go:build aego_editor

package docsview

import (
	"encoding/json"
	"math"
	"testing"

	"aego/app/playground"
	"aego/engine"
	"aego/modules/audio"
	"aego/modules/render2d"
	"aego/scene"
)

func TestAudioZoneTryChangesRealWorldBusAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("audio.AudioZone2D")
	if !ok || e.Demo == nil {
		t.Fatal("audio zone contract is absent")
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
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(16); err != nil {
		t.Fatal(err)
	}
	check := func(want float64) *render2d.SpriteRenderer {
		t.Helper()
		world := a.session.Controller().World()
		mod, ok := engine.ServiceOf[*audio.Module](world, audio.MixerID)
		if !ok || mod.Mixer() == nil || math.Abs(float64(mod.Mixer().BusGain(int(audio.BusMusic)))-want) > 0.001 {
			t.Fatalf("actual mixer music bus did not reach gain %v", want)
		}
		r := a.Call("inspect", []byte(`{"component":"audio.AudioZone2D","fields":["audio.AudioZone2D.WorldGain"]}`))
		if r.Error != "" || r.Inspection == nil || r.Inspection.Observables["audio.AudioZone2D.WorldGain"] != want {
			t.Fatalf("runtime world gain is not observable: %+v", r)
		}
		sc := a.session.Controller().Scene()
		h := sc.Find(sc.Root(), "Indicator")
		marker, ok := scene.Get[render2d.SpriteRenderer](sc, h)
		if !ok {
			t.Fatal("gain indicator is absent")
		}
		return marker
	}
	before := check(0.25).Tint
	if r := a.Call("set", []byte(`{"component":"audio.AudioZone2D","field":"Gain","value":0.75}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(16); err != nil {
		t.Fatal(err)
	}
	if marker := check(0.75); marker.Tint == before {
		t.Fatal("indicator did not follow the actual world gain")
	}
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(16); err != nil {
		t.Fatal(err)
	}
	if marker := check(0.25); marker.Tint != before {
		t.Fatal("Reset did not restore the gain indicator")
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("audio zone Try runtime errors: %v", messages)
	}
}
