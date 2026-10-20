//go:build aego_editor

package mutate_test

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"unsafe"

	"aego/core/ids"
	"aego/editor/core/clipboard"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
)

func platformer(t *testing.T) (*scene.Scene, *history.History) {
	t.Helper()
	e := env(t)
	src, err := os.ReadFile(sceneFile)
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := sceneio.Load(src, "platformer", e.Types, nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	return s, history.New(target{s})
}

func nodeID(t *testing.T, s *scene.Scene, name string) ids.NodeID {
	t.Helper()
	h := s.Find(s.Root(), name)
	if h.IsZero() {
		t.Fatalf("no node %s", name)
	}
	return s.ID(h)
}

func floatField(t *testing.T, s *scene.Scene, comp, name string) (*schema.TypeDescriptor, *schema.FieldDescriptor) {
	t.Helper()
	td, ok := s.Registry().ByName(comp)
	if !ok {
		t.Fatalf("no component %s", comp)
	}
	fd, ok := td.Field(name)
	if !ok {
		t.Fatalf("no field %s.%s", comp, name)
	}
	return td, fd
}

func undoRedo(t *testing.T, s *scene.Scene, h *history.History, before []byte) {
	t.Helper()
	after := sceneio.Save(s)
	if err := h.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := sceneio.Save(s); !bytes.Equal(got, before) {
		t.Fatalf("undo did not restore the scene:\n%s", got)
	}
	if err := h.Redo(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sceneio.Save(s), after) {
		t.Fatal("redo differs from the first do")
	}
}

func TestPasteTwoRoots(t *testing.T) {
	s, h := platformer(t)
	before := sceneio.Save(s)
	data, tops, err := clipboard.Fragment(s, []ids.NodeID{nodeID(t, s, "Player"), nodeID(t, s, "Floor")})
	if err != nil {
		t.Fatal(err)
	}
	data, err = clipboard.Remap(data)
	if err != nil {
		t.Fatal(err)
	}
	frags, err := commands.Fragments(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 2 {
		t.Fatalf("%d fragments for two roots", len(frags))
	}
	const index = 1
	cmd := commands.NewMutation("Create Paste", commands.EnvFor(s),
		commands.Insert(ids.NodeID{}, index, frags[0]), commands.Insert(ids.NodeID{}, index+1, frags[1]))
	if err := h.Exec(cmd); err != nil {
		t.Fatal(err)
	}
	created := cmd.Created()
	if len(created) != 2 {
		t.Fatalf("created %d nodes", len(created))
	}
	for i, id := range created {
		n := s.Handle(id)
		if !strings.HasPrefix(s.Name(n), s.Name(tops[i])) || commands.ChildIndex(s, n) != index+i || s.Parent(n) != s.Root() {
			t.Fatalf("root %d: %s at %d", i, s.Name(n), commands.ChildIndex(s, n))
		}
	}
	undoRedo(t, s, h, before)
}

func TestRemoveScriptUndoKeepsAttachments(t *testing.T) {
	s, h := platformer(t)
	before := sceneio.Save(s)
	player := nodeID(t, s, "Player")
	if _, ok := scriptcomp.ScriptsOf(s, s.Handle(player)); !ok {
		t.Fatal("the player has no scripts")
	}
	td, _ := s.Registry().ByID(scriptcomp.ScriptTypeID)
	if err := h.Exec(commands.NewMutation("Remove "+td.FullName, commands.EnvFor(s), commands.RemoveComp(player, td))); err != nil {
		t.Fatal(err)
	}
	if _, ok := scriptcomp.ScriptsOf(s, s.Handle(player)); ok {
		t.Fatal("the script component is still there")
	}
	undoRedo(t, s, h, before)
}

func TestOnAddRunsOnce(t *testing.T) {
	s, h := platformer(t)
	n := s.Create(s.Root(), "Fresh")
	s.Flush()
	id := s.ID(n)
	before := sceneio.Save(s)
	td, fd := floatField(t, s, "physics.Body2D", "gravityScale")
	calls := 0
	env := commands.WithOnAdd(commands.EnvFor(s), func(got *schema.TypeDescriptor) func(*scene.Scene, scene.NodeHandle) {
		if got != td {
			return nil
		}
		return func(s *scene.Scene, h scene.NodeHandle) {
			calls++
			p, _ := s.GetByType(h, td.ID)
			fd.Set(p, schema.Float32(7))
		}
	})
	if err := h.Exec(commands.NewMutation("Add "+td.FullName, env, commands.AddComp(s, id, td, nil))); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || gravity(t, s, id, td, fd) != 7 {
		t.Fatalf("calls %d, value %v", calls, gravity(t, s, id, td, fd))
	}
	undoRedo(t, s, h, before)
	if calls != 1 || gravity(t, s, id, td, fd) != 7 {
		t.Fatalf("after redo: calls %d, value %v", calls, gravity(t, s, id, td, fd))
	}
}

func gravity(t *testing.T, s *scene.Scene, id ids.NodeID, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) float64 {
	t.Helper()
	v, err := s.GetProperty(s.Handle(id), td.ID, fd.ID)
	if err != nil {
		t.Fatal(err)
	}
	return v.F
}

func TestResetComponent(t *testing.T) {
	s, h := platformer(t)
	player := nodeID(t, s, "Player")
	td, fd := floatField(t, s, "physics.Body2D", "gravityScale")
	if err := h.Exec(commands.SetField(s, player, td, fd, schema.Float32(2.5), fd.Name)); err != nil {
		t.Fatal(err)
	}
	before := sceneio.Save(s)
	env := commands.EnvFor(s)
	tx := h.Begin("Reset " + td.FullName)
	if err := tx.Exec(commands.NewMutation("Remove "+td.FullName, env, commands.RemoveComp(player, td))); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(commands.NewMutation("Add "+td.FullName, env, commands.AddComp(s, player, td, nil))); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	var want schema.Value
	fd.Get(unsafe.Pointer(td.Default()), &want)
	if got := gravity(t, s, player, td, fd); got != want.F {
		t.Fatalf("reset left %v, default is %v", got, want.F)
	}
	undoRedo(t, s, h, before)
}
