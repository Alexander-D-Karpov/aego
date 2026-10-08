package session

import (
	"testing"

	"aego/core/ids"
	"aego/editor/state"
)

func TestHandoffRoundTrip(t *testing.T) {
	root := t.TempDir()
	if _, ok := Take(root); ok {
		t.Fatal("empty session must not yield a handoff")
	}
	id := ids.NewNodeID()
	st := state.Default()
	st.Active = "scenes/a.aescene"
	st.Docs = []state.Doc{{Path: "scenes/a.aescene", Selection: []ids.NodeID{id}}, {Path: "scenes/new.aescene"}}
	h := &Handoff{State: st, Scenes: map[string][]byte{"scenes/new.aescene": []byte("scene version=1 {\n}\n")}}
	if err := RequestRestart(root); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, h); err != nil {
		t.Fatal(err)
	}
	if RestartRequested(root) {
		t.Fatal("writing a handoff must clear the restart request")
	}
	back, ok := Take(root)
	if !ok {
		t.Fatal("handoff lost")
	}
	if back.State.Active != st.Active || len(back.State.Docs) != 2 || back.State.Docs[0].Selection[0] != id {
		t.Fatalf("state = %+v", back.State)
	}
	if string(back.Scenes["scenes/new.aescene"]) != string(h.Scenes["scenes/new.aescene"]) {
		t.Fatal("scene data lost")
	}
	if _, again := Take(root); again {
		t.Fatal("handoff must be consumed once")
	}
}

func TestStatus(t *testing.T) {
	root := t.TempDir()
	if _, ok := ReadStatus(root); ok {
		t.Fatal("status without a build")
	}
	if err := WriteStatus(root, false, "main.go:3: undefined: x\nmore"); err != nil {
		t.Fatal(err)
	}
	st, ok := ReadStatus(root)
	if !ok || st.OK || st.Log != "main.go:3: undefined: x\nmore" || st.Time.IsZero() {
		t.Fatalf("status = %+v", st)
	}
}
