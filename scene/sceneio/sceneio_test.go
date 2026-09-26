package sceneio

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
)

type Health struct {
	Max     float32 `aego:"max"`
	Current float32 `aego:"current"`
}

type Marker struct {
	Target scene.NodeRef `aego:"target"`
	Label  string        `aego:"label"`
	Tint   mathx.Color   `aego:"tint"`
}

func newRegistry(t *testing.T) *scene.Types {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[Health](types, schema.Name("test.Health")); err != nil {
		t.Fatal(err)
	}
	if err := scene.RegisterComponent[Marker](types, schema.Name("test.Marker")); err != nil {
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

func opts() LoadOptions {
	return LoadOptions{KeepIDs: true}
}

func TestLoadBasicScene(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "tests", "scenes", "basic.aescene"))
	if err != nil {
		t.Fatal(err)
	}
	s, report, err := Load(src, "basic.aescene", newRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unknown) != 0 || len(report.Warnings) != 0 {
		t.Fatalf("report = %+v", report)
	}
	player := s.Find(s.Root(), "Player")
	if player.IsZero() {
		t.Fatal("player node missing")
	}
	if got := s.Local(player).Position; !got.NearlyEqual(mathx.Vec2{X: 100, Y: 200}, 1e-4) {
		t.Fatalf("position = %v", got)
	}
	h, ok := scene.Get[Health](s, player)
	if !ok || h.Max != 100 || h.Current != 72.5 {
		t.Fatalf("health = %+v ok=%v", h, ok)
	}
	if s.Find(s.Root(), "Player/Weapon").IsZero() {
		t.Fatal("child node missing")
	}
	if s.ChildCount(s.Root()) != 2 {
		t.Fatalf("root children = %d", s.ChildCount(s.Root()))
	}
}

func TestSaveIsByteIdenticalAfterLoad(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "scenes", "basic.aescene")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := Load(src, "basic.aescene", newRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	out := Save(s)
	if string(out) != string(src) {
		t.Fatal(firstDiff(src, out))
	}
}

func firstDiff(want, got []byte) string {
	n := min(len(want), len(got))
	for i := 0; i < n; i++ {
		if want[i] != got[i] {
			lo := max(0, i-40)
			hiW := min(len(want), i+40)
			hiG := min(len(got), i+40)
			return fmt.Sprintf("byte %d: want %q got %q\nwant ...%q...\ngot  ...%q...",
				i, want[i], got[i], want[lo:hiW], got[lo:hiG])
		}
	}
	if len(want) != len(got) {
		return fmt.Sprintf("lengths differ: want %d got %d", len(want), len(got))
	}
	return "identical"
}

