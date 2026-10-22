package integration

import (
	"testing"

	"aego/engine/recorder"
	"aego/modules/gameplay"
	"aego/scene"
)

func gameplayRecords(r *recorder.Recorder) map[string]int {
	out := map[string]int{}
	for _, rc := range r.Snapshot(nil) {
		if rc.Kind == recorder.Gameplay {
			out[rc.Name]++
		}
	}
	return out
}

// Damage and death reach the Event Monitor as gameplay records.
func TestGameplayEventsRecorded(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(60)
	hp, ok := scene.Get[gameplay.Health](pf.app.Scene, pf.enemy)
	if !ok {
		t.Fatal("EnemyLeft has no Health")
	}
	if !hp.Damage(hp.Max, pf.player) {
		t.Fatal("the enemy took no damage")
	}
	pf.step(140)
	rec := pf.app.Engine.MainWorld().Recorder
	got := gameplayRecords(rec)
	if got["damage"] == 0 || got["died"] == 0 {
		t.Fatalf("gameplay records %v, want damage and died", got)
	}
	if n := rec.Counts()[recorder.Gameplay]; n == 0 {
		t.Fatal("the gameplay count is zero")
	}
}

// A tick that records a gameplay event does not allocate; the dummy has no scripts, so only the engine side is measured.
func TestGameplayTickAllocs(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(120)
	sc := pf.app.Scene
	dummy := sc.Create(sc.Root(), "Dummy")
	hp := scene.Add[gameplay.Health](sc, dummy)
	*hp = gameplay.DefaultHealth()
	hp.InvulnTime, hp.Max = 0, 1<<30
	sc.Flush()
	pf.step(2)
	hp, _ = scene.Get[gameplay.Health](sc, dummy)
	rec := pf.app.Engine.MainWorld().Recorder
	before := rec.Counts()[recorder.Gameplay]
	a := testing.AllocsPerRun(60, func() {
		hp.Damage(1, pf.enemy)
		pf.step(1)
	})
	if rec.Counts()[recorder.Gameplay]-before < 60 {
		t.Fatalf("only %d gameplay records over 61 damaged ticks", rec.Counts()[recorder.Gameplay]-before)
	}
	if a != 0 {
		t.Fatalf("a tick with a damage record allocates %v times", a)
	}
}
