package integration

import (
	"testing"

	"aego/app/run"
	"aego/core/mathx"
	"aego/modules/gameplay"
	"aego/modules/input"
	"aego/scene"
)

type topdown struct {
	t    *testing.T
	app  *run.App
	in   *input.Service
	hero scene.NodeHandle
	axes [3]float32
}

func openTopdown(t *testing.T) *topdown {
	t.Helper()
	app := openAt(t, copyExample(t, "topdown"))
	sc := app.Scene
	td := &topdown{t: t, app: app, hero: sc.Find(sc.Root(), "Hero")}
	td.in = input.Of(app.Engine.MainWorld())
	if td.in == nil || td.hero.IsZero() {
		t.Fatal("the top-down example has no input service or no Hero")
	}
	return td
}

func (td *topdown) set(i int, name string, v float32) {
	if td.axes[i] != v {
		td.axes[i] = v
		if err := td.in.Inject(0, name, v); err != nil {
			td.t.Fatal(err)
		}
	}
}

// walk steers the hero towards a point with the move axes.
func (td *topdown) walk(to mathx.Vec2) float32 {
	d := to.Sub(td.app.Scene.WorldPosition(td.hero))
	l := d.Len()
	if l > 0.15 {
		d = d.Scale(1 / l)
	} else {
		d = mathx.Vec2{}
	}
	td.set(0, "MoveX", d.X)
	td.set(1, "MoveY", d.Y)
	return l
}

func (td *topdown) step() {
	td.app.Engine.Step(td.app.Driver())
}

type topdownRun struct {
	damaged, chased, patrolAgain bool
	stats                        gameplay.Stats
	hash                         uint64
}

func driveTopdown(t *testing.T) topdownRun {
	td := openTopdown(t)
	sc := td.app.Scene
	grunt := sc.Find(sc.Root(), "GruntC")
	coin := sc.WorldPosition(sc.Find(sc.Root(), "Coin1"))
	svc := gameplay.Of(td.app.Engine.MainWorld())
	var res topdownRun
	phase := 0
	for i := 0; i < 600; i++ {
		fsm, _ := scene.Get[gameplay.StateMachine](sc, grunt)
		hp, _ := scene.Get[gameplay.Health](sc, grunt)
		if fsm != nil && fsm.Current == "Chase" {
			res.chased = true
		}
		if hp != nil && hp.Current > 0 && hp.Current < hp.Max || !sc.IsAlive(grunt) {
			res.damaged = true
		}
		switch phase {
		case 0:
			if td.walk(coin) < 0.2 || svc.Stats().Pickups > 0 {
				phase = 1
			}
		case 1:
			if td.walk(mathx.Vec2{X: -7, Y: 1}) < 0.3 {
				phase = 2
			}
		case 2:
			if !sc.IsAlive(grunt) || res.damaged {
				phase = 3
				break
			}
			if td.walk(sc.WorldPosition(grunt)) < 1.3 {
				td.set(2, "Attack", float32(i/10%2))
			}
		case 3:
			td.set(2, "Attack", 0)
			td.walk(mathx.Vec2{X: -10, Y: -5})
			if res.chased && fsm != nil && fsm.Current == "Patrol" {
				res.patrolAgain = true
			}
		}
		td.step()
	}
	res.stats = svc.Stats()
	pf := &platformer{app: td.app}
	pf.noFaults(t)
	res.hash = digest(pf)
	return res
}

func TestTopdownExample(t *testing.T) {
	a := driveTopdown(t)
	if a.stats.Pickups == 0 {
		t.Fatal("the hero did not pick up a coin")
	}
	if !a.damaged {
		t.Fatal("the sword never hurt GruntC")
	}
	if !a.chased || !a.patrolAgain {
		t.Fatalf("chase %v, back to patrol %v", a.chased, a.patrolAgain)
	}
	if b := driveTopdown(t); b.hash != a.hash {
		t.Fatal("two runs of the same input differ")
	}
}
