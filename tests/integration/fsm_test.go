package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/edit"
	"aego/contracts/persistence"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/play"
	"aego/engine"
	"aego/modules/gameplay"
	"aego/modules/savegame"
	"aego/project"
	"aego/save"
	"aego/scene"
	"aego/script/runtime"
	"aego/script/vm"
)

const brainType = "mygame.brain.Brain"

const brainSrc = `package brain

script Brain {
    var log int
    var ticks int
    var changes int
    var conn Connection

    func Ready() {
        conn = node.Get<StateMachine>().Changed.Connect(OnChanged)
    }

    func OnChanged(from int, to int) {
        changes++
    }

    func EnterIdle() {
        log = log * 10 + 1
    }

    func ExitIdle() {
        log = log * 10 + 2
    }

    func EnterRun() {
        log = log * 10 + 3
        node.Get<StateMachine>().Go("Air")
    }

    func ExitRun() {
        log = log * 10 + 4
    }

    func EnterAir() {
        log = log * 10 + 5
    }

    func FixedIdle(dt float) {
        ticks += 1
        if ticks == 3 {
            node.Get<StateMachine>().Go("Run")
        }
    }
}
`

const brainNode = `  node "c00000000000000000000000000000f1" name="Brain" enabled=true layers=1 {
    transform {
      position=[0, -20]
      rotation=0
      scale=[1, 1]
    }
    component "gameplay.StateMachine" version=1 {
      states=["Idle", "Run", "Air"]
      initial="Idle"
    }
    component "gameplay.Respawn" version=1 {
      delay=0
      persist=true
    }
    component "engine.Script" version=1 {
      script "mygame.brain.Brain" enabled=true {
      }
    }
  }
`

// brainProject is the platformer with a Brain node: a three-state machine driven by script hooks.
func brainProject(t *testing.T) string {
	t.Helper()
	root := copyExample(t, "platformer")
	dir := filepath.Join(root, "scripts", "brain")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "brain.aes"), []byte(brainSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "scenes", "main.aescene")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimRight(string(data), "\n")
	text = strings.TrimSuffix(text, "}") + brainNode + "}\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

type brain struct {
	t    *testing.T
	pf   *platformer
	node scene.NodeHandle
}

func openBrain(t *testing.T) *brain {
	t.Helper()
	pf := platformerAt(t, brainProject(t))
	sc := pf.app.Scene
	return &brain{t: t, pf: pf, node: sc.Find(sc.Root(), "Brain")}
}

func (b *brain) field(name string) int64 {
	id, ok := b.pf.app.Scripts.InstanceOf(b.node, brainType)
	if !ok {
		b.t.Fatal("Brain has no script instance")
	}
	v, _ := b.pf.app.Scripts.Field(id, name)
	return v.Int()
}

func (b *brain) fsm() *gameplay.StateMachine {
	m, ok := scene.Get[gameplay.StateMachine](b.pf.app.Scene, b.node)
	if !ok {
		b.t.Fatal("Brain has no StateMachine")
	}
	return m
}

func (b *brain) reload(from, to string) {
	b.t.Helper()
	pf := b.pf
	path := filepath.Join(pf.root, "scripts", "brain", "brain.aes")
	patch(b.t, path, from, to)
	pf.app.Workspace.Changed([]string{path})
	if _, _, err := pf.app.Pipeline.Scan(); err != nil {
		b.t.Fatal(err)
	}
	batch, err := pf.app.Pipeline.ImportAll()
	if err != nil {
		b.t.Fatal(err)
	}
	if err := pf.app.Loader.Swap(batch.Imported, pf.app.Pipeline.Artifacts()); err != nil {
		b.t.Fatal(err)
	}
}

func TestStateMachineTransitions(t *testing.T) {
	b := openBrain(t)
	b.pf.step(10)
	m := b.fsm()
	if strings.Join(m.States, ",") != "Idle,Run,Air" {
		t.Fatalf("states %v", m.States)
	}
	if got := b.field("log"); got != 12345 {
		t.Fatalf("hooks ran as %d, want 12345: a Go inside EnterRun waits for the transition", got)
	}
	if m.Current != "Air" || m.Previous != "Run" || !m.Is("Air") {
		t.Fatalf("current %s previous %s", m.Current, m.Previous)
	}
	if got := b.field("changes"); got != 2 {
		t.Fatalf("Changed fired %d times, want 2", got)
	}
	if m.Go("Nope") {
		t.Fatal("Go of an unknown state must return false")
	}
	b.pf.noFaults(t)
}

