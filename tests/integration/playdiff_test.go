package integration

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/gizmo"
	pd "aego/editor/core/playdiff"
	"aego/modules/gameplay"
	"aego/scene"
	"aego/schema"
)

type playDiffRig struct {
	edit, play *document.SceneDocument
	enemy      ids.NodeID
	hitbox     *schema.TypeDescriptor
	area       *schema.FieldDescriptor
}

func playDiffSession(t *testing.T) (*playDiffRig, func(int)) {
	t.Helper()
	s, pc := rewindSession(t)
	m := s.App.Context().Docs
	play := m.PlayDoc()
	if play == nil {
		t.Fatal("no play document")
	}
	edit := m.Get(play.ID)
	ps := play.Scene()
	hb, ok := ps.Registry().ByName(gameplay.NameHitbox)
	if !ok {
		t.Fatal("Hitbox 2D is not registered")
	}
	area, _ := hb.Field("area")
	enemy := ps.Find(ps.Root(), "EnemyLeft")
	if !scene.Has[gameplay.Hitbox2D](ps, enemy) {
		t.Fatal("EnemyLeft has no Hitbox 2D")
	}
	tick := func(n int) {
		for range n {
			pc.Tick(1.0 / 60)
		}
	}
	return &playDiffRig{edit: edit, play: play, enemy: ps.ID(enemy), hitbox: hb, area: area}, tick
}

// gizmoArea drags the enemy hitbox's Area handle in two steps, as a gizmo drag does.
func (r *playDiffRig) gizmoArea(t *testing.T, w float32) mathx.Rect {
	t.Helper()
	ps := r.play.Scene()
	cur, _ := ps.GetProperty(ps.Handle(r.enemy), r.hitbox.ID, r.area.ID)
	rect := cur.R
	tx := r.play.History.Begin("Edit Area")
	e := &gizmo.EditCtx{Scene: ps, Tx: tx, Node: r.enemy}
	for _, k := range []float32{0.5, 1} {
		next := rect
		next.W = rect.W + (w-rect.W)*k
		gizmo.SetProperty(e, r.hitbox.ID, r.area, schema.Rect(next))
	}
	if e.Err != nil {
		tx.Abort()
		t.Fatal(e.Err)
	}
	tx.Commit()
	rect.W = w
	return rect
}

func (r *playDiffRig) setBounce(t *testing.T) {
	t.Helper()
	ps := r.play.Scene()
	td, ok := ps.Registry().ByName(playerType)
	if !ok {
		t.Fatal("PlayerController is not registered")
	}
	fd, _ := td.Field("Bounce")
	player := ps.ID(ps.Find(ps.Root(), "Player"))
	if err := r.play.Props.Set(nil, []ids.NodeID{player}, td.ID, fd.ID, schema.Vec2(mathx.Vec2{X: 0, Y: -14})); err != nil {
		t.Fatal(err)
	}
}

func TestPlayDiffListsOnlyYourEdits(t *testing.T) {
	r, tick := playDiffSession(t)
	tick(300)
	r.setBounce(t)
	want := r.gizmoArea(t, 3)
	ps := r.play.Scene()
	from := ps.Local(ps.Handle(r.enemy)).Position
	tick(60)
	if ps.Local(ps.Handle(r.enemy)).Position == from {
		t.Fatal("the enemy did not move after the gizmo edit")
	}
	hist := pd.FromHistory(r.edit, r.play)
	if len(hist.Changes) != 2 {
		for _, c := range hist.Changes {
			t.Logf("row %s %s.%s", c.Name, c.Owner, c.Label)
		}
		t.Fatalf("history rows = %d, want Bounce and Area", len(hist.Changes))
	}
	var area *pd.Change
	for i := range hist.Changes {
		c := &hist.Changes[i]
		if !c.Checked || c.Gameplay {
			t.Fatalf("row %s.%s should be checked and yours", c.Owner, c.Label)
		}
		if c.Type == r.hitbox.ID {
			area = c
		}
	}
	if area == nil || area.After.R != want || area.Name != "EnemyLeft" {
		t.Fatalf("area row %+v, want the gizmo value %v on EnemyLeft", area, want)
	}
	full := pd.Full(r.edit, r.play, hist)
	game := 0
	for _, c := range full.Changes {
		if c.Gameplay {
			if c.Checked {
				t.Fatalf("game row %s.%s is checked", c.Owner, c.Label)
			}
			if c.Node == r.enemy && c.Kind == pd.KindTransform2D {
				game++
			}
		}
	}
	if game != 1 {
		t.Fatalf("the enemy's position is not among the game rows (%d)", game)
	}
}

func TestPlayDiffEmptyWithoutEdits(t *testing.T) {
	r, tick := playDiffSession(t)
	tick(300)
	if hist := pd.FromHistory(r.edit, r.play); !hist.Empty() || len(hist.Created)+len(hist.Removed) != 0 {
		t.Fatalf("a run with no edits lists %+v", hist)
	}
}

func TestPlayDiffDialogOnlyAfterEdits(t *testing.T) {
	for _, edited := range []bool{false, true} {
		s, pc := rewindSession(t)
		for range 120 {
			pc.Tick(1.0 / 60)
		}
		if edited {
			m := s.App.Context().Docs
			play := m.PlayDoc()
			r := &playDiffRig{edit: m.Get(play.ID), play: play}
			r.setBounce(t)
		}
		pc.Stop()
		s.Step()
		if got := s.App.PlayDiffOpen(); got != edited {
			t.Fatalf("edited=%v: play changes dialog open=%v", edited, got)
		}
	}
}
