package integration

import (
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/core/mathx"
	"aego/modules/physics/components"
	"aego/platform"
	"aego/platform/input"
	"aego/project"
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

type combatModule struct{}

func (combatModule) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "mygame", Version: "0.1.0"}
}

func (combatModule) Register(r *sdk.Registry) error {
	if err := sdk.RegisterComponent[Health](r, sdk.Name("game.Health")); err != nil {
		return err
	}
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

func openExample(t testing.TB, name string, mods ...sdk.Module) (*run.App, string) {
	t.Helper()
	root := copyExample(t, name)
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	app, err := run.Open(p, run.Config{Headless: true, FrameTime: 1.0 / 60, Logger: log.Nop(), ScenePath: filepath.Join(root, "scenes", "main.aescene")}, mods...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app, root
}

type platformer struct {
	app    *run.App
	root   string
	player scene.NodeHandle
	enemy  scene.NodeHandle
	pid    vm.InstanceID
}

func openPlatformer(t testing.TB) *platformer {
	t.Helper()
	app, root := openExample(t, "platformer", combatModule{})
	sc := app.Scene
	pf := &platformer{app: app, root: root, player: sc.Find(sc.Root(), "Player"), enemy: sc.Find(sc.Root(), "EnemyLeft")}
	id, ok := app.Scripts.InstanceOf(pf.player, playerType)
	if !ok {
		t.Fatal("player has no PlayerController instance")
	}
	pf.pid = id
	return pf
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
	pf.step(2)
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
