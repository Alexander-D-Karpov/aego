package physics_test

import (
	"testing"

	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/physics"
	"aego/scene/frame"
	"aego/script/bind"
)

// The physics module builds an engine without the scripting module.
func TestBuildWithoutScripting(t *testing.T) {
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := module.Load(types, bind.New(), base.Core{}, physics.New())
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
