package integration

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"aego/core/mathx"
	"aego/modules/gameplay"
	"aego/modules/input"
	"aego/scene"
)

var updateRun = flag.Bool("update", false, "record tests/integration/testdata/platformer_run.aeinput again")

const (
	runFile   = "testdata/platformer_run.aeinput"
	runFrames = 900
	runEnd    = 13
	pitLeft   = -1
)

// runner presses keys like a player: always right, jumps over the pit and onto enemies, steers onto their heads.
type runner struct {
	pf      *platformer
	in      *input.Service
	move    float32
	jumping int
}

func (r *runner) set(name string, v float32, cur *float32) {
	if *cur != v {
		*cur = v
		if err := r.in.Inject(0, name, v); err != nil {
			panic(err)
		}
	}
}

func (r *runner) nextEnemy(x float32) (mathx.Vec2, bool) {
	sc := r.pf.app.Scene
	best, found := mathx.Vec2{}, false
	for _, name := range []string{"EnemyLeft", "EnemyRight"} {
		h := sc.Find(sc.Root(), name)
		if h.IsZero() || !sc.IsAlive(h) {
			continue
		}
		if hp, ok := scene.Get[gameplay.Health](sc, h); ok && hp.Dead {
			continue
		}
		p := sc.WorldPosition(h)
		if !found || mathx.Abs(p.X-x) < mathx.Abs(best.X-x) {
			best, found = p, true
		}
	}
	return best, found
}

func sign(v float32) float32 {
	if v < 0 {
		return -1
	}
	return 1
}

func (r *runner) frame(jump *float32) {
	sc := r.pf.app.Scene
	p := sc.WorldPosition(r.pf.player)
	m, _ := scene.Get[gameplay.Motion2D](sc, r.pf.player)
	e, haveEnemy := r.nextEnemy(p.X)
	dx := e.X - p.X
	move := float32(1)
	switch {
	case !m.Grounded && m.Velocity.Y > 0 && p.Y > 2 && p.X > pitLeft-4 && p.X < pitLeft:
		move = 0
	case haveEnemy && !m.Grounded && mathx.Abs(dx) < 4:
		move = mathx.Clamp(dx*2, -1, 1)
	case haveEnemy:
		move = sign(dx)
	case p.X > runEnd+0.5:
		move = 0
	}
	r.set("Move", move, &r.move)
	if r.jumping > 0 {
		r.jumping--
		if r.jumping == 0 {
			r.set("Jump", 0, jump)
		}
		return
	}
	nearPit := move > 0 && p.X > pitLeft-2.2 && p.X < pitLeft-0.6
	want := m.Grounded && ((haveEnemy && mathx.Abs(dx) > 1.5 && mathx.Abs(dx) < 3.5) || nearPit)
	if want {
		r.set("Jump", 1, jump)
		r.jumping = 40
	}
}

func drivePlatformer(t *testing.T, pf *platformer, frames int) {
	r := &runner{pf: pf, in: actionsOf(t, pf)}
	var jump float32
	for i := 0; i < frames; i++ {
		r.frame(&jump)
		pf.step(1)
	}
}

func recordRun(t *testing.T) []byte {
	pf := openPlatformer(t)
	s := actionsOf(t, pf)
	s.Record(input.RecordMeta{Project: "platformer", FixedHz: 60})
	drivePlatformer(t, pf, runFrames)
	return s.StopRecording()
}

type runResult struct {
	x     float32
	kills int64
	stats gameplay.Stats
	h200  uint64
	hEnd  uint64
}

func replayRun(t *testing.T, data []byte) runResult {
	t.Helper()
	pf := openPlatformer(t)
	if _, err := actionsOf(t, pf).Replay(data); err != nil {
		t.Fatal(err)
	}
	var res runResult
	for i := 1; i <= runFrames; i++ {
		pf.step(1)
		if i == 200 {
			res.h200 = digest(pf)
		}
	}
	res.hEnd = digest(pf)
	res.x = pf.app.Scene.WorldPosition(pf.player).X
	res.kills = pf.field("kills").Int()
	res.stats = gameplay.Of(pf.app.Engine.MainWorld()).Stats()
	pf.noFaults(t)
	return res
}

// TestPlatformerRunReplays plays the recorded run: the level is beaten without dying, both enemies stomped.
func TestPlatformerRunReplays(t *testing.T) {
	if *updateRun {
		data := recordRun(t)
		if err := os.MkdirAll(filepath.Dir(runFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(runFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatalf("%v; record it with go test ./tests/integration -run TestPlatformerRunReplays -update", err)
	}
	a := replayRun(t, data)
	if a.x < runEnd {
		t.Fatalf("the run stopped at x=%v, the right edge is past %v", a.x, runEnd)
	}
	if a.kills != 2 || a.stats.Respawns != 0 || a.stats.Deaths != 2 {
		t.Fatalf("kills %d, stats %+v: want both enemies stomped and no respawn", a.kills, a.stats)
	}
	b := replayRun(t, data)
	if a.h200 != b.h200 || a.hEnd != b.hEnd {
		t.Fatal("two replays of the run differ")
	}
}

// TestPlatformerPitRespawns drops the player into the pit: it comes back at the checkpoint once taken, at the start before, with full health.
func TestPlatformerPitRespawns(t *testing.T) {
	for _, took := range []bool{false, true} {
		pf := openPlatformer(t)
		sc := pf.app.Scene
		pf.step(10)
		start := sc.WorldPosition(pf.player)
		want := start
		if took {
			want = sc.WorldPosition(sc.Find(sc.Root(), "Checkpoint"))
			sc.SetPosition(pf.player, mathx.Vec2{X: want.X, Y: 2.9})
			pf.step(10)
		}
		sc.SetPosition(pf.player, mathx.Vec2{X: 0.5, Y: 2.9})
		pf.health().Damage(1, scene.NodeHandle{})
		svc := gameplay.Of(pf.app.Engine.MainWorld())
		for i := 0; i < 240 && svc.Stats().Respawns == 0; i++ {
			pf.step(1)
		}
		if svc.Stats().Respawns != 1 {
			t.Fatal("falling into the pit did not respawn the player")
		}
		pf.step(30)
		p := sc.WorldPosition(pf.player)
		if mathx.Abs(p.X-want.X) > 0.2 || !pf.motion().Grounded {
			t.Fatalf("checkpoint taken %v: respawned at %v, standing %v, want x=%v", took, p, pf.motion().Grounded, want.X)
		}
		if hp := pf.health(); hp.Current != hp.Max || hp.Dead {
			t.Fatalf("health %d of %d after respawn", hp.Current, hp.Max)
		}
		pf.noFaults(t)
	}
}
