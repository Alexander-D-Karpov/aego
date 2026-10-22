package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/edit"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/project"
	"aego/schema"
)

// The Inspector writes script fields through a mutation, and Undo restores them.
func TestInspectorSetsScriptField(t *testing.T) {
	s, _ := rewindSession(t)
	m := s.App.Context().Docs
	d := m.Get(m.PlayDoc().ID)
	sc := d.Scene()
	td, _ := sc.Registry().ByName(playerType)
	fd, _ := td.Field("Bounce")
	player := sc.ID(sc.Find(sc.Root(), "Player"))
	before, err := d.Props.Get(player, td.ID, fd.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := mathx.Vec2{X: 1, Y: -3}
	if err := d.Props.Set(nil, []ids.NodeID{player}, td.ID, fd.ID, schema.Vec2(want)); err != nil {
		t.Fatal(err)
	}
	if v, _ := d.Props.Get(player, td.ID, fd.ID); v.V != want {
		t.Fatalf("Bounce = %v, want %v", v.V, want)
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if v, _ := d.Props.Get(player, td.ID, fd.ID); v.V != before.V {
		t.Fatalf("Undo left Bounce at %v, want %v", v.V, before.V)
	}
}

// A script field set on a prefab instance is saved as an override and survives Undo and Redo.
func TestScriptFieldOverrideSaves(t *testing.T) {
	root := copyExample(t, "platformer")
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	m := s.App.Context().Docs
	d, err := m.Open("scenes/main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	sc := d.Scene()
	td, ok := sc.Registry().ByName("mygame.enemy.EnemyAI")
	if !ok {
		t.Fatal("EnemyAI is not registered")
	}
	fd, _ := td.Field("PointA")
	enemy := sc.ID(sc.Find(sc.Root(), "EnemyLeft"))
	target := sc.ID(sc.Find(sc.Root(), "Checkpoint"))
	if target.IsZero() {
		t.Fatal("no Checkpoint node")
	}
	if err := d.Props.Set(nil, []ids.NodeID{enemy}, td.ID, fd.ID, schema.Node(target)); err != nil {
		t.Fatal(err)
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := d.History.Redo(); err != nil {
		t.Fatal(err)
	}
	if v, _ := d.Props.Get(enemy, td.ID, fd.ID); v.Node != target {
		t.Fatalf("PointA after Undo and Redo = %v", v.Node)
	}
	if err := m.Save(d.ID); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(root, "scenes", "main.aescene"))
	if err != nil {
		t.Fatal(err)
	}
	want := `prop="script/mygame.enemy.EnemyAI.PointA" value=node("` + target.String() + `")`
	if !strings.Contains(string(src), want) {
		t.Fatalf("saved scene lacks %s", want)
	}
}
