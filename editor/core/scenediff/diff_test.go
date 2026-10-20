package scenediff

import (
	"testing"
)

const before = `scene version=1 {
  node "a" name="Player" {
    component "game.Stats" version=1 {
      speed = 2
      hp = 10
    }
    node "c" name="Gun" {
    }
  }
  node "b" name="Enemy" {
    component "engine.Script" version=1 {
      script "game.Spinner" enabled=true {
        rate = 1
      }
    }
  }
}
`

const after = `scene version=1 {
  node "a" name="Hero" {
    component "game.Stats" version=1 {
      speed = 3
      hp = 10
    }
    component "game.Tag" version=1 {
    }
  }
  node "b" name="Enemy" {
    component "engine.Script" version=1 {
      script "game.Spinner" enabled=true {
        rate = 4
      }
    }
    node "c" name="Gun" {
    }
  }
  node "d" name="Coin" {
  }
}
`

func find(r Result, k Kind, node string) *Change {
	for i := range r.Changes {
		if r.Changes[i].Kind == k && r.Changes[i].Node == node {
			return &r.Changes[i]
		}
	}
	return nil
}

func TestSceneDiff(t *testing.T) {
	r, err := Compute([]byte(before), []byte(after), "main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Scene {
		t.Fatal("a scene file must be diffed as a tree")
	}
	if c := find(r, NodeRenamed, "a"); c == nil || c.Before != "Player" || c.After != "Hero" {
		t.Fatalf("rename: %+v", c)
	}
	if c := find(r, FieldChanged, "a"); c == nil || c.Owner != "game.Stats" || c.Field != "speed" || c.Before != "2" || c.After != "3" {
		t.Fatalf("field: %+v", c)
	}
	if find(r, CompAdded, "a") == nil {
		t.Fatal("added component is missing")
	}
	if c := find(r, NodeMoved, "c"); c == nil || c.After != "Enemy" {
		t.Fatalf("move: %+v", c)
	}
	if find(r, NodeAdded, "d") == nil {
		t.Fatal("added node is missing")
	}
	if c := find(r, FieldChanged, "b"); c == nil || c.Field != "rate" || c.Owner != "engine.Script › script game.Spinner" {
		t.Fatalf("script field: %+v", c)
	}
	for _, c := range r.Changes {
		if c.Field == "hp" {
			t.Fatal("unchanged fields must not be listed")
		}
	}
}

func TestNewFileIsAllAdded(t *testing.T) {
	r, err := Compute(nil, []byte(after), "main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range r.Changes {
		if c.Kind == NodeAdded {
			n++
		}
	}
	if n != 4 {
		t.Fatalf("added %d nodes, want 4", n)
	}
}

func TestFlatDiff(t *testing.T) {
	a := "data type=\"game.Weapon\" {\n  damage = 3\n  name = \"Axe\"\n}\n"
	b := "data type=\"game.Weapon\" {\n  damage = 5\n  name = \"Axe\"\n  rate = 2\n}\n"
	r, err := Compute([]byte(a), []byte(b), "axe.aedata")
	if err != nil {
		t.Fatal(err)
	}
	if r.Scene || len(r.Changes) != 2 {
		t.Fatalf("changes = %+v", r.Changes)
	}
}