func TestStateMachineRebindsAfterReload(t *testing.T) {
	b := openBrain(t)
	b.pf.step(10)
	before := b.field("log")
	b.reload("func EnterAir() {", "func EnterAirborne() {")
	b.fsm().Go("Run")
	b.pf.step(3)
	if got := b.field("log"); got != before*100+34 {
		t.Fatalf("log %d after renaming EnterAir, want %d", got, before*100+34)
	}
	before = b.field("log")
	b.reload("func EnterAirborne() {\n        log = log * 10 + 5", "func EnterAir() {\n        log = log * 10 + 6")
	b.fsm().Go("Run")
	b.pf.step(3)
	if got := b.field("log"); got != before*1000+346 {
		t.Fatalf("log %d after the new EnterAir, want %d", got, before*1000+346)
	}
	b.pf.noFaults(t)
}

func TestRespawnPointPersists(t *testing.T) {
	b := openBrain(t)
	pf := b.pf
	st := save.NewStore(t.TempDir())
	pf.app.Engine.SetService(persistence.StoreID, st)
	pf.step(2)
	r, _ := scene.Get[gameplay.Respawn](pf.app.Scene, b.node)
	r.SetPoint(mathx.Vec2{X: 7, Y: -3})
	w := pf.app.Engine.MainWorld()
	doc, err := savegame.Of(w).Capture("fsm")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Write("fsm", save.Encode(doc), nil); err != nil {
		t.Fatal(err)
	}
	r.SetPoint(mathx.Vec2{X: -5, Y: 0})
	if err := savegame.Of(w).Load("fsm"); err != nil {
		t.Fatal(err)
	}
	pf.step(2)
	sc := pf.app.Scene
	h := sc.Find(sc.Root(), "Brain")
	r, ok := scene.Get[gameplay.Respawn](sc, h)
	if !ok {
		t.Fatal("Brain lost its Respawn after loading")
	}
	r.Trigger()
	pf.step(2)
	if p := sc.WorldPosition(h); p.Sub(mathx.Vec2{X: 7, Y: -3}).Len() > 1e-3 {
		t.Fatalf("respawned at %v, the saved point is (7, -3)", p)
	}
}

// brainSession opens the Brain project in the editor and starts Play, for rewind.
func brainSession(t *testing.T) (*edit.Session, *play.Controller) {
	t.Helper()
	p, err := project.Load(brainProject(t))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	pc := s.App.Play()
	if err := pc.StartProject(); err != nil {
		t.Fatal(err)
	}
	return s, pc
}

func brainLog(t *testing.T, pc *play.Controller) int64 {
	t.Helper()
	rt, ok := engine.ServiceOf[*runtime.Runtime](pc.World(), runtime.ServiceID)
	sc := pc.Scene()
	if !ok {
		t.Fatal("no script runtime")
	}
	id, ok := rt.InstanceOf(sc.Find(sc.Root(), "Brain"), brainType)
	if !ok || id == vm.NoInstance {
		t.Fatal("Brain has no script instance")
	}
	v, _ := rt.Field(id, "log")
	return v.Int()
}

func TestStateMachineRewind(t *testing.T) {
	s, pc := brainSession(t)
	pc.SetKeyEvery(1)
	var want int64
	var hash [32]byte
	drive(s.Host.Engine, pc, 0, 20, func(f int) {
		if f == 12 {
			want, hash = brainLog(t, pc), pausedHash(pc)
		}
	})
	if want != 12345 {
		t.Fatalf("log %d at frame 12", want)
	}
	if tl := pc.Timeline(); tl.KeysOff != "" {
		t.Fatalf("keyframes off: %s", tl.KeysOff)
	}
	if err := pc.Seek(2); err != nil {
		t.Fatal(err)
	}
	if got := brainLog(t, pc); got != 1 {
		t.Fatalf("log %d at frame 2, want 1: only Idle was entered", got)
	}
	if err := pc.Seek(12); err != nil {
		t.Fatal(err)
	}
	if got := brainLog(t, pc); got != want {
		t.Fatalf("log %d after rewind and replay, want %d", got, want)
	}
	if pc.StateHash() != hash {
		t.Fatal("state hash differs after rewind and replay")
	}
}
