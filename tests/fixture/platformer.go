package fixture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/sdk"
)

type health struct {
	Max     float32 `aego:"max,min=1"`
	Current float32 `aego:"current,min=0"`
	Hits    int64   `aego:"hits"`
}

type enemyStats struct {
	Speed    float32     `aego:"speed,min=0,max=10"`
	Cooldown float32     `aego:"cooldown,min=0,max=5"`
	Damage   int64       `aego:"damage,min=0"`
	Tint     mathx.Color `aego:"tint"`
}

type Platformer struct{}

func (Platformer) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "mygame", Version: "0.1.0"}
}

func (Platformer) Register(r *sdk.Registry) error {
	if err := sdk.Component[health](r, sdk.Name("game.Health"), sdk.Persist("current", "hits")).Err(); err != nil {
		return err
	}
	if err := sdk.DataType[enemyStats](r, sdk.Name("mygame.EnemyStats")).Err(); err != nil {
		return err
	}
	m := r.ScriptModule("mygame/combat")
	sdk.Proc2(m, "Damage", func(c *sdk.ScriptCtx, target sdk.NodeHandle, amount int64) {
		if h, ok := sdk.Get[health](c.Scene, target); ok {
			h.Current -= float32(amount)
		}
	})
	sdk.Func1(m, "HealthOf", func(c *sdk.ScriptCtx, target sdk.NodeHandle) float32 {
		if h, ok := sdk.Get[health](c.Scene, target); ok {
			return h.Current
		}
		return 0
	})
	return nil
}

func Copy(t testing.TB, name string) string {
	t.Helper()
	src := filepath.Join("..", "..", "examples", name)
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
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
