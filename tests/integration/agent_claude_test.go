package integration

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"aego/app/agentui/backend"
	"aego/app/edit"
	"aego/app/edit/agentbridge"
	"aego/core/log"
	"aego/project"
)

func drainTurn(t *testing.T, s *edit.Session, conv backend.Conversation) []backend.Event {
	t.Helper()
	var out []backend.Event
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case e, ok := <-conv.Events():
			if !ok {
				return out
			}
			out = append(out, e)
			if e.Kind == backend.EvTurnEnd || e.Kind == backend.EvError {
				return out
			}
		default:
			s.Step()
		}
	}
	t.Fatal("the turn did not end")
	return nil
}

// The real Claude Code CLI reaches the editor's tools; it runs only with AEGO_AGENT_SMOKE=1 and claude in PATH, since it calls the model.
func TestClaudeCallsAnEditorTool(t *testing.T) {
	if os.Getenv("AEGO_AGENT_SMOKE") == "" {
		t.Skip("set AEGO_AGENT_SMOKE=1 to call the model")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude is not in PATH")
	}
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	ctx := s.App.Context()
	if _, err := ctx.Docs.Open(bridgeScene); err != nil {
		t.Fatal(err)
	}
	b, err := agentbridge.Start(agentbridge.Config{Ctx: ctx, OnMain: s.Host.Engine.Jobs().OnMain})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	var be backend.Claude
	if v, err := be.Available(b.Backend()); err != nil {
		t.Fatal(err)
	} else {
		t.Log("claude", v)
	}
	conv, err := be.Start(b.Backend())
	if err != nil {
		t.Fatal(err)
	}
	defer conv.Close()
	if err := conv.Send("Call the scene_tree tool once for scenes/main.aescene, then answer with the names of the top-level nodes only."); err != nil {
		t.Fatal(err)
	}
	called, answered := false, false
	for _, e := range drainTurn(t, s, conv) {
		switch e.Kind {
		case backend.EvError:
			t.Fatal(e.Err)
		case backend.EvToolStart:
			t.Logf("tool %s %s", e.Tool, e.Args)
			called = called || e.Tool == "scene_tree"
		case backend.EvToolResult:
			if e.Failed {
				t.Fatalf("the tool failed: %s", e.Result)
			}
		case backend.EvText:
			t.Logf("text %q", e.Text)
			answered = answered || strings.Contains(e.Text, "Player")
		}
	}
	if !called || !answered {
		t.Fatalf("called scene_tree %v, answered with Player %v", called, answered)
	}
}

// The acceptance of 11b with the real model: a platform and gravity 15 become two cards, two Undo steps, and the second undoes from its card.
func TestClaudePanelPlatformAndGravity(t *testing.T) {
	if os.Getenv("AEGO_AGENT_SMOKE") == "" {
		t.Skip("set AEGO_AGENT_SMOKE=1 to call the model")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude is not in PATH")
	}
	s, _ := agentSession(t, backend.Claude{})
	panel := s.App.Agent()
	panel.Submit("Add a platform node named AgentPlatform above the player in scenes/main.aescene and set the project physics gravity to [0, -15]. Do both without asking me first.")
	applied := map[int]bool{}
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		s.Step()
		for _, id := range panel.Pending() {
			if !applied[id] {
				applied[id] = true
				panel.Decide(id, true)
			}
		}
		if len(applied) >= 2 && len(panel.Pending()) == 0 && !panel.Busy() {
			break
		}
	}
	for range 120 {
		s.Step()
	}
	d := s.App.Context().Docs.Edit()
	steps := d.History.Steps(nil)
	t.Logf("cards %d, history %v", len(applied), steps)
	if len(steps) < 2 {
		t.Fatalf("history %v, want the platform and the gravity", steps)
	}
	for _, l := range steps {
		if !strings.HasPrefix(l, "Agent: ") {
			t.Fatalf("history %v has a step that is not the agent's", steps)
		}
	}
	last := 0
	for id := range applied {
		last = max(last, id)
	}
	if err := panel.UndoCard(last); err != nil {
		t.Fatal(err)
	}
	if n := len(d.History.Steps(nil)); n != len(steps) {
		t.Fatalf("Undo from the card left %d steps of %d; the undone one stays for Redo", n, len(steps))
	}
	if label, depth := d.History.Top(); depth != len(steps)-1 {
		t.Fatalf("after card Undo the top is %q at %d", label, depth)
	}
}
