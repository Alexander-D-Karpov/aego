package integration

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"slices"
	"testing"

	"aego/modules/input"
	"aego/modules/physics/components"
	settingsmod "aego/modules/settings"
	"aego/scene"
)

func actionsOf(t testing.TB, pf *platformer) *input.Service {
	t.Helper()
	s := input.Of(pf.app.Engine.MainWorld())
	if s == nil {
		t.Fatal("the play world has no input service")
	}
	return s
}

func digest(pf *platformer) uint64 {
	h := fnv.New64a()
	s := pf.app.Scene
	var buf [4]byte
	put := func(f float32) {
		binary.LittleEndian.PutUint32(buf[:], math.Float32bits(f))
		h.Write(buf[:])
	}
	for _, n := range s.PreOrder() {
		if n == s.Root() || !s.IsAlive(n) || s.Space(n) != scene.Space2D {
			continue
		}
		t := s.Local(n)
		put(t.Position.X)
		put(t.Position.Y)
		put(t.Rotation)
		if b, ok := scene.Get[components.Body2D](s, n); ok {
			put(b.Velocity.X)
			put(b.Velocity.Y)
			put(b.AngularVelocity)
		}
	}
	return h.Sum64()
}

func TestInjectionMovesPlayerWithoutSDL(t *testing.T) {
	pf := openPlatformer(t)
	s := actionsOf(t, pf)
	pf.step(30)
	x0 := pf.app.Scene.Local(pf.player).Position.X
	if err := s.Inject(31, "Move", 1); err != nil {
		t.Fatal(err)
	}
	pf.step(60)
	if pf.app.Scene.Local(pf.player).Position.X <= x0+0.5 {
		t.Fatal("injected Move did not move the player")
	}
	pf.noFaults(t)
}

func TestRecordReplayIsBitwiseEqual(t *testing.T) {
	first := openPlatformer(t)
	s := actionsOf(t, first)
	s.Record(input.RecordMeta{Project: "platformer", FixedHz: 60})
	for _, in := range []struct {
		tick uint64
		name string
		v    float32
	}{{10, "Move", 1}, {40, "Jump", 1}, {43, "Jump", 0}, {90, "Move", -1}, {150, "Move", 0}, {200, "Jump", 1}, {201, "Jump", 0}, {240, "Move", 1}} {
		if err := s.Inject(in.tick, in.name, in.v); err != nil {
			t.Fatal(err)
		}
	}
	first.step(300)
	data := s.StopRecording()
	want := digest(first)
	first.noFaults(t)

	second := openPlatformer(t)
	meta, err := actionsOf(t, second).Replay(data)
	if err != nil {
		t.Fatal(err)
	}
	if meta.FixedHz != 60 {
		t.Fatalf("meta %+v", meta)
	}
	second.step(300)
	if got := digest(second); got != want {
		t.Fatalf("replay diverged: %x != %x", got, want)
	}
}

func TestPlatformerPauseOpensSettings(t *testing.T) {
	pf := openPlatformer(t)
	s := actionsOf(t, pf)
	w := pf.app.Engine.MainWorld()
	view := settingsmod.Of(w)
	if view == nil {
		t.Fatal("the play world has no settings view")
	}
	pf.step(10)
	press := func(name string) {
		if err := s.Inject(0, name, 1); err != nil {
			t.Fatal(err)
		}
		pf.step(1)
		if err := s.Inject(0, name, 0); err != nil {
			t.Fatal(err)
		}
		pf.step(1)
	}
	press("Pause")
	if !view.Opened() || !w.Time.Paused() || !slices.Contains(s.Contexts(), settingsmod.ContextView) {
		t.Fatalf("open %v paused %v stack %v", view.Opened(), w.Time.Paused(), s.Contexts())
	}
	tick := w.Time.Tick()
	pf.step(5)
	if w.Time.Tick() != tick {
		t.Fatal("fixed steps ran with the settings screen open")
	}
	press(settingsmod.ActionToggle)
	if view.Opened() || w.Time.Paused() {
		t.Fatalf("toggle did not close the screen: open %v paused %v", view.Opened(), w.Time.Paused())
	}
	pf.noFaults(t)
}
