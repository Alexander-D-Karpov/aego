package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/project"
)

func TestGameAuthoringActionsAndStartupAreUndoable(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	e.b.SetAutoApply(true)
	steps := func(name string, args map[string]any) {
		res := results(e.turn(t, []backend.Step{{Tool: name, Args: args}}, nil))
		if len(res) != 1 || res[0].Failed {
			t.Fatalf("%s: %+v", name, res)
		}
	}
	settings := filepath.Join(e.root, project.FileName)
	before, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	read := func() string {
		b, e := os.ReadFile(settings)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	undo := func() {
		ps := e.b.Proposals()
		if err := e.b.Undo(ps[len(ps)-1].ID); err != nil {
			t.Fatal(err)
		}
	}
	steps("project_input_action", map[string]any{"name": "QuickRespawn", "type": "button", "context": "Gameplay", "active": true, "bind": []string{"Key.R"}})
	if !strings.Contains(read(), "QuickRespawn") {
		t.Fatal("new action missing")
	}
	undo()
	if string(before) != read() {
		t.Fatal("input Undo did not restore original settings")
	}
	steps("scene_new", map[string]any{"file": "scenes/finish.aescene"})
	steps("project_main_scene", map[string]any{"file": "scenes/finish.aescene"})
	if string(before) == read() {
		t.Fatal("startup did not change")
	}
	undo()
	if string(before) != read() {
		t.Fatal("startup Undo did not restore original scene")
	}
	if err := e.doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.root, "scenes/finish.aescene")); !os.IsNotExist(err) {
		t.Fatal("undo did not remove created scene")
	}
}
