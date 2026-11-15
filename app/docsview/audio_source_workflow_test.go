//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/asset"
	"aego/modules/audio"
	"aego/modules/render2d"
	"aego/scene"
)

// The example requests a real looped voice at volume zero. The color reads the
// audio service's actual SourcePlaying state, not the editable checkbox.
func TestAudioSourceTryStartsRealSilentVoiceAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("audio.AudioSource2D")
	if !ok || e.Demo == nil {
		t.Fatal("audio source contract is absent")
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
	state := func() (*audio.AudioSource2D, *render2d.SpriteRenderer, bool) {
		t.Helper()
		sc := a.session.Controller().Scene()
		h := sc.Find(sc.Root(), "Source")
		src, ok := scene.Get[audio.AudioSource2D](sc, h)
		if !ok || src.Clip.ID != asset.BuiltinHum || src.Volume != 0 || !src.Loop {
			t.Fatal("expected a silent looping builtin source")
		}
		marker, ok := scene.Get[render2d.SpriteRenderer](sc, h)
		if !ok {
			t.Fatal("audio source indicator is absent")
		}
		service := audio.Of(a.session.Controller().World())
		if service == nil {
			t.Fatal("audio service is absent")
		}
		return src, marker, service.SourcePlaying(h)
	}
	src, marker, playing := state()
	idleColor := marker.Tint
	if src.PlayOnStart || src.Playing || playing {
		t.Fatal("source should start without a voice")
	}
	if reply := a.Call("inspect", []byte(`{"component":"audio.AudioSource2D","fields":["audio.AudioSource2D.Playing"],"dependencies":["ae900000000000000000000000000a03"]}`)); reply.Error != "" || reply.Inspection == nil || len(reply.Inspection.Dependencies) != 1 {
		t.Fatalf("the actual builtin audio dependency was not resolved: %+v", reply)
	}
	if reply := a.Call("set", []byte(`{"component":"audio.AudioSource2D","field":"PlayOnStart","value":1}`)); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(12); err != nil {
		t.Fatal(err)
	}
	src, marker, playing = state()
	if !src.PlayOnStart || !src.Playing || !playing || marker.Tint == idleColor {
		t.Fatalf("real silent voice did not start: enabled=%v playing=%v service=%v tint=%v", src.PlayOnStart, src.Playing, playing, marker.Tint)
	}
	if reply := a.Call("reset", nil); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(12); err != nil {
		t.Fatal(err)
	}
	src, marker, playing = state()
	if src.PlayOnStart || src.Playing || playing || marker.Tint != idleColor {
		t.Fatal("Reset did not stop the voice and restore the indicator")
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("audio Try runtime errors: %v", messages)
	}
}
