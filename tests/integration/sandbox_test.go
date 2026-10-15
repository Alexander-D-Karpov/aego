package integration

import (
	"testing"

	"aego/modules/physics"
	"aego/modules/physics/world"
)

const laserType = "sandbox.lab.Laser"

func TestSandboxSettlesAndLaserHits(t *testing.T) {
	app, _ := openExample(t, "physics-sandbox")
	d := app.Driver()
	for i := 0; i < 600; i++ {
		app.Engine.Step(d)
	}
	sim := physics.SimOf(app.Engine.MainWorld())
	if sim == nil {
		t.Fatal("physics is not attached to the main world")
	}
	w := sim.World()
	dynamic := 0
	w.ForEachBody(func(h world.BodyHandle) bool {
		if tp, _ := w.BodyType(h); tp == world.Dynamic {
			dynamic++
		}
		return true
	})
	st := sim.Stats()
	if st.Bodies != dynamic+1 || st.Active != 0 || st.Sleeping != st.Bodies-1 {
		t.Fatalf("bodies %d dynamic %d active %d sleeping %d", st.Bodies, dynamic, st.Active, st.Sleeping)
	}
	sc := app.Scene
	id, ok := app.Scripts.InstanceOf(sc.Find(sc.Root(), "Laser"), laserType)
	if !ok {
		t.Fatal("laser has no script instance")
	}
	if v, _ := app.Scripts.Field(id, "hits"); v.Int() == 0 {
		t.Fatal("the laser never hit the wall")
	}
	if f := app.Scripts.Faults(); len(f) > 0 {
		t.Fatalf("script fault: %s", f[0].Fault.Message)
	}
}
