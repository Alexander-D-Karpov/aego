package integration

import (
	"slices"
	"testing"
	"time"

	"aego/app/agentui/backend"
	"aego/app/edit"
	"aego/core/log"
	"aego/editor/core/agentui"
	agentpanel "aego/editor/panels/agent"
	"aego/project"
)

func agentSession(t *testing.T, b backend.Backend) (*edit.Session, string) {
	t.Helper()
	root := copyExample(t, "platformer")
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop(), AgentBackend: b}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	if _, err := s.App.Context().Docs.Open(bridgeScene); err != nil {
		t.Fatal(err)
	}
	s.App.Workspace().Show(agentpanel.ID)
	return s, root
}

func stepUntil(t *testing.T, s *edit.Session, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		s.Step()
	}
}

// The panel shows the proposal as a card, Apply makes one Undo step, and the session log holds every kind of entry.
func TestAgentPanelAppliesACardAndLogsTheSession(t *testing.T) {
	fake := &backend.Fake{Turns: [][]backend.Step{{
		{Text: "Adding a crate."},
		{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "AgentCrate"}},
		{Text: "Done."},
	}}}
	s, root := agentSession(t, fake)
	panel := s.App.Agent()
	if panel == nil {
		t.Fatal("no agent panel")
	}
	panel.Submit("add a crate")
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 })
	panel.Decide(panel.Pending()[0], true)
	stepUntil(t, s, func() bool {
		d := s.App.Context().Docs.Edit()
		return len(d.History.Steps(nil)) == 1 && !d.Scene().Find(d.Scene().Root(), "AgentCrate").IsZero()
	})
	for range 30 {
		s.Step()
	}
	ids, err := agentui.List(root)
	if err != nil || len(ids) != 1 {
		t.Fatalf("sessions %v %v", ids, err)
	}
	entries, err := agentui.Load(root, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	var kinds, states []string
	for _, e := range entries {
		kinds = append(kinds, e.Kind)
		if e.Kind == "card" {
			states = append(states, e.State)
		}
	}
	for _, k := range []string{"user", "text", "tool", "result", "card"} {
		if !slices.Contains(kinds, k) {
			t.Fatalf("the log has no %s entry: %v", k, kinds)
		}
	}
	if !slices.Contains(states, "pending") || !slices.Contains(states, "applied") {
		t.Fatalf("card states %v", states)
	}
}

func TestAgentPanelExplainsAMissingClaude(t *testing.T) {
	s, _ := agentSession(t, backend.Claude{})
	panel := s.App.Agent()
	s.App.SetAgentCommand("/nonexistent/claude-for-aego-tests")
	stepUntil(t, s, panel.Checked)
	panel.Recheck()
	stepUntil(t, s, func() bool { return panel.Missing() != nil })
}

func TestAgentAskFillsTheInput(t *testing.T) {
	s, _ := agentSession(t, &backend.Fake{})
	panel := s.App.Agent()
	panel.Ask("In scenes/main.aescene, Player: ")
	s.Step()
	if panel.Input() != "In scenes/main.aescene, Player: " {
		t.Fatalf("input %q", panel.Input())
	}
}

// An idle Agent panel adds no allocations to a frame until the first message.
func TestAgentPanelIdleAllocs(t *testing.T) {
	skipUnderRace(t)
	measure := func(b backend.Backend) float64 {
		root := copyExample(t, "platformer")
		p, err := project.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop(), AgentBackend: b}, combatModule{})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		s.Step()
		if _, err := s.App.Context().Docs.Open(bridgeScene); err != nil {
			t.Fatal(err)
		}
		for range 60 {
			s.Step()
		}
		return testing.AllocsPerRun(30, func() { s.Step() })
	}
	without := measure(nil)
	with := measure(&backend.Fake{})
	if with > without {
		t.Fatalf("frame allocations: %v without the agent panel, %v with it idle", without, with)
	}
}

// Apply All takes the waiting change and the rest of the answer's, but leaves a removal to the user.
func TestAgentPanelApplyAllForTheTurn(t *testing.T) {
	fake := &backend.Fake{Turns: [][]backend.Step{{
		{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "AgentA"}},
		{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "AgentB"}},
		{Tool: "scene_remove_node", Args: map[string]any{"file": bridgeScene, "node": "AgentA"}},
		{Text: "done"},
	}}}
	s, _ := agentSession(t, fake)
	panel := s.App.Agent()
	panel.Submit("add two nodes")
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 })
	panel.ApplyAll()
	d := s.App.Context().Docs.Edit()
	stepUntil(t, s, func() bool { return len(d.History.Steps(nil)) == 2 && len(panel.Pending()) == 1 })
	for range 20 {
		s.Step()
	}
	if n := len(d.History.Steps(nil)); n != 2 {
		t.Fatalf("Apply All applied a removal: %d steps", n)
	}
	panel.Decide(panel.Pending()[0], false)
	stepUntil(t, s, func() bool { return !panel.Busy() })
}
