package native

import "aego/sdk"

type Health struct {
	Max     float32 `aego:"max,min=1"`
	Current float32 `aego:"current,min=0"`
	Hits    int64   `aego:"hits"`
}

type Module struct{}

func (Module) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "mygame", Version: "0.1.0"}
}

func (Module) Register(r *sdk.Registry) error {
	if err := sdk.RegisterComponent[Health](r, sdk.Name("game.Health"), sdk.Doc("hit points of a character")); err != nil {
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
		if c.Scene == nil {
			return 0
		}
		if h, ok := sdk.Get[Health](c.Scene, target); ok {
			return h.Current
		}
		return 0
	})
	return nil
}

func Modules() []sdk.Module {
	return []sdk.Module{Module{}}
}
