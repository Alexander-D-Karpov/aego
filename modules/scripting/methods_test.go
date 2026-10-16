package scripting_test

import (
	"strings"
	"testing"

	"aego/engine/module"
	"aego/modules/scripting"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
	"aego/script/stdlib"
)

type Probe struct {
	Count int64 `aego:"count"`
}

type probeModule struct {
	fn any
}

func (probeModule) Info() module.Info {
	return module.Info{ID: "test"}
}

func (m probeModule) Register(r *module.Registry) error {
	return module.Component[Probe](r, schema.Name("test.Probe")).ScriptMethod("Bump", m.fn).Err()
}

func build(fn any) error {
	types := scene.NewTypes(schema.NewRegistry())
	_, err := module.Load(types, bind.New(), scripting.New(stdlib.Options{}), probeModule{fn: fn})
	return err
}

func TestBadScriptMethodNamesOwner(t *testing.T) {
	err := build(func(p *Probe) {})
	if err == nil || !strings.Contains(err.Error(), "of module test") {
		t.Fatalf("err = %v", err)
	}
}

type typedModule struct{}

func (typedModule) Info() module.Info {
	return module.Info{ID: "test"}
}

func (typedModule) Register(r *module.Registry) error {
	return module.Component[Probe](r, schema.Name("test.Probe")).
		Method1("Bump", func(_ *bind.Ctx, p *Probe, by int64) int64 {
			p.Count += by
			return p.Count
		}).
		Err()
}

func TestTypedMethodIsBoundWithoutWarning(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	reg := bind.New()
	man, err := module.Load(types, reg, scripting.New(stdlib.Options{}), typedModule{})
	if err != nil {
		t.Fatal(err)
	}
	if len(man.Warnings) != 0 {
		t.Fatalf("warnings = %v", man.Warnings)
	}
	found := false
	for _, f := range reg.Table().ComponentMethodList() {
		if f.Comp == "test.Probe" && f.Name == "Bump" && f.Sig.String() == "(int) int" {
			found = true
		}
	}
	if reg.Table().Module("test/probe") {
		t.Fatal("component methods must not create an importable module")
	}
	if !found {
		t.Fatal("Bump is not in the bindings table")
	}
}
