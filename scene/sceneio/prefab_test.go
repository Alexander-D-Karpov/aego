package sceneio

import (
	"strings"
	"testing"

	"aego/core/ids"
	"aego/scene"
	"aego/schema"
)

type pfHealth struct {
	Max    float32       `aego:"max"`
	Target scene.NodeRef `aego:"target"`
}

type pfTag struct {
	Label string `aego:"label"`
}

const (
	enemyPrefab = "f0000000000000000000000000000001"
	squadPrefab = "f0000000000000000000000000000002"
	enemyRoot   = "a0000000000000000000000000000001"
	enemyStomp  = "a0000000000000000000000000000002"
	sceneRoot   = "b0000000000000000000000000000001"
	sceneStomp  = "b0000000000000000000000000000002"
	squadRoot   = "d0000000000000000000000000000001"
)

const enemySrc = `prefab version=1 {
  node "a0000000000000000000000000000001" name="Enemy" enabled=true layers=1 {
    transform {
      position=[0, 0]
      rotation=0
      scale=[1, 1]
    }
    component "test.Health" version=1 {
      max=5
      target=node("a0000000000000000000000000000002")
    }
    node "a0000000000000000000000000000002" name="Stomp" enabled=true layers=1 {
      transform {
        position=[0, -1]
        rotation=0
        scale=[1, 1]
      }
    }
  }
}
`

const enemyNoStompSrc = `prefab version=1 {
  node "a0000000000000000000000000000001" name="Enemy" enabled=true layers=1 {
    transform {
      position=[0, 0]
      rotation=0
      scale=[1, 1]
    }
    component "test.Health" version=1 {
      max=6
    }
  }
}
`

const squadSrc = `prefab version=1 {
  node "c0000000000000000000000000000001" name="Squad" enabled=true layers=1 {
    transform {
      position=[0, 0]
      rotation=0
      scale=[1, 1]
    }
    node "c0000000000000000000000000000002" prefab=asset("f0000000000000000000000000000001") {
      override "a0000000000000000000000000000001" prop="transform.position" value=[2, 0] {}
    }
  }
}
`

const enemyCycleSrc = `prefab version=1 {
  node "a0000000000000000000000000000001" name="Enemy" enabled=true layers=1 {
    transform {
      position=[0, 0]
      rotation=0
      scale=[1, 1]
    }
    node "e0000000000000000000000000000001" prefab=asset("f0000000000000000000000000000002") {}
  }
}
`

const instanceSrc = `scene version=1 {
  node "b0000000000000000000000000000001" prefab=asset("f0000000000000000000000000000001") {
    id "a0000000000000000000000000000001" node=node("b0000000000000000000000000000001") {}
    id "a0000000000000000000000000000002" node=node("b0000000000000000000000000000002") {}
    override "a0000000000000000000000000000001" prop="test.Health.max" value=7 {}
    override "a0000000000000000000000000000001" prop="transform.position" value=[3, 0] {}
  }
}
`

const squadSceneSrc = `scene version=1 {
  node "d0000000000000000000000000000001" prefab=asset("f0000000000000000000000000000002") {
    id "c0000000000000000000000000000001" node=node("d0000000000000000000000000000001") {}
  }
}
`

type fixture struct {
	t     *testing.T
	types *scene.Types
	lib   *Library
}

func nodeID(t *testing.T, s string) ids.NodeID {
	t.Helper()
	id, err := ids.ParseNodeID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assetID(t *testing.T, s string) ids.AssetID {
	t.Helper()
	id, err := ids.ParseAssetID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newFixture(t *testing.T, prefabs map[string]string) *fixture {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := scene.RegisterComponent[pfHealth](types, schema.Name("test.Health")); err != nil {
		t.Fatal(err)
	}
	if err := scene.RegisterComponent[pfTag](types, schema.Name("test.Tag")); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, types: types, lib: NewLibrary(types.Schema(), nil)}
	for id, src := range prefabs {
		f.setPrefab(id, src)
	}
	return f
}

func (f *fixture) setPrefab(id, src string) {
	f.t.Helper()
	p, err := ParsePrefab([]byte(src), id, f.types.Schema())
	if err != nil {
		f.t.Fatal(err)
	}
	f.lib.Set(assetID(f.t, id), p)
}

func (f *fixture) load(data string) (*scene.Scene, *Report) {
	f.t.Helper()
	s, rep, err := Load([]byte(data), "t.aescene", f.types, nil, LoadOptions{KeepIDs: true, Prefabs: f.lib})
	if err != nil {
		f.t.Fatal(err)
	}
	return s, rep
}

func (f *fixture) save(s *scene.Scene) string {
	return string(SaveWith(s, f.lib))
}

func (f *fixture) stable(s *scene.Scene) string {
	f.t.Helper()
	out := f.save(s)
	back, _ := f.load(out)
	if again := f.save(back); again != out {
		f.t.Fatalf("save is not stable:\n%s\n---\n%s", out, again)
	}
	return out
}

func mustContain(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q in:\n%s", w, out)
		}
	}
}

