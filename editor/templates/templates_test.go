package templates

import (
	"testing"

	"aego/scene"
)

func TestModulesSplitTheMenu(t *testing.T) {
	r := New()
	if err := RegisterBuiltin(r); err != nil {
		t.Fatal(err)
	}
	build := func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle { return s.Create(parent, "X") }
	r.Register(Template{ID: "game.enemy", Name: "Enemy", Module: "game", Category: "Actors", Build: build})
	r.Register(Template{ID: "game.coin", Name: "Coin", Module: "game", Category: "Pickups", Build: build})
	r.Register(Template{ID: "fx.spark", Name: "Spark", Module: "fx", Build: build})

	root := r.Root()
	if len(root) != 9 {
		t.Fatalf("root = %d templates", len(root))
	}
	for _, tpl := range root {
		if tpl.Module != "engine" {
			t.Fatalf("non-builtin %s in root", tpl.ID)
		}
	}
	mods := r.Modules()
	if len(mods) != 2 || mods[0] != "fx" || mods[1] != "game" {
		t.Fatalf("modules = %v", mods)
	}
	game := r.InModule("game")
	if len(game) != 2 || game[0].ID != "game.enemy" || game[1].ID != "game.coin" {
		t.Fatalf("game = %+v", game)
	}
}