func TestLoadSaveLoadIsStable(t *testing.T) {
	src, _ := os.ReadFile(filepath.Join("..", "..", "tests", "scenes", "basic.aescene"))
	first, _, err := Load(src, "a", newRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	saved := Save(first)
	second, _, err := Load(saved, "b", newRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if string(Save(second)) != string(saved) {
		t.Fatal("second round trip differs")
	}
}

func TestUnknownComponentSurvives(t *testing.T) {
	src := []byte(`scene version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="Player" enabled=true layers=1 {
    transform {
      position=[0, 0]
      rotation=0
      scale=[1, 1]
    }
    component "game.Rotator" version=3 {
      speed=2.5
      axis="z"
    }
  }
}
`)
	s, report, err := Load(src, "x.aescene", newRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unknown) != 1 || report.Unknown[0] != "game.Rotator" {
		t.Fatalf("unknown = %v", report.Unknown)
	}
	out := Save(s)
	if string(out) != string(src) {
		t.Fatalf("unknown component was not preserved:\n%s", out)
	}
}

func TestStrictRejectsUnknown(t *testing.T) {
	src := []byte(`scene version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="A" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    component "game.Nope" version=1 {}
  }
}
`)
	o := opts()
	o.Strict = true
	if _, _, err := Load(src, "x", newRegistry(t), nil, o); !errs.Has(err, errs.SceneIOStrict) {
		t.Fatalf("err = %v", err)
	}
}

func TestNodeRefResolvesAndSurvivesWhenDangling(t *testing.T) {
	target := ids.NewNodeID()
	src := []byte(`scene version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="A" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    component "test.Marker" version=1 {
      target=node("b17e5a3c9d2f4e6a8b0c1d2e3f405162")
      label=""
      tint=#00000000
    }
  }
  node "b17e5a3c9d2f4e6a8b0c1d2e3f405162" name="B" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
  }
}
`)
	s, report, err := Load(src, "x", newRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unresolved) != 0 {
		t.Fatalf("unresolved = %+v", report.Unresolved)
	}
	a := s.Find(s.Root(), "A")
	m, ok := scene.Get[Marker](s, a)
	if !ok {
		t.Fatal("marker missing")
	}
	b := s.Find(s.Root(), "B")
	if m.Target.ID != s.ID(b) {
		t.Fatal("node reference did not resolve")
	}

	dangling := []byte(`scene version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="A" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    component "test.Marker" version=1 {
      target=node("` + target.String() + `")
      label=""
      tint=#00000000
    }
  }
}
`)
	s2, report2, err := Load(dangling, "y", newRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report2.Unresolved) != 1 {
		t.Fatalf("a dangling reference must be reported: %+v", report2.Unresolved)
	}
	out := Save(s2)
	if !contains(out, target.String()) {
		t.Fatal("a dangling node id must survive a save")
	}
}

func contains(haystack []byte, needle string) bool {
	return len(needle) > 0 && string(haystack) != "" && indexOf(string(haystack), needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestMigrationV0ToV1(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "tests", "scenes", "v0.aescene"))
	if err != nil {
		t.Fatal(err)
	}
	s, report, err := Load(src, "v0.aescene", newRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) == 0 {
		t.Fatal("migration must be reported")
	}
	player := s.Find(s.Root(), "Player")
	if got := s.Local(player).Position; !got.NearlyEqual(mathx.Vec2{X: 100, Y: 200}, 1e-4) {
		t.Fatalf("migrated position = %v", got)
	}
	out := Save(s)
	if !contains(out, "position=[100, 200]") || contains(out, "pos=[") {
		t.Fatalf("saved document still uses the old field:\n%s", out)
	}
	if !contains(out, "version=1") {
		t.Fatal("version was not bumped")
	}
}

func TestBrokenFileLeavesSceneUntouched(t *testing.T) {
	reg := newRegistry(t)
	good, _ := os.ReadFile(filepath.Join("..", "..", "tests", "scenes", "basic.aescene"))
	s, _, err := Load(good, "good", reg, nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	before := Save(s)

	if _, _, err := Load([]byte("scene version=1 {"), "broken", reg, nil, opts()); err == nil {
		t.Fatal("a broken document must fail to load")
	}
	if string(Save(s)) != string(before) {
		t.Fatal("a failed load touched the existing scene")
	}
	if _, _, err := Load([]byte("nonsense {}"), "wrong-root", reg, nil, opts()); !errs.Has(err, errs.SceneIOParse) {
		t.Fatal("wrong document root must be rejected")
	}
}

func TestLoadIntoInstantiatesWithFreshIDs(t *testing.T) {
	reg := newRegistry(t)
	src := []byte(`prefab version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="Turret" enabled=true layers=1 {
    transform { position=[1, 2] rotation=0 scale=[1, 1] }
    component "test.Health" version=1 { max=50 current=50 }
  }
}
`)
	s := scene.New(reg, nil, scene.Options{})
	parent := s.Create(s.Root(), "Holder")
	s.Flush()

	created, _, err := LoadInto(s, parent, src, "turret.aeprefab", LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 {
		t.Fatalf("created = %d nodes", len(created))
	}
	if s.Parent(created[0]) != parent {
		t.Fatal("prefab was not attached to the requested parent")
	}
	if s.ID(created[0]).String() == "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" {
		t.Fatal("instantiation must generate fresh node ids")
	}
	h, ok := scene.Get[Health](s, created[0])
	if !ok || h.Max != 50 {
		t.Fatal("prefab component data lost")
	}

	again, _, err := LoadInto(s, parent, src, "turret.aeprefab", LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if again[0] == created[0] {
		t.Fatal("second instantiation must create a new node")
	}
	if s.ChildCount(parent) != 2 {
		t.Fatalf("parent children = %d", s.ChildCount(parent))
	}
}
