package scripting_test

import (
	"testing"

	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/scripting"
	"aego/scene/frame"
	"aego/script/bind"
	"aego/script/stdlib"
)

// The scripting module builds an engine without the physics module.
func TestBuildWithoutPhysics(t *testing.T) {
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := module.Load(types, bind.New(), base.Core{}, scripting.New(stdlib.Options{}))
	if err != nil {
		t.Fatal(err)
	}
	b := engine.NewBuilder(engine.Config{Headless: true})
	frame.Register(b.Registry())
	b.Use(man.Engine())
	e, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	e.Close()
}
