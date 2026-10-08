package sceneio_test

import (
	"strings"
	"testing"

	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
	"aego/schema"
)

const scriptScene = `scene version=1 {
  node "0123456789abcdef0123456789abcdef" name="A" enabled=true layers=1 {
    component "engine.Script" version=1 {
      script "game.OldMover" enabled=false {
        Velocity=321
        Extra=5
      }
      script "other.Missing" enabled=true {
        Kept=true
      }
    }
  }
}
`

func TestScriptAttachmentRoundTrip(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := components.Register(types); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	td, err := schema.NewScriptType("game.Mover", []string{"game.OldMover"}, []schema.FieldDescriptor{
		{Name: "Speed", Kind: schema.KindFloat32, Former: []string{"Velocity"}, Flags: schema.FlagSerialize | schema.FlagEditor},
		{Name: "Label", Kind: schema.KindString, Default: schema.String("hi"), HasDefault: true, Flags: schema.FlagSerialize | schema.FlagEditor},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().SetDynamic([]*schema.TypeDescriptor{td}); err != nil {
		t.Fatal(err)
	}
	s, rep, err := sceneio.Load([]byte(scriptScene), "t.aescene", types, nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) == 0 || len(rep.Unknown) == 0 {
		t.Fatal("unknown property and unknown script type were not reported")
	}
	h := s.PreOrder()[1]
	v, err := s.GetProperty(h, td.ID, td.Fields[0].ID)
	if err != nil || v.F != 321 {
		t.Fatalf("Speed through the overlay = %v, %v", v.F, err)
	}
	out := string(sceneio.Save(s))
	for _, want := range []string{
		`script "game.Mover" enabled=false`, `Speed=321`, `Label="hi"`, `Extra=5`,
		`script "other.Missing" enabled=true`, `Kept=true`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("saved scene misses %q:\n%s", want, out)
		}
	}
	again, _, err := sceneio.Load([]byte(out), "t2.aescene", types, nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(sceneio.Save(again)) != out {
		t.Fatal("second save is not canonical")
	}
}
