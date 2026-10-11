package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/core/mathx"
	"aego/project"
	"aego/scene"
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

func copyPlatformer(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "examples", "platformer")
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

type platformer struct {
	app    *run.App
	root   string
	player scene.NodeHandle
	enemy  scene.NodeHandle
	pid    vm.InstanceID
}

func openPlatformer(t *testing.T) *platformer {
	t.Helper()
	root := copyPlatformer(t)
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	app, err := run.Open(p, run.Config{Headless: true, FrameTime: 1.0 / 60, Logger: log.Nop(), ScenePath: filepath.Join(root, "scenes", "main.aescene")}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
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

func TestPlatformerStompEmitsDied(t *testing.T) {
	pf := openPlatformer(t)
	sc := pf.app.Scene
	pf.step(5)
	for i := 0; i < 120 && pf.field("kills").Int() == 0 && sc.IsAlive(pf.enemy); i++ {
		sc.SetPosition(pf.player, mathx.Vec2{X: sc.Local(pf.enemy).Position.X, Y: 2.3})
		pf.step(1)
	}
	pf.step(1)
	if k := pf.field("kills").Int(); k != 1 {
		t.Fatalf("kills %d", k)
	}
	if sc.IsAlive(pf.enemy) {
		t.Fatal("the stomped enemy is still alive")
	}
}

func TestPlatformerBlinkSurvivesReload(t *testing.T) {
	pf := openPlatformer(t)
	sc := pf.app.Scene
	pf.step(5)
	for i := 0; i < 240 && pf.health().Hits == 0; i++ {
		sc.SetPosition(pf.player, mathx.Vec2{X: sc.Local(pf.enemy).Position.X, Y: 3})
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
