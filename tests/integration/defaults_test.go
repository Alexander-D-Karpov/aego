package integration

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/mathx"
	"aego/modules/physics/components"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/script/bind"
)

func TestMissingFieldsTakeDefaults(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{Seed: 1}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	src := `scene version=1 {
  node "d0000000000000000000000000000001" name="Box" enabled=true layers=1 {
    component "physics.Body2D" version=1 {
    }
    component "physics.BoxCollider2D" version=1 {
      size=[2, 3]
    }
  }
}
`
	s, _, err := sceneio.Load([]byte(src), "hand.aescene", types, nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Find(s.Root(), "Box")
	b, _ := scene.Get[components.Body2D](s, h)
	if b.Type != components.Dynamic || b.GravityScale != 1 || b.Mask != -1 || !b.AllowSleep {
		t.Fatalf("body %+v", *b)
	}
	c, _ := scene.Get[components.BoxCollider2D](s, h)
	if c.Size != (mathx.Vec2{X: 2, Y: 3}) || c.Density != 1 || c.Friction != 0.6 {
		t.Fatalf("box %+v", *c)
	}
}

func TestScriptVectorDefaultPublished(t *testing.T) {
	pf := openPlatformer(t)
	td, ok := pf.app.Types.Schema().ByName(playerType)
	if !ok {
		t.Fatal("PlayerController is not published")
	}
	fd, ok := td.Field("JumpImpulse")
	if !ok || !fd.HasDefault || fd.Default.V != (mathx.Vec2{Y: -5}) {
		t.Fatalf("JumpImpulse default %+v", fd)
	}
	if v := pf.field("JumpImpulse"); v.Lane(1) != -5 {
		t.Fatalf("instance JumpImpulse lane y = %v", v.Lane(1))
	}
}
