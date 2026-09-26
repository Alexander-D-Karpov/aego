package sceneio

import (
	"testing"

	"aego/scene"
	"aego/schema"
)

type Spinner struct {
	Speed float32 `aego:"speed"`
	Axis  string  `aego:"axis"`
}

const legacyScene = `scene version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="Player" enabled=true layers=1 {
    transform {
      position=[0, 0]
      rotation=0
      scale=[1, 1]
    }
    component "game.Rotator" version=1 {
      rate=2.5
      axis="z"
    }
  }
}
`

func renamedTypes(t *testing.T) *scene.Types {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[Spinner](types,
		schema.Name("game.Spinner"),
		schema.FormerName("game.Rotator"),
		schema.FormerField("speed", "rate"),
	); err != nil {
		t.Fatal(err)
	}
	if err := RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types
}

func TestRenamedComponentLoadsAndSavesUnderNewName(t *testing.T) {
	s, report, err := Load([]byte(legacyScene), "legacy.aescene", renamedTypes(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unknown) != 0 {
		t.Fatalf("former name was treated as unknown: %v", report.Unknown)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("former field name produced warnings: %+v", report.Warnings)
	}
	player := s.Find(s.Root(), "Player")
	sp, ok := scene.Get[Spinner](s, player)
	if !ok {
		t.Fatal("component did not load under its new type")
	}
	if sp.Speed != 2.5 {
		t.Fatalf("former field did not map onto the new one: speed=%v", sp.Speed)
	}
	if sp.Axis != "z" {
		t.Fatalf("unchanged field lost: axis=%q", sp.Axis)
	}

	out := Save(s)
	if contains(out, "game.Rotator") {
		t.Fatalf("save kept the former type name:\n%s", out)
	}
	if !contains(out, `component "game.Spinner"`) {
		t.Fatalf("save did not use the new type name:\n%s", out)
	}
	if contains(out, "rate=") || !contains(out, "speed=2.5") {
		t.Fatalf("save did not migrate the field name:\n%s", out)
	}

	again, _, err := Load(out, "renamed.aescene", renamedTypes(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if string(Save(again)) != string(out) {
		t.Fatal("second round trip after a rename is not stable")
	}
}

func TestFormerNameWithoutRegistrationStaysUnknown(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s, report, err := Load([]byte(legacyScene), "legacy.aescene", types, nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unknown) != 1 {
		t.Fatalf("unknown = %v", report.Unknown)
	}
	if string(Save(s)) != legacyScene {
		t.Fatalf("unknown component was not preserved verbatim:\n%s", Save(s))
	}
}
