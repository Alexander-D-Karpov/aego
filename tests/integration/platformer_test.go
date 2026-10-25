package integration

import (
	"aego/contracts/physics2d"
	"aego/engine"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/run"
	"aego/contracts/persistence"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/modules/gameplay"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/modules/savegame"
	"aego/modules/spawn"
	"aego/platform"
	"aego/platform/input"
	"aego/project"
	"aego/save"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/script/vm"
	"aego/sdk"
)

type EnemyStats struct {
	Speed    float32     `aego:"speed,min=0,max=10"`
	Cooldown float32     `aego:"cooldown,min=0,max=5"`
	Damage   int64       `aego:"damage,min=0"`
	Tint     mathx.Color `aego:"tint"`
}

type combatModule struct{}

func (combatModule) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "mygame", Version: "0.1.0"}
}

func (combatModule) Register(r *sdk.Registry) error {
	if err := sdk.DataType[EnemyStats](r, sdk.Name("mygame.EnemyStats")).Err(); err != nil {
		return err
	}
	sdk.Live[EnemyStats](r, "speed")
	return nil
}

const playerType = "mygame.player.PlayerController"

func copyExample(t testing.TB, name string) string {
	t.Helper()
	src := filepath.Join("..", "..", "examples", name)
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch rel {
			case ".aego", "native", "exports":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if rel == "go.mod" || rel == "go.sum" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if rel == "project.aego" {
			data = []byte(strings.Replace(string(data), "native=true", "native=false", 1))
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func openAt(t testing.TB, root string, mods ...sdk.Module) *run.App {
	t.Helper()
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	app, err := run.Open(p, run.Config{Headless: true, FrameTime: 1.0 / 60, Seed: 42, Logger: log.Nop(), ScenePath: filepath.Join(root, "scenes", "main.aescene")}, mods...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app
}

func openExample(t testing.TB, name string, mods ...sdk.Module) (*run.App, string) {
	t.Helper()
	root := copyExample(t, name)
	return openAt(t, root, mods...), root
}

type platformer struct {
	app    *run.App
	root   string
	player scene.NodeHandle
	enemy  scene.NodeHandle
	pid    vm.InstanceID
}

func platformerAt(t testing.TB, root string) *platformer {
	t.Helper()
	app := openAt(t, root, combatModule{})
	sc := app.Scene
	pf := &platformer{app: app, root: root, player: sc.Find(sc.Root(), "Player"), enemy: sc.Find(sc.Root(), "EnemyLeft")}
	id, ok := app.Scripts.InstanceOf(pf.player, playerType)
	if !ok {
		t.Fatal("player has no PlayerController instance")
	}
	pf.pid = id
	return pf
}

func openPlatformer(t testing.TB) *platformer {
	t.Helper()
	return platformerAt(t, copyExample(t, "platformer"))
}

// hitPlayer deals one point of damage from EnemyLeft, as its hitbox would.
func (pf *platformer) hitPlayer(t testing.TB) {
	t.Helper()
	hp, ok := scene.Get[gameplay.Health](pf.app.Scene, pf.player)
	if !ok || !hp.Damage(1, pf.enemy) {
		t.Fatal("the player was not hit")
	}
	pf.step(1)
}

// stomp drops the player onto EnemyLeft from above and waits for the kill.
func (pf *platformer) stomp(t testing.TB) {
	t.Helper()
	sc := pf.app.Scene
	e := sc.Local(pf.enemy).Position
	sc.SetPosition(pf.player, mathx.Vec2{X: e.X, Y: e.Y - 2.5})
	for i := 0; i < 90 && pf.field("kills").Int() == 0; i++ {
		e = sc.Local(pf.enemy).Position
		p := sc.Local(pf.player).Position
		sc.SetPosition(pf.player, mathx.Vec2{X: e.X, Y: p.Y})
		pf.step(1)
	}
	pf.step(20)
}

func (pf *platformer) field(name string) vm.Value {
	v, _ := pf.app.Scripts.Field(pf.pid, name)
	return v
}

func (pf *platformer) step(n int) {
	d := pf.app.Driver()
	for i := 0; i < n; i++ {
		pf.app.Engine.Step(d)
	}
}

func (pf *platformer) health() *gameplay.Health {
	h, _ := scene.Get[gameplay.Health](pf.app.Scene, pf.player)
	return h
}

func (pf *platformer) motion() *gameplay.Motion2D {
	m, _ := scene.Get[gameplay.Motion2D](pf.app.Scene, pf.player)
	return m
}

func (pf *platformer) body(h scene.NodeHandle) components.Body2D {
	b, _ := scene.Get[components.Body2D](pf.app.Scene, h)
	return *b
}

func (pf *platformer) y() float32 {
	return pf.app.Scene.Local(pf.player).Position.Y
}

func (pf *platformer) hold(k input.Scancode, down bool) {
	kind := platform.EvKeyUp
	if down {
		kind = platform.EvKeyDown
	}
	platform.ApplyInput(pf.app.Engine.Input(), platform.Event{Kind: kind, Key: input.KeyEvent{Scan: k}})
}

func (pf *platformer) noFaults(t testing.TB) {
	t.Helper()
	if f := pf.app.Scripts.Faults(); len(f) > 0 {
		t.Fatalf("%d script faults, first: %s %s", len(f), f[0].Type, f[0].Fault.Message)
	}
}

func TestPlatformerSceneIsByteStable(t *testing.T) {
	pf := openPlatformer(t)
	data, _ := os.ReadFile(filepath.Join(pf.root, "scenes", "main.aescene"))
	if string(sceneio.Save(pf.app.Scene)) != string(data) {
		t.Fatal("platformer scene is not byte-stable")
	}
}

func TestPlatformerStandsOnGround(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(120)
	if vy := pf.motion().Velocity.Y; math.Abs(float64(vy)) > 0.05 {
		t.Fatalf("player still moves vertically: %v", vy)
	}
	if y := pf.y(); y < 2.9 || y > 3.05 {
		t.Fatalf("player rests at y=%v, want about 3", y)
	}
	if m, _ := scene.Get[gameplay.Motion2D](pf.app.Scene, pf.player); !m.Grounded {
		t.Fatal("Motion 2D never reported the floor")
	}
	pf.noFaults(t)
}

func TestPlatformerJump(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(60)
	y0 := pf.y()
	rise := func(hold int) float32 {
		pf.hold(input.KeySpace, true)
		top := y0
		for i := 0; i < 120; i++ {
			if i == hold {
				pf.hold(input.KeySpace, false)
			}
			pf.step(1)
			top = min(top, pf.y())
		}
		pf.step(60)
		if !pf.motion().Grounded || math.Abs(float64(pf.y()-y0)) > 0.05 {
			t.Fatalf("player did not land: y=%v", pf.y())
		}
		return y0 - top
	}
	full, short := rise(60), rise(3)
	if full < 2.4 || full > 2.8 {
		t.Fatalf("a held jump rose %v units, Jump Height is 2.6", full)
	}
	if short > full*0.6 {
		t.Fatalf("a tapped jump rose %v units, the held one %v: Jump Released must cut it", short, full)
	}
	pf.noFaults(t)
}

func TestPlatformerStompEmitsDied(t *testing.T) {
	pf := openPlatformer(t)
	sc := pf.app.Scene
	pf.step(30)
	pf.stomp(t)
	if k := pf.field("kills").Int(); k != 1 {
		t.Fatalf("kills %d", k)
	}
	if sc.IsAlive(pf.enemy) {
		t.Fatal("the stomped enemy is still alive")
	}
	pf.noFaults(t)
}

func TestCoroutineSurvivesReload(t *testing.T) {
	b := openBrain(t)
	b.pf.step(10)
	m := b.pf.app.Scripts.Machine()
	beat := b.value("beat")
	if !m.CoRunning(beat) {
		t.Fatal("Beat is not running")
	}
	canceled := b.pf.app.Scripts.CoStats().Canceled
	b.reload("yield wait(0.1)", "yield wait(0.05)")
	b.pf.step(2)
	if got := b.pf.app.Scripts.CoStats().Canceled; got != canceled+1 {
		t.Fatalf("canceled %d, want %d", got, canceled+1)
	}
	if after := b.value("beat"); after == beat || !m.CoRunning(after) {
		t.Fatal("Reloaded did not restart Beat")
	}
	b.pf.noFaults(t)
}

func TestPlatformerDrawsSprites(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(3)
	if st := pf.app.Runner.Stats(); st.Views < 1 || st.Sprites < 1 || st.MissingAssets != 0 {
		t.Fatalf("runner stats = %+v", st)
	}
}

func TestPlatformerSteadyFrameAllocs(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(240)
	d := pf.app.Driver()
	if n := testing.AllocsPerRun(120, func() { pf.app.Engine.Step(d) }); n >= 1 {
		t.Fatalf("%v allocations per steady frame", n)
	}
}

func BenchmarkPlatformerFrame(b *testing.B) {
	pf := openPlatformer(b)
	pf.step(240)
	d := pf.app.Driver()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pf.app.Engine.Step(d)
	}
}

// BenchmarkCastBody sweeps the player's colliders a little down and right on the platformer, as Motion 2D does every step.
func BenchmarkCastBody(b *testing.B) {
	pf := openPlatformer(b)
	pf.step(60)
	q, ok := engine.ServiceOf[physics2d.Queries](pf.app.Engine.MainWorld(), physics2d.ServiceID)
	if !ok {
		b.Fatal("no physics2d queries")
	}
	f := physics2d.QueryFilter{LayerMask: ^uint32(0), ExcludeBody: pf.player}
	delta := mathx.Vec2{X: 0.1, Y: 0.2}
	if _, ok := q.CastBody(pf.player, mathx.Vec2{}, delta, f); !ok {
		b.Fatal("the cast hits nothing, the player should stand on the ground")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.CastBody(pf.player, mathx.Vec2{}, delta, f)
	}
}

func TestPlatformerEnemySpeedFromData(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(30)
	if vx := math.Abs(float64(pf.body(pf.enemy).Velocity.X)); vx != 2.5 {
		t.Fatalf("enemy speed %v, want 2.5 from enemy_stats.aedata", vx)
	}
	pf.noFaults(t)
}

const enemyPrefab = "7a1e0c5b3d9f4a2e8c6b1d0f9e8a7c02"

func (pf *platformer) drive(from, to int) {
	for i := from; i < to; i++ {
		pf.hold(input.KeyD, i >= 40 && i < 120)
		pf.hold(input.KeyA, i >= 200 && i < 300)
		pf.hold(input.KeySpace, (i >= 130 && i < 134) || (i >= 320 && i < 324))
		pf.step(1)
	}
}

func (pf *platformer) physicsHash() [32]byte {
	return physics.SimOf(pf.app.Engine.MainWorld()).World().Snapshot().Hash()
}

func TestPlatformerReplayIsDeterministic(t *testing.T) {
	a := openPlatformer(t)
	a.drive(0, 200)
	h200, s200 := a.physicsHash(), string(sceneio.Save(a.app.Scene))
	a.drive(200, 400)
	h400, s400 := a.physicsHash(), string(sceneio.Save(a.app.Scene))
	a.noFaults(t)

	b := openPlatformer(t)
	b.drive(0, 200)
	if b.physicsHash() != h200 {
		t.Fatal("physics differs at frame 200 after replay")
	}
	if string(sceneio.Save(b.app.Scene)) != s200 {
		t.Fatal("scene differs at frame 200 after replay")
	}
	b.drive(200, 400)
	if b.physicsHash() != h400 {
		t.Fatal("physics differs at frame 400 after replay")
	}
	if string(sceneio.Save(b.app.Scene)) != s400 {
		t.Fatal("scene differs at frame 400 after replay")
	}
	for _, f := range []string{"ground", "kills", "jumpWait"} {
		if a.field(f) != b.field(f) {
			t.Fatalf("script field %s differs after replay", f)
		}
	}
}

func TestPlatformerSaveRoundTrip(t *testing.T) {
	pf := openPlatformer(t)
	st := save.NewStore(t.TempDir())
	pf.app.Engine.SetService(persistence.StoreID, st)
	sc := pf.app.Scene
	pf.step(30)
	pf.stomp(t)
	if pf.field("kills").Int() != 1 || sc.IsAlive(pf.enemy) {
		t.Fatal("the stomp did not kill EnemyLeft")
	}
	enemyID := sc.ID(pf.enemy)

	w := pf.app.Engine.MainWorld()
	prefab, _ := ids.ParseAssetID(enemyPrefab)
	spawned, err := spawn.Of(w).Spawn(prefab, scene.NodeHandle{}, mathx.Vec2{X: 2, Y: 2.9}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pid, ok := spawn.Of(w).PersistentID(spawned)
	if !ok {
		t.Fatal("the spawned enemy has no persistent id")
	}
	pf.step(5)
	hp, _ := scene.Get[gameplay.Health](sc, pf.player)
	hp.Current = 2
	sc.SetPosition(pf.player, mathx.Vec2{X: -2, Y: 1})
	at := sc.Local(spawned).Position

	svc := savegame.Of(w)
	doc, err := svc.Capture("round")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Write("round", save.Encode(doc), nil); err != nil {
		t.Fatal(err)
	}
	pf.step(30)
	if hp, _ := scene.Get[gameplay.Health](pf.app.Scene, pf.player); hp != nil {
		hp.Current = 3
	}
	before := pf.app.Scene
	if err := svc.Load("round"); err != nil {
		t.Fatal(err)
	}
	pf.step(2)

	sc = pf.app.Scene
	if sc == before {
		t.Fatal("the scene was not reloaded")
	}
	player := sc.Find(sc.Root(), "Player")
	if x := sc.Local(player).Position.X; math.Abs(float64(x+2)) > 1e-3 {
		t.Fatalf("player x = %v, want -2", x)
	}
	if h, _ := scene.Get[gameplay.Health](sc, player); h == nil || h.Current != 2 {
		t.Fatalf("health = %+v, want current 2", h)
	}
	if h := sc.Handle(enemyID); !h.IsZero() && sc.IsAlive(h) {
		t.Fatal("the killed enemy came back")
	}
	back, ok := spawn.Of(pf.app.Engine.MainWorld()).ByPersistentID(pid)
	if !ok {
		t.Fatal("the spawned enemy is not restored with its persistent id")
	}
	if p := sc.Local(back).Position; p.Sub(at).Len() > 0.5 {
		t.Fatalf("spawned enemy at %v, saved at %v", p, at)
	}
	id, ok := pf.app.Scripts.InstanceOf(player, playerType)
	if !ok {
		t.Fatal("the reloaded player has no script")
	}
	if k, _ := pf.app.Scripts.Field(id, "kills"); k.Int() != 1 {
		t.Fatalf("persisted kills = %d, want 1", k.Int())
	}
	pf.noFaults(t)
}
