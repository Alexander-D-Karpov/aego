package animation_test

import (
	"slices"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/engine"
	"aego/modules/animation"
	"aego/modules/gameplay"
	"aego/scene/frame"
	"aego/script/bind"
)

func TestFixedOrder(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := builtin.Default().Load(types, bind.New(), assembly.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := engine.NewBuilder(engine.Config{Headless: true})
	if err := frame.Register(b.Registry()); err != nil {
		t.Fatal(err)
	}
	b.Use(man.Engine())
	reg := b.Registry()
	if err := reg.Freeze(); err != nil {
		t.Fatal(err)
	}
	order := reg.Systems().Order(engine.PhasePrePhysics)
	at := func(id string) int {
		i := slices.Index(order, id)
		if i < 0 {
			t.Fatalf("%s is not in PrePhysics: %v", id, order)
		}
		return i
	}
	fsm, intents, anim, motion, move := at(gameplay.SystemFSMFixed), at(engine.AnchorIntents), at(animation.SystemFixed), at(engine.AnchorMotion), at(gameplay.SystemMotion)
	if !(fsm < intents && intents < anim && anim < motion && motion < move) {
		t.Fatalf("PrePhysics order %v", order)
	}
}
