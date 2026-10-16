//go:build !aego_runtime

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

func TestScriptMethodIsBound(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	reg := bind.New()
	fn := func(c *bind.Ctx, p *Probe, by int64) int64 {
		p.Count += by
		return p.Count
	}
	if _, err := module.Load(types, reg, scripting.New(stdlib.Options{}), probeModule{fn: fn}); err != nil {
		t.Fatal(err)
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

func TestReflectionFallbackWarns(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	man, err := module.Load(types, bind.New(), scripting.New(stdlib.Options{}), probeModule{fn: func(*bind.Ctx, *Probe) {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(man.Warnings) != 1 || !strings.Contains(man.Warnings[0], "reflection fallback") {
		t.Fatalf("warnings = %v", man.Warnings)
	}
}
