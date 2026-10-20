package integration

import (
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

type Health struct {
	Max     float32 `aego:"max,min=1"`
	Current float32 `aego:"current,min=0"`
	Hits    int64   `aego:"hits"`
}

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

var healthHit sdk.Signal1[int64]

func (combatModule) Register(r *sdk.Registry) error {
	b := sdk.Component[Health](r, sdk.Name("game.Health"), sdk.Persist("current", "hits"))
	healthHit = b.Signal1[int64]("Hit", "Hit fires when Damage lands; hits is the new hit count.", "hits")
	if err := b.Err(); err != nil {
		return err
	}
	if err := sdk.DataType[EnemyStats](r, sdk.Name("mygame.EnemyStats")).Err(); err != nil {
		return err
	}
	sdk.Live[EnemyStats](r, "speed")
	m := sdk.ScriptModule(r, "mygame/combat")
	sdk.Proc2(m, "Damage", func(c *sdk.ScriptCtx, target sdk.NodeHandle, amount int64) {
		if c.Scene == nil || !c.Scene.IsAlive(target) {
			c.Fail("damage target is nil or destroyed")
			return
		}
		h, ok := sdk.Get[Health](c.Scene, target)
		if !ok {
			return
		}
		h.Current -= float32(amount)
		h.Hits++
		if h.Current <= 0 {
			h.Current = h.Max
		}
		healthHit.Emit(c.World, target, h.Hits)
	})
	sdk.Func1(m, "HealthOf", func(c *sdk.ScriptCtx, target sdk.NodeHandle) float32 {
		if h, ok := sdk.Get[Health](c.Scene, target); ok {
			return h.Current
		}
		return 0
	})
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

func (pf *platformer) hitPlayer(t testing.TB) {
	t.Helper()
	sc := pf.app.Scene
	for i := 0; i < 240 && pf.health().Hits == 0; i++ {
		e := sc.Local(pf.enemy).Position
		sc.SetPosition(pf.player, mathx.Vec2{X: e.X - 0.74, Y: e.Y - 0.05})
		pf.step(1)
	}
	if pf.health().Hits == 0 {
		t.Fatal("the player was never hit")
	}
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

func (pf *platformer) health() Health {
	h, _ := scene.Get[Health](pf.app.Scene, pf.player)
	return *h
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
	if vy := pf.body(pf.player).Velocity.Y; math.Abs(float64(vy)) > 0.05 {
		t.Fatalf("player still moves vertically: %v", vy)
	}
	if y := pf.y(); y < 2.9 || y > 3.05 {
		t.Fatalf("player rests at y=%v, want about 3", y)
	}
	if pf.field("ground").Int() == 0 {
		t.Fatal("OnCollisionEnter never reported the floor")
	}
	pf.noFaults(t)
}

func TestPlatformerJump(t *testing.T) {
	pf := openPlatformer(t)
	pf.step(60)
	y0 := pf.y()
	pf.hold(input.KeySpace, true)
	pf.step(3)
	pf.hold(input.KeySpace, false)
	top := y0
	for i := 0; i < 60; i++ {
		pf.step(1)
		top = min(top, pf.y())
	}
	if y0-top < 1.5 {
		t.Fatalf("jump rose %v units, want at least 1.5", y0-top)
	}
	pf.step(120)
	if pf.field("ground").Int() == 0 || math.Abs(float64(pf.y()-y0)) > 0.05 {
		t.Fatalf("player did not land: y=%v ground=%d", pf.y(), pf.field("ground").Int())
	}
	pf.noFaults(t)
}

func TestPlatformerStompEmitsDied(t *testing.T) {
	pf := openPlatformer(t)
	sc := pf.app.Scene
	pf.step(30)
	for i := 0; i < 120 && pf.field("kills").Int() == 0 && sc.IsAlive(pf.enemy); i++ {
		e := sc.Local(pf.enemy).Position
		sc.SetPosition(pf.player, mathx.Vec2{X: e.X, Y: e.Y - 1.3})
		pf.step(1)
	}
	pf.step(20)
	if k := pf.field("kills").Int(); k != 1 {
		t.Fatalf("kills %d", k)
	}
	if sc.IsAlive(pf.enemy) {
		t.Fatal("the stomped enemy is still alive")
	}
	pf.noFaults(t)
}

func TestPlatformerBlinkSurvivesReload(t *testing.T) {
	pf := openPlatformer(t)
	sc := pf.app.Scene
	pf.step(30)
	for i := 0; i < 240 && pf.health().Hits == 0; i++ {
		e := sc.Local(pf.enemy).Position
		sc.SetPosition(pf.player, mathx.Vec2{X: e.X - 0.74, Y: e.Y - 0.05})
		pf.step(1)
	}
	if pf.health().Hits == 0 {
		t.Fatal("the player was never hit")
	}
	sc.SetPosition(pf.player, mathx.Vec2{X: 13, Y: 3})
	pf.step(1)
	m := pf.app.Scripts.Machine()
	blink := pf.field("blink")
	if !m.CoRunning(blink) {
		t.Fatal("Blink is not running after a hit")
	}
	before := pf.health()
	canceled := pf.app.Scripts.CoStats().Canceled

	path := filepath.Join(pf.root, "scripts", "player", "player.aes")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), "yield wait(0.1)", "yield wait(0.05)", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	pf.app.Workspace.Changed([]string{path})
	if _, _, err := pf.app.Pipeline.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pf.app.Pipeline.ImportAll()
	if err != nil {
		t.Fatal(err)
	}
	if err := pf.app.Loader.Swap(batch.Imported, pf.app.Pipeline.Artifacts()); err != nil {
		t.Fatal(err)
	}
	pf.step(2)

	if got := pf.app.Scripts.CoStats().Canceled; got != canceled+1 {
		t.Fatalf("canceled %d, want %d", got, canceled+1)
	}
	after := pf.field("blink")
	if after == blink || !m.CoRunning(after) {
		t.Fatal("Reloaded did not restart Blink")
	}
	if pf.health() != before {
		t.Fatalf("health changed across reload: %+v → %+v", before, pf.health())
	}
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
	for i := 0; i < 120 && pf.field("kills").Int() == 0 && sc.IsAlive(pf.enemy); i++ {
		e := sc.Local(pf.enemy).Position
		sc.SetPosition(pf.player, mathx.Vec2{X: e.X, Y: e.Y - 1.3})
		pf.step(1)
	}
	pf.step(20)
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
	hp, _ := scene.Get[Health](sc, pf.player)
	hp.Current, hp.Hits = 2, 3
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
	if hp, _ := scene.Get[Health](pf.app.Scene, pf.player); hp != nil {
		hp.Current, hp.Hits = 5, 0
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
	if h, _ := scene.Get[Health](sc, player); h == nil || h.Current != 2 || h.Hits != 3 {
		t.Fatalf("health = %+v, want current 2 and hits 3", h)
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
