package integration

import (
	"testing"

	"aego/engine/recorder"
	"aego/schema"
)

func TestPlatformerSettingChangedEvent(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(5)
	vol := 0.5
	if st := pf.app.Settings; st.Value(st.Key("audio.bus.music.volume")).F == vol {
		vol = 0.25
	}
	tx := pf.app.Settings.Begin()
	if err := tx.SetID("audio.bus.music.volume", schema.Float64(vol)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Apply(); err != nil {
		t.Fatal(err)
	}
	pf.step(3)
	player := pf.app.Scene.ID(pf.player)
	n := 0
	for _, r := range pf.app.Engine.MainWorld().Recorder.Snapshot(nil) {
		if r.Kind == recorder.Event && r.Name == "OnSettingChanged" {
			if r.Owner != player {
				t.Fatalf("delivered to %v, want the player %v", r.Owner, player)
			}
			n++
		}
	}
	if n != 1 {
		t.Fatalf("OnSettingChanged delivered %d times, want 1", n)
	}
	pf.noFaults(t)
}
