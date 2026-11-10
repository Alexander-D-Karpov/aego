package integration

import (
	"slices"
	"sync"
	"testing"
	"time"

	"aego/app/agentui/backend"
	"aego/app/edit"
	"aego/core/log"
	"aego/editor/core/agentui"
	"aego/editor/core/tasks"
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

// Allocation budgets measure a settled editor, not background preview or
// agent-availability work.
func editorSettled(s *edit.Session) bool {
	panel := s.App.Agent()
	return s.App.Idle() && !s.App.Previews().Busy() &&
		(panel == nil || (panel.Checked() && !panel.Busy()))
}

func waitEditorSettled(t *testing.T, s *edit.Session) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	stable := 0
	for stable < 10 {
		s.Step()
		if editorSettled(s) {
			stable++
		} else {
			stable = 0
		}
		if time.Now().After(deadline) {
			t.Fatal("editor or agent did not settle before measuring allocations")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type delayedAvailabilityBackend struct {
	backend.Fake
	started chan struct{}
	release chan struct{}
	start   sync.Once
	finish  sync.Once
}

func (b *delayedAvailabilityBackend) Available(backend.Config) (string, error) {
	b.start.Do(func() { close(b.started) })
	<-b.release
	return "fake", nil
}

func (b *delayedAvailabilityBackend) unblock() {
	b.finish.Do(func() { close(b.release) })
}

func TestAgentEditorIdleWaitsForAvailability(t *testing.T) {
	b := &delayedAvailabilityBackend{started: make(chan struct{}), release: make(chan struct{})}
	defer b.unblock()
	s, _ := agentSession(t, b)
	stepUntil(t, s, func() bool {
		select {
		case <-b.started:
			return s.App.Idle() && !s.App.Previews().Busy()
		default:
			return false
		}
	})
	if editorSettled(s) || s.App.Agent().Checked() {
		t.Fatal("editor was considered idle while agent availability was still running")
	}
	b.unblock()
	waitEditorSettled(t, s)
	if !s.App.Agent().Checked() || s.App.Agent().Missing() != nil {
		t.Fatal("agent availability did not settle successfully")
	}
}

func TestEditorSettledWaitsPastFixedFrameWarmup(t *testing.T) {
	s := workspaceSession(t)
	steps := 0
	s.App.Tasks().Step(tasks.Spec{Title: "delayed test task"}, tasks.LaneMain, func(*tasks.Task) (bool, error) {
		steps++
		return steps >= 180, nil
	})
	for range 60 {
		s.Step()
	}
	if !s.App.Tasks().Busy() || editorSettled(s) {
		t.Fatal("a fixed 60-frame warmup did not leave the delayed task active")
	}
	waitEditorSettled(t, s)
	if s.App.Tasks().Busy() || steps < 180 {
		t.Fatal("the editor was considered settled before the delayed task finished")
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
		waitEditorSettled(t, s)
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