func TestInstanceRoundTrip(t *testing.T) {
	f := newFixture(t, map[string]string{enemyPrefab: enemySrc})
	s, rep := f.load(instanceSrc)
	if len(rep.Warnings) != 0 || len(rep.Unresolved) != 0 {
		t.Fatalf("report = %+v", rep)
	}
	root := s.Handle(nodeID(t, sceneRoot))
	stomp := s.Handle(nodeID(t, sceneStomp))
	if root.IsZero() || stomp.IsZero() || s.Parent(stomp) != root || s.Name(stomp) != "Stomp" {
		t.Fatal("instance nodes must keep their scene ids")
	}
	h, ok := scene.Get[pfHealth](s, root)
	if !ok || h.Max != 7 || s.Local(root).Position.X != 3 {
		t.Fatalf("overrides not applied: %+v", h)
	}
	if got, ok := h.Target.Resolve(s); !ok || got != stomp {
		t.Fatal("internal reference must point at the instance node")
	}
	if pi, ok := InstanceOf(s, root); !ok || !pi.Owner || pi.Keys[nodeID(t, sceneStomp)] == "" {
		t.Fatalf("instance metadata = %+v", pi)
	}
	out := f.stable(s)
	mustContain(t, out, `prefab=asset("`+enemyPrefab+`")`, `prop="test.Health.max" value=7`, `prop="transform.position" value=[3, 0]`)
	if strings.Contains(out, `name="Stomp"`) || strings.Contains(out, "component ") {
		t.Fatalf("instance was flattened:\n%s", out)
	}
}

func TestOverridesFollowEdits(t *testing.T) {
	f := newFixture(t, map[string]string{enemyPrefab: enemySrc})
	s, _ := f.load(instanceSrc)
	root := s.Handle(nodeID(t, sceneRoot))
	stomp := s.Handle(nodeID(t, sceneStomp))
	h, _ := scene.Get[pfHealth](s, root)
	h.Max = 9
	mustContain(t, f.save(s), `prop="test.Health.max" value=9`)
	h.Max = 5
	if out := f.save(s); strings.Contains(out, "test.Health.max") {
		t.Fatalf("value equal to the prefab must not be an override:\n%s", out)
	}
	s.SetName(stomp, "Feet")
	mustContain(t, f.stable(s), `override "`+enemyStomp+`" prop="node.name" value="Feet"`)
}

func TestPrefabChangeReachesClosedScene(t *testing.T) {
	f := newFixture(t, map[string]string{enemyPrefab: enemySrc})
	s, _ := f.load(instanceSrc)
	s.SetName(s.Handle(nodeID(t, sceneStomp)), "Feet")
	out := f.save(s)
	f.setPrefab(enemyPrefab, enemyNoStompSrc)
	s2, rep := f.load(out)
	root := s2.Handle(nodeID(t, sceneRoot))
	if !s2.Child(root, "Feet").IsZero() || s2.ChildCount(root) != 0 {
		t.Fatal("node removed from the prefab must leave the instance")
	}
	if h, _ := scene.Get[pfHealth](s2, root); h.Max != 7 {
		t.Fatal("instance override must survive the prefab change")
	}
	if len(rep.Warnings) == 0 {
		t.Fatal("lost targets must be reported")
	}
	out2 := f.stable(s2)
	mustContain(t, out2, `prop="node.name" value="Feet" orphan=true`)
	f.setPrefab(enemyPrefab, enemySrc)
	s3, _ := f.load(out2)
	if s3.Child(s3.Handle(nodeID(t, sceneRoot)), "Stomp").IsZero() {
		t.Fatal("restored prefab node must come back, orphans are not applied")
	}
}

func TestMissingPrefabKeepsBlock(t *testing.T) {
	f := newFixture(t, nil)
	s, rep := f.load(instanceSrc)
	if len(rep.Missing) != 1 || rep.Missing[0].Prefab != assetID(t, enemyPrefab) {
		t.Fatalf("missing = %+v", rep.Missing)
	}
	root := s.Handle(nodeID(t, sceneRoot))
	if root.IsZero() || s.Name(root) != "Missing prefab" {
		t.Fatal("missing prefab must leave a placeholder")
	}
	if _, ok := scene.Get[pfHealth](s, root); ok {
		t.Fatal("placeholder must be empty")
	}
	out := f.save(s)
	mustContain(t, out, `prop="test.Health.max" value=7`, `id "`+enemyStomp+`" node=node("`+sceneStomp+`")`)
	f.setPrefab(enemyPrefab, enemySrc)
	s2, _ := f.load(out)
	if h, ok := scene.Get[pfHealth](s2, s2.Handle(nodeID(t, sceneRoot))); !ok || h.Max != 7 {
		t.Fatal("instance must come back once the prefab exists")
	}
}

