package integration

import (
	"strings"
	"testing"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/app/run"
)

// scene_run presses actions by frame and checks the result: the player walks right, a check that never holds fails with its value.
func TestAgentRunPressesAndChecks(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{Run: run.AgentRunner(combatModule{})})
	ev := results(e.turn(t, []backend.Step{
		{Tool: "scene_run", Args: map[string]any{"file": bridgeScene, "frames": 90, "input": "1-80 axis Move 1; 40 press Jump",
			"assert": "@1 Player.transform.position.x ~= -13; Player.transform.position.x > -10; @90 exists Player; Player.transform.position.x < -20"}},
		{Tool: "scene_run", Args: map[string]any{"file": bridgeScene, "frames": 5, "input": "1 press Fly"}},
	}, nil))
	if len(ev) != 2 {
		t.Fatalf("got %d results", len(ev))
	}
	r := ev[0].Result
	if strings.Contains(r, "holds the asset index") {
		t.Fatalf("a run next to the editor warns about the shared index:\n%s", r)
	}
	for _, want := range []string{`"assert":"@1 Player.transform.position.x ~= -13","ok":true`, `"assert":"Player.transform.position.x \u003e -10","ok":true`,
		`"assert":"@90 exists Player","ok":true`, `"assert":"Player.transform.position.x \u003c -20","ok":false`, `assert failed: Player.transform.position.x`} {
		if !strings.Contains(r, want) {
			t.Fatalf("the run misses %s:\n%s", want, r)
		}
	}
	if !ev[1].Failed || !strings.Contains(ev[1].Result, "no input action Fly") {
		t.Fatalf("an unknown action did not fail the run: %s", ev[1].Result)
	}
}
