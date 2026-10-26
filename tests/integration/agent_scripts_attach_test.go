package integration

import (
	"os"
	"strings"
	"testing"

	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/app/run"
	"aego/editor/core/command"
	"aego/project"
	"aego/scene/scriptcomp"
)

// The agent attaches a script to a node, points its node field at another node by path, reads it back, and Undo takes it off.
func TestAgentAttachesAScript(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	tx := `tx version=1 { label="Patrol Platform" op "add-comp" { node="Platform" comp="EnemyAI" } op "set" { node="Platform" path="EnemyAI.PointA" value="Root/PatrolA2" } }`
	ev := results(e.turn(t, []backend.Step{
		{Tool: "scene_apply", Args: map[string]any{"file": bridgeScene, "tx": tx}},
		{Tool: "scene_get", Args: map[string]any{"file": bridgeScene, "node": "Platform", "path": "EnemyAI.PointA"}},
	}, e.approveAll))
	if len(ev) != 2 || ev[0].Failed || ev[1].Failed {
		t.Fatalf("the tools failed: %+v", ev)
	}
	sc := e.doc.Scene()
	plat := sc.Find(sc.Root(), "Platform")
	scs, ok := scriptcomp.ScriptsOf(sc, plat)
	if !ok || scs.Index("mygame.enemy.EnemyAI") < 0 {
		t.Fatal("EnemyAI is not on Platform")
	}
	want := sc.ID(sc.Find(sc.Root(), "PatrolA2")).String()
	if !strings.Contains(ev[1].Result, want) {
		t.Fatalf("PointA reads %s, want node %s", ev[1].Result, want)
	}
	if err := e.ctx.Commands.Run(command.EditUndo); err != nil {
		t.Fatal(err)
	}
	if scs, ok := scriptcomp.ScriptsOf(sc, sc.Find(sc.Root(), "Platform")); ok && scs.Index("mygame.enemy.EnemyAI") >= 0 {
		t.Fatal("Undo left EnemyAI on Platform")
	}
}

// scene_run in the editor plays the scene as the editor has it, unsaved changes included, and reports watched nodes.
func TestAgentRunsTheUnsavedScene(t *testing.T) {
	var ran agent.RunRequest
	var text string
	runner := run.AgentRunner(combatModule{})
	e := bridgeSetup(t, agentbridge.Config{Run: func(p *project.Project, r agent.RunRequest) (agent.RunReport, error) {
		ran = r
		data, _ := os.ReadFile(r.Scene)
		text = string(data)
		return runner(p, r)
	}})
	ev := results(e.turn(t, []backend.Step{
		{Tool: "scene_rename", Args: map[string]any{"file": bridgeScene, "node": "Player", "name": "Hero"}},
		{Tool: "scene_run", Args: map[string]any{"file": bridgeScene, "frames": 30, "watch": "Hero,EnemyLeft"}},
	}, e.approveAll))
	if len(ev) != 2 || ev[1].Failed {
		t.Fatalf("scene_run failed: %+v", ev)
	}
	if !strings.Contains(text, `"Hero"`) {
		t.Fatalf("the run played %s without the unsaved rename", ran.Scene)
	}
	if !strings.Contains(ev[1].Result, `"node":"Hero"`) || !strings.Contains(ev[1].Result, `"node":"EnemyLeft"`) {
		t.Fatalf("the report has no watched nodes:\n%s", ev[1].Result)
	}
}