func TestAddedNodesComponentsAndOrder(t *testing.T) {
	f := newFixture(t, map[string]string{enemyPrefab: enemySrc})
	s, _ := f.load(instanceSrc)
	root := s.Handle(nodeID(t, sceneRoot))
	stomp := s.Handle(nodeID(t, sceneStomp))
	extra := s.Create(root, "Extra")
	scene.Add[pfTag](s, extra).Label = "x"
	scene.Add[pfTag](s, stomp).Label = "t"
	s.Flush()
	out := f.stable(s)
	mustContain(t, out, `addNode "`+enemyRoot+`"`, `addComponent "`+enemyStomp+`"`)
	s2, _ := f.load(out)
	root2 := s2.Handle(nodeID(t, sceneRoot))
	e2 := s2.Child(root2, "Extra")
	if tag, ok := scene.Get[pfTag](s2, e2); !ok || tag.Label != "x" || s2.ID(e2) != s.ID(extra) {
		t.Fatal("added node lost")
	}
	if tag, ok := scene.Get[pfTag](s2, s2.Handle(nodeID(t, sceneStomp))); !ok || tag.Label != "t" {
		t.Fatal("added component lost")
	}
	if err := s2.Reparent(e2, root2, 0); err != nil {
		t.Fatal(err)
	}
	s2.Flush()
	out2 := f.stable(s2)
	mustContain(t, out2, `order "`+enemyRoot+`"`)
	s3, _ := f.load(out2)
	root3 := s3.Handle(nodeID(t, sceneRoot))
	if s3.Name(s3.FirstChild(root3)) != "Extra" {
		t.Fatal("child order lost")
	}
	s3.Destroy(s3.Handle(nodeID(t, sceneStomp)))
	s3.Flush()
	out3 := f.stable(s3)
	mustContain(t, out3, `remove "`+enemyStomp+`"`)
	s4, _ := f.load(out3)
	if !s4.Child(s4.Handle(nodeID(t, sceneRoot)), "Stomp").IsZero() {
		t.Fatal("removed node came back")
	}
}

func TestTemplateInstantiate(t *testing.T) {
	f := newFixture(t, map[string]string{enemyPrefab: enemySrc})
	tmpl, err := f.lib.Template(assetID(t, enemyPrefab))
	if err != nil {
		t.Fatal(err)
	}
	s := scene.New(f.types, nil, scene.Options{})
	a, err := Instantiate(s, scene.NodeHandle{}, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Instantiate(s, scene.NodeHandle{}, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	s.Flush()
	if s.ID(a) == s.ID(b) {
		t.Fatal("every copy needs fresh ids")
	}
	for _, r := range []scene.NodeHandle{a, b} {
		stomp := s.Child(r, "Stomp")
		h, ok := scene.Get[pfHealth](s, r)
		if got, rok := h.Target.Resolve(s); !ok || !rok || got != stomp {
			t.Fatal("internal reference must point into the same copy")
		}
		if root, ok := PrefabRoot(s, stomp); !ok || root != r {
			t.Fatal("prefab root of a child must be its copy")
		}
		if pi, _ := InstanceOf(s, r); pi.Owner {
			t.Fatal("runtime copy must not own overrides")
		}
	}
	mustContain(t, f.save(s), `instanceOf=asset("`+enemyPrefab+`")`)
}

func TestNestedPrefabAndCycle(t *testing.T) {
	f := newFixture(t, map[string]string{enemyPrefab: enemySrc, squadPrefab: squadSrc})
	s, _ := f.load(squadSceneSrc)
	root := s.Handle(nodeID(t, squadRoot))
	enemy := s.Child(root, "Enemy")
	stomp := s.Child(enemy, "Stomp")
	if s.Name(root) != "Squad" || enemy.IsZero() || stomp.IsZero() || s.Local(enemy).Position.X != 2 {
		t.Fatal("nested prefab not expanded")
	}
	if h, _ := scene.Get[pfHealth](s, enemy); func() bool { got, ok := h.Target.Resolve(s); return !ok || got != stomp }() {
		t.Fatal("nested reference must point into the nested copy")
	}
	if pi, ok := InstanceOf(s, enemy); !ok || !pi.Nested {
		t.Fatal("nested root must be marked")
	}
	if r, _ := PrefabRoot(s, stomp); r != enemy {
		t.Fatal("prefab root of a nested child is the nested root")
	}
	s.SetName(stomp, "Feet")
	mustContain(t, f.stable(s), `override "c0000000000000000000000000000002/`+enemyStomp+`" prop="node.name" value="Feet"`)
	f.setPrefab(enemyPrefab, enemyCycleSrc)
	_, rep := f.load(squadSceneSrc)
	if len(rep.Missing) != 1 {
		t.Fatal("a prefab cycle must load as a missing instance")
	}
}
