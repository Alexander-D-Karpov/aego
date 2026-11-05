package integration

import (
	"os"
	"strings"
	"testing"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/editor/core/command"
	"aego/editor/core/follow"
	"aego/editor/core/workspace"
)

func replayFollow(t *testing.T) *backend.Replay {
	t.Helper()
	data, err := os.ReadFile("testdata/agent_follow.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	return &backend.Replay{Turns: backend.ReplayTurns(data)}
}

// A recorded session played with Follow on opens the scene, selects the node in the agent's selection and lights the field it changed; the user's selection and Delete stay theirs (11b5b).
func TestAgentFollowLeadsToTheChange(t *testing.T) {
	s, _ := agentSession(t, replayFollow(t))
	app := s.App
	f := app.Follow()
	f.SetOn(true)
	f.Resume = 0
	panel := app.Agent()
	ctx := app.Context()
	d := ctx.Docs.Find(bridgeScene)
	sc := d.Scene()
	player := sc.ID(sc.Find(sc.Root(), "Player"))
	panel.Submit("make the player faster")
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 })
	if it, ok := app.AgentSelection().Active(); !ok || it.Node != player {
		t.Fatalf("the agent's selection is %+v, want Player", it)
	}
	if len(ctx.Selected()) != 0 {
		t.Fatal("following changed the user's selection")
	}
	if !app.Presence().Editing(bridgeScene, player, "gameplay.Motion2D", "maxSpeed") {
		t.Fatal("the waiting card does not mark the field in the Inspector")
	}
	panel.Decide(panel.Pending()[0], true)
	stepUntil(t, s, func() bool { return !panel.Busy() })
	now := s.Engine.Platform().Now()
	if a := app.Presence().FieldLight(bridgeScene, player, "gameplay.Motion2D", "maxSpeed", now); a <= 0 {
		t.Fatal("the changed field is not lit")
	}
	if sp, _, ok := app.Presence().Where(follow.Agent, now); !ok || sp.Node != player {
		t.Fatalf("the agent's marker is at %+v", sp)
	}
	if err := ctx.Commands.Run("edit.delete"); err == nil && !sc.IsAlive(sc.Handle(player)) {
		t.Fatal("Delete removed the agent's selection")
	}
}

// The user's own work pauses following until they resume it or stay idle.
func TestAgentFollowPausesForTheUser(t *testing.T) {
	s, _ := agentSession(t, replayFollow(t))
	app := s.App
	f := app.Follow()
	f.SetOn(true)
	f.Resume = 5
	now := s.Engine.Platform().Now()
	f.UserActive(now)
	panel := app.Agent()
	panel.Submit("make the player faster")
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 })
	if app.AgentSelection().Len() != 0 {
		t.Fatal("following went on while paused")
	}
	f.Tick(now + 6)
	if f.Paused() {
		t.Fatal("following did not resume after the user was idle")
	}
	f.Go("scene_get", `{"file":"scenes/main.aescene","node":"Player"}`)
	if it, ok := app.AgentSelection().Active(); !ok || it.Node.IsZero() {
		t.Fatal("Go did not take the editor to the node")
	}
}

// Agent edits persist in the editable scene even while the game is running.
func TestAgentEditsTheDocumentDuringPlay(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	if err := e.ctx.Commands.Run(command.PlayStart); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		e.s.Step()
	}
	play := e.ctx.Docs.PlayDoc()
	if play == nil || play.Path != bridgeScene {
		t.Fatal("Play did not start on the scene")
	}
	ev := results(e.turn(t, []backend.Step{{Tool: "scene_rename", Args: map[string]any{"file": bridgeScene, "node": "Player", "name": "Hero"}}}, e.approveAll))
	if len(ev) != 1 || ev[0].Failed || !strings.Contains(ev[0].Result, "not to the running game") {
		t.Fatalf("expected an explicit Play note, got %+v", ev)
	}
	ps := play.Scene()
	if h := ps.Find(ps.Root(), "Hero"); !h.IsZero() {
		t.Fatal("the agent must not silently edit the temporary running game")
	}
	if !e.alive("Hero") || e.alive("Player") {
		t.Fatal("the agent's change did not reach the editable scene")
	}
	if err := e.ctx.Commands.Run(command.PlayStop); err != nil {
		t.Fatal(err)
	}
	if !e.alive("Hero") {
		t.Fatal("stopping Play lost the agent's change")
	}
	if err := e.doc.History.Undo(); err != nil || !e.alive("Player") || e.alive("Hero") {
		t.Fatalf("the change was not undoable after Stop: %v", err)
	}
}

func TestAgentProposalSurvivesStop(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	if err := e.ctx.Commands.Run(command.PlayStart); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		e.s.Step()
	}
	stopped := false
	ev := results(e.turn(t, []backend.Step{{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "AfterStop"}}}, func() {
		if len(e.b.Pending()) == 0 || stopped {
			return
		}
		if err := e.ctx.Commands.Run(command.PlayStop); err != nil {
			t.Fatal(err)
		}
		stopped = true
		e.approveAll()
	}))
	if !stopped || len(ev) != 1 || ev[0].Failed || !e.alive("AfterStop") {
		t.Fatalf("pending proposal did not survive Stop: stopped=%v results=%+v", stopped, ev)
	}
	if err := e.doc.History.Undo(); err != nil || e.alive("AfterStop") {
		t.Fatalf("proposal was not undoable after Stop: %v", err)
	}
}

// After a script step opens the code editor over the Scene View, the next scene step brings the Scene View back.
func TestAgentFollowReturnsToTheScene(t *testing.T) {
	s, _ := agentSession(t, &backend.Fake{Turns: [][]backend.Step{{
		{Tool: "script_read", Args: map[string]any{"file": "scripts/player/player.aes"}},
		{Tool: "scene_apply", Args: map[string]any{"file": bridgeScene, "tx": `tx version=1 { label="Probe" op "set" { node="Player" path="transform.position" value=[-13, 3] } }`}},
		{Text: "done"},
	}}})
	app := s.App
	f := app.Follow()
	f.SetOn(true)
	f.Resume = 0
	panel := app.Agent()
	ws := app.Workspace()
	front := func(id string) bool {
		n, ok := ws.Tree.Node(ws.Tree.FindPanel(id))
		return ok && n.Active < len(n.Tabs) && n.Tabs[n.Active] == id
	}
	panel.Submit("move the player")
	stepUntil(t, s, func() bool { return !front(workspace.Scene) || len(panel.Pending()) == 1 })
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 })
	if !front(workspace.Scene) {
		t.Fatal("the scene step left the code editor in front of the Scene View")
	}
	sc := app.Context().Docs.Find(bridgeScene).Scene()
	if it, ok := app.AgentSelection().Active(); !ok || it.Node != sc.ID(sc.Find(sc.Root(), "Player")) {
		t.Fatalf("the agent's selection is %+v, want Player from the transaction", it)
	}
}
