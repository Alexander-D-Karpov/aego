package templates

import (
	"testing"

	"aego/scene"
)

func TestModulesSplitTheMenu(t *testing.T) {
	r := New()
	build := func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle { return s.Create(parent, "X") }
	r.Register(Template{ID: "engine.empty", Name: "Empty", Builtin: true, Build: build})
	r.Register(Template{ID: "render3d.cube", Name: "Cube", Builtin: true, Space: scene.Space3D, Build: build})
	r.Register(Template{ID: "game.enemy", Name: "Enemy", Module: "game", Category: "Actors", Build: build})
	r.Register(Template{ID: "game.coin", Name: "Coin", Module: "game", Category: "Pickups", Build: build})
	r.Register(Template{ID: "fx.spark", Name: "Spark", Module: "fx", Build: build})
	secs := Layout(r, Place{})
	if len(secs) != 4 || len(secs[0].Items) != 1 || secs[1].Title != Category3D || secs[2].Title != "fx" || secs[3].Title != "game" {
		t.Fatalf("sections = %+v", secs)
	}
	if g := secs[3]; len(g.Items) != 2 || g.Items[0].ID != "game.enemy" || len(g.Breaks) != 1 {
		t.Fatalf("game = %+v", g)
	}
}
