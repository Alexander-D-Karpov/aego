package document

import (
	"testing"

	"aego/scene"
)

func TestPrefabModeNavigation(t *testing.T) {
	m := newManager(t)
	main := m.New("scenes/main.aescene", scene.New(m.opt.Types, nil, scene.Options{}))
	enemy := m.New("prefabs/enemy"+PrefabExt, scene.New(m.opt.Types, nil, scene.Options{}))
	loose := m.New("prefabs/loose"+PrefabExt, scene.New(m.opt.Types, nil, scene.Options{}))

	m.remember(enemy, main.ID)
	if chain := m.Chain(enemy.ID); len(chain) != 2 || chain[0].ID != main.ID {
		t.Fatalf("chain must start at the scene, got %d entries", len(chain))
	}
	if src, ok := m.PlaySource(enemy.ID); !ok || src.ID != main.ID {
		t.Fatal("play from a prefab opened in a scene must run that scene")
	}
	enemy.forced = true
	if p := m.Pending(main.ID); len(p) != 1 || p[0].ID != enemy.ID {
		t.Fatal("a dirty prefab opened from the scene must be pending there")
	}
	enemy.forced = false

	m.noteBack(loose, enemy.ID)
	if _, ok := m.PlaySource(loose.ID); ok {
		t.Fatal("a prefab opened from Assets has no scene to play")
	}
	if chain := m.Chain(loose.ID); len(chain) != 1 {
		t.Fatal("a prefab opened from Assets shows only itself in the crumbs")
	}
	if b, ok := m.BackTarget(loose.ID); !ok || b != enemy.ID {
		t.Fatal("back must lead to the previously active document")
	}

	if enemy.Label() != "enemy (Prefab)" {
		t.Fatalf("label %q", enemy.Label())
	}
	m.SetActive(enemy.ID)
	if err := m.Close(enemy.ID, false); err != nil {
		t.Fatal(err)
	}
	if m.Edit() == nil || m.Edit().ID != main.ID {
		t.Fatal("closing the prefab must return to its scene")
	}
	if b, ok := m.BackTarget(loose.ID); !ok || b != main.ID {
		t.Fatal("back links must be re-pointed when a document in the middle closes")
	}
}
