package integration

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"aego/app/run"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/engine"
	"aego/engine/recorder"
	"aego/modules/animation"
	"aego/modules/gameplay"
	"aego/modules/input"
	"aego/project"
	"aego/scene"
)

const detTicks = 600

type detInput struct {
	tick uint64
	name string
	v    float32
}

var detInputs = []detInput{
	{31, "Move", 1}, {61, "Jump", 1}, {75, "Jump", 0},
	{121, "Attack", 1}, {123, "Attack", 0}, {129, "Attack", 1}, {131, "Attack", 0},
	{201, "Move", -1}, {241, "Jump", 1}, {251, "Jump", 0},
	{301, "Move", 1}, {361, "Attack", 1}, {363, "Attack", 0}, {451, "Move", 0},
}

type animRec struct {
	tick   uint64
	kind   recorder.Kind
	owner  ids.NodeID
	name   string
	detail string
}

var simOwners = []string{"physics.sim", animation.ServiceID, gameplay.ServiceID}

func simHash(t *testing.T, w *engine.World) [32]byte {
	t.Helper()
	h := rewind.NewHasher()
	n := 0
	w.EachOwner(func(id string, v any) {
		if !slices.Contains(simOwners, id) {
			return
		}
		if k, ok := rewind.Of(v); ok {
			n++
			h.String(id)
			k.Hash(h)
		}
	})
	if n != len(simOwners) {
		t.Fatalf("the hash found %d of %v", n, simOwners)
	}
	return h.Sum()
}

