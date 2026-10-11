package builtin_test

import (
	"bytes"
	"strings"
	"testing"

	"aego/engine/builtin"
	"aego/engine/module"
	"aego/schema"
	"aego/script/bind"
	"aego/script/bind/manifest"
)

type nativeModule struct {
	reg      func(*bind.Module)
	requires []string
}

func (n nativeModule) Info() module.Info {
	return module.Info{ID: "game", Requires: n.requires}
}

func (n nativeModule) Register(r *module.Registry) error {
	if n.reg != nil {
		n.reg(r.ScriptModule("game/combat"))
	}
	return nil
}

func TestBuiltinRoundTrip(t *testing.T) {
	m, err := builtin.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	data := manifest.Encode(m)
	back, err := manifest.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.ABIHash != m.ABIHash || !bytes.Equal(manifest.Encode(back), data) {
		t.Fatal("manifest is not stable across a round trip")
	}
	if _, ok := back.Component("engine.SpriteRenderer"); !ok {
		t.Fatal("builtin components are missing")
	}
	if _, ok := back.FuncDoc("aego/math", "Clamp"); !ok {
		t.Fatal("stdlib functions are missing")
	}
}

func native(t *testing.T, reg func(*bind.Module)) *manifest.Manifest {
	t.Helper()
	m, err := builtin.Manifest(nativeModule{reg: reg})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Table(); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNativeDocsAndLocation(t *testing.T) {
	m := native(t, func(mod *bind.Module) {
		bind.Proc2(mod, "Damage", func(*bind.Ctx, int64, float32) {}, bind.Doc("deals damage"))
	})
	f, ok := m.FuncDoc("game/combat", "Damage")
	if !ok || f.Doc != "deals damage" || !strings.Contains(f.Loc, "builtin_test.go") {
		t.Fatalf("func = %+v", f)
	}
	if got := f.Sig.String(); got != "(int, float)" {
		t.Fatalf("signature = %s", got)
	}
}

func TestSignatureChangeChangesHash(t *testing.T) {
	a := native(t, func(mod *bind.Module) { bind.Proc2(mod, "Damage", func(*bind.Ctx, int64, float32) {}) })
	b := native(t, func(mod *bind.Module) { bind.Proc2(mod, "Damage", func(*bind.Ctx, int64, string) {}) })
	if a.ABIHash == b.ABIHash {
		t.Fatal("abi hash ignores a native signature change")
	}
}

func TestRequiresMissingModule(t *testing.T) {
	_, err := builtin.Manifest(nativeModule{requires: []string{"physics"}})
	if err == nil || !strings.Contains(err.Error(), "game requires physics") {
		t.Fatalf("err = %v", err)
	}
}

type scoped struct {
	Speed float32 `aego:"speed"`
	Spins int64   `aego:"spins"`
}

type scopedModule struct{}

func (scopedModule) Info() module.Info {
	return module.Info{ID: "game"}
}

func (scopedModule) Register(r *module.Registry) error {
	return module.Component[scoped](r, schema.Name("game.Scoped")).Script("Speed").Err()
}

func TestScriptFieldsLimitBindings(t *testing.T) {
	m, err := builtin.Manifest(scopedModule{})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := m.Component("game.Scoped")
	if !ok || len(c.Fields) != 1 || c.Fields[0].Name != "Speed" {
		t.Fatalf("component = %+v", c)
	}
}
