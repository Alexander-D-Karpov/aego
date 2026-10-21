package audio

import (
	"math"
	"testing"

	"aego/app/assembly"
	"aego/audio/mix"
	"aego/core/log"
	"aego/demos"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/settings"
	audiomod "aego/modules/audio"
	"aego/schema"
	"aego/script/bind"
)

func boot(t *testing.T) (*engine.Engine, *audiomod.Module, *settings.Store) {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := module.Load(types, bind.New(), audiomod.New())
	if err != nil {
		t.Fatal(err)
	}
	st, err := man.NewSettings()
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.NewBuilder(engine.Config{
		Headless: true, HeadlessFrameTime: 1.0 / 60, UserRoot: t.TempDir(), AppID: "aego-audio-test",
	}).WithLogger(log.Nop()).Use(man.Engine()).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := man.Init(&module.InitContext{Engine: e, Settings: st, Log: log.Nop()}); err != nil {
		t.Fatal(err)
	}
	st.Start()
	t.Cleanup(func() {
		man.Shutdown()
		e.Close()
	})
	mod, ok := engine.Service[*audiomod.Module](e, audiomod.MixerID)
	if !ok || mod.Mixer() == nil {
		t.Fatal("no audio.mixer service")
	}
	return e, mod, st
}

func step(t *testing.T, e *engine.Engine, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if !e.Step(demos.Base{}) {
			t.Fatalf("engine stopped: %v", e.Err())
		}
	}
}

func set(t *testing.T, st *settings.Store, id string, v schema.Value) {
	t.Helper()
	tx := st.Begin()
	if err := tx.SetID(id, v); err != nil {
		t.Fatal(err)
	}
	if err := tx.Apply(); err != nil {
		t.Fatal(err)
	}
}

func TestBusSettings(t *testing.T) {
	e, mod, st := boot(t)
	step(t, e, 2)
	set(t, st, audiomod.VolumeKey("music"), schema.Float64(0.5))
	set(t, st, audiomod.MuteKey("sfx"), schema.Bool(true))
	step(t, e, 5)
	music, _ := mod.Mixer().Bus("music")
	sfx, _ := mod.Mixer().Bus("sfx")
	if g := mod.Mixer().BusGain(music); math.Abs(float64(g-0.5)) > 1e-6 {
		t.Fatalf("music bus gain %v", g)
	}
	if g := mod.Mixer().BusGain(sfx); g != 0 {
		t.Fatalf("muted sfx bus gain %v", g)
	}
}

func TestReopenKeepsVoices(t *testing.T) {
	e, mod, st := boot(t)
	var bad []mix.Event
	mod.OnEvent(func(ev mix.Event) {
		if ev.Kind == mix.EvFinished || ev.Kind == mix.EvStolen || ev.Kind == mix.EvRejected {
			bad = append(bad, ev)
		}
	})
	if !mod.Play(tone(440, rate, 0.3), "music", audiomod.PlayOptions{Loop: true}).Valid() {
		t.Fatal("play refused")
	}
	step(t, e, 3)
	set(t, st, audiomod.KeyBufferSize, schema.Enum(1024))
	step(t, e, 5)
	if f := mod.Spec().Frames; f != 1024 {
		t.Fatalf("output has %d frames per block after reopening", f)
	}
	if n := mod.Mixer().Active(); n != 1 || len(bad) != 0 {
		t.Fatalf("%d voices active, events %+v", n, bad)
	}
	if s := mod.Stats(); s.Underruns != 0 {
		t.Fatalf("%d underruns", s.Underruns)
	}
}