func runAtRate(t *testing.T, frame float64) ([]animRec, [32]byte) {
	t.Helper()
	root := copyExample(t, "platformer")
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	app, err := run.Open(p, run.Config{Headless: true, FrameTime: frame, Seed: 42, Logger: log.Nop(), ScenePath: filepath.Join(root, "scenes", "main.aescene")}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	w := app.Engine.MainWorld()
	w.Recorder.SetFilter(recorder.Bit(recorder.Animation)|recorder.Bit(recorder.Gameplay), nil)
	in := input.Of(w)
	for _, x := range detInputs {
		if err := in.Inject(x.tick, x.name, x.v); err != nil {
			t.Fatal(err)
		}
	}
	d := app.Driver()
	twoSteps := frame > 1.0/45
	for w.Time.Tick() < detTicks {
		if !app.Engine.Step(d) {
			t.Fatalf("the game stopped: %v", app.Engine.Err())
		}
		if twoSteps && w.Time.Tick()%2 != 0 {
			t.Fatalf("a 1/30 frame ended on tick %d; the injections on odd ticks would land a step late", w.Time.Tick())
		}
	}
	if f := app.Scripts.Faults(); len(f) > 0 {
		t.Fatalf("%d script faults, first: %s", len(f), f[0].Fault.Message)
	}
	var out []animRec
	for _, r := range w.Recorder.Snapshot(nil) {
		if r.Detail == "presentation" {
			continue
		}
		out = append(out, animRec{tick: r.Tick, kind: r.Kind, owner: r.Owner, name: r.Name, detail: r.Detail})
	}
	return out, simHash(t, w)
}

func countRec(recs []animRec, name, detail string) int {
	n := 0
	for _, r := range recs {
		if r.kind == recorder.Animation && r.name == name && r.detail == detail {
			n++
		}
	}
	return n
}

func TestPlatformerAnimDeterminism(t *testing.T) {
	base, hash := runAtRate(t, 1.0/60)
	if countRec(base, "Hit", "enter") == 0 || countRec(base, "Hit", "") == 0 {
		t.Fatal("the run never attacked")
	}
	for _, ft := range []float64{1.0 / 30, 1.0 / 144} {
		got, h := runAtRate(t, ft)
		if !slices.Equal(got, base) {
			for i := range min(len(got), len(base)) {
				if got[i] != base[i] {
					t.Fatalf("frame time %v: record %d is %+v, at 1/60 %+v", ft, i, got[i], base[i])
				}
			}
			t.Fatalf("frame time %v: %d records, at 1/60 %d", ft, len(got), len(base))
		}
		if h != hash {
			t.Fatalf("frame time %v: the simulation hash differs", ft)
		}
	}
}

func TestHitPulseOnce(t *testing.T) {
	pf := openPlatformer(t)
	in := actionsOf(t, pf)
	for _, x := range []struct {
		tick uint64
		v    float32
	}{{31, 1}, {33, 0}, {37, 1}, {39, 0}, {91, 1}, {93, 0}} {
		if err := in.Inject(x.tick, "Attack", x.v); err != nil {
			t.Fatal(err)
		}
	}
	pf.step(140)
	player := pf.app.Scene.ID(pf.player)
	enters, hits := 0, 0
	for _, r := range pf.app.Engine.MainWorld().Recorder.Snapshot(nil) {
		if r.Kind != recorder.Animation || r.Owner != player || r.Name != "Hit" {
			continue
		}
		switch r.Detail {
		case "enter":
			enters++
		case "":
			hits++
		}
	}
	if enters != 2 || hits != 2 {
		t.Fatalf("Hit entered %d times and fired %d events, want 2 and 2: a press during the attack must not restart it", enters, hits)
	}
	pf.noFaults(t)
}

func TestRootMotionLunge(t *testing.T) {
	td := openTopdown(t)
	sc := td.app.Scene
	m, _ := scene.Get[gameplay.Motion2D](sc, td.hero)
	m.Teleport(mathx.Vec2{X: 11.2, Y: -3})
	for range 5 {
		td.step()
	}
	td.set(0, "MoveX", 1)
	td.step()
	td.step()
	td.set(0, "MoveX", 0)
	for range 30 {
		td.step()
	}
	x0 := sc.WorldPosition(td.hero).X
	td.set(2, "Attack", 1)
	td.step()
	td.set(2, "Attack", 0)
	for range 40 {
		td.step()
	}
	const face = 12.0 - 0.4
	x := sc.WorldPosition(td.hero).X
	if x > face || x < x0+0.1 {
		t.Fatalf("lunge from %v ended at %v, the wall stops it at %v", x0, x, face)
	}
	m, _ = scene.Get[gameplay.Motion2D](sc, td.hero)
	if v := m.Velocity.X; v > 1e-3 || v < -1e-3 {
		t.Fatalf("root motion changed the velocity: %v", v)
	}
	(&platformer{app: td.app}).noFaults(t)
}

func TestEnemyDieDestroys(t *testing.T) {
	_, pc := rewindSession(t)
	data, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatal(err)
	}
	actions := input.Of(pc.World())
	if _, err := actions.Replay(data); err != nil {
		t.Fatal(err)
	}
	dead, gone := 0, 0
	for i := 1; i <= runFrames && gone == 0; i++ {
		pc.Tick(1.0 / 60)
		sc := pc.Scene()
		h := sc.Find(sc.Root(), "EnemyLeft")
		switch {
		case h.IsZero() || !sc.IsAlive(h):
			gone = i
		case dead == 0:
			if hp, ok := scene.Get[gameplay.Health](sc, h); ok && hp.Dead {
				dead = i
			}
		}
	}
	if dead == 0 || gone == 0 {
		t.Fatalf("EnemyLeft died at %d, gone at %d", dead, gone)
	}
	if d := gone - dead; d < 10 || d > 20 {
		t.Fatalf("EnemyLeft was destroyed %d frames after death, the Die clip plays 0.2 s", d)
	}
	if err := pc.Seek(dead - 5); err != nil {
		t.Fatal(err)
	}
	sc := pc.Scene()
	h := sc.Find(sc.Root(), "EnemyLeft")
	if h.IsZero() || !sc.IsAlive(h) {
		t.Fatal("rewinding past the death did not bring EnemyLeft back")
	}
	if a, ok := scene.Get[animation.Animator](sc, h); !ok || a.State != "Walk" {
		t.Fatalf("EnemyLeft after rewind: animator %v %+v", ok, a)
	}
	if err := pc.Seek(gone); err != nil {
		t.Fatal(err)
	}
	sc = pc.Scene()
	if h := sc.Find(sc.Root(), "EnemyLeft"); !h.IsZero() && sc.IsAlive(h) {
		t.Fatal("replaying past the death must destroy EnemyLeft again")
	}
}

func TestDeterminismTopdown(t *testing.T) {
	p, err := project.Load(copyExample(t, "topdown"))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := run.Determinism(p, run.DeterminismOptions{Frames: 240, Every: 30})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Diverged != 0 {
		t.Fatalf("two runs diverge by frame %d", rep.Diverged)
	}
	if !slices.Contains(rep.Covered, animation.ServiceID) {
		t.Fatalf("hash does not cover %s: %v", animation.ServiceID, rep.Covered)
	}
}
