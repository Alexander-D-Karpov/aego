package integration

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"aego/app/agentui/backend"
	agentpanel "aego/editor/panels/agent"
)

// A long answer stays cheap: 500 messages with Markdown, code and tool steps cost the frame no more than the panel's budget, and an idle frame allocates nothing (11b5a).
func TestAgentPanelBudgetOn500Messages(t *testing.T) {
	skipUnderRace(t)
	var steps []backend.Step
	for i := range 500 {
		if i%5 == 4 {
			steps = append(steps, backend.Step{Tool: "scene_get", Args: map[string]any{"file": bridgeScene, "node": "Player"}})
			continue
		}
		steps = append(steps, backend.Step{Text: "### Step " + strconv.Itoa(i) + "\n\nThe player has **Motion 2D**; its `MaxSpeed` sets how fast it runs.\n\n```aes\nfunc Update(dt float) {\n\tm.Move(input.Axis(\"Move\"))\n}\n```\n" + strings.Repeat("A line of the answer that wraps in a narrow panel. ", 3)})
	}
	steps = append(steps, backend.Step{Text: "done"})
	s, _ := agentSession(t, &backend.Fake{Turns: [][]backend.Step{steps}})
	panel := s.App.Agent()
	activePanel := func() bool {
		tree := s.App.Workspace().Tree
		n, ok := tree.Node(tree.FindPanel(agentpanel.ID))
		if !ok {
			return false
		}
		id, ok := n.ActivePanel()
		return ok && id == agentpanel.ID
	}
	type frameTiming struct{ wall, cpu time.Duration }
	frame := func(n int) frameTiming {
		cpu := cpuTime()
		start := time.Now()
		for range n {
			s.Step()
		}
		wall := time.Since(start) / time.Duration(n)
		used := cpuTime()
		if cpu != 0 && used >= cpu {
			used = (used - cpu) / time.Duration(n)
		} else {
			used = 0 // CPU time is unavailable on some platforms.
		}
		return frameTiming{wall: wall, cpu: used}
	}
	for range 30 {
		s.Step()
	}
	waitEditorSettled(t, s)
	if !activePanel() {
		t.Fatal("the empty Agent panel is not the active tab")
	}
	emptyTiming := frame(60)
	empty := emptyTiming.wall
	emptyAllocs := testing.AllocsPerRun(30, func() { s.Step() })
	panel.Submit("explain the player")
	stepUntil(t, s, func() bool { return !panel.Busy() })
	for range 30 {
		s.Step()
	}
	waitEditorSettled(t, s)
	if !activePanel() {
		t.Fatal("the Agent transcript is not the active tab")
	}
	fullTiming := frame(120)
	full := fullTiming.wall
	t.Logf("frame: %v with an empty panel, %v with 500 messages", empty, full)
	if extra := full - empty; extra > 3*time.Millisecond {
		t.Errorf("a 500-message transcript costs %v a frame over an empty panel (%v vs %v); the budget is 3 ms", extra, full, empty)
		// Diagnostic only: the original wall-clock measurement above decides
		// the result. CPU includes every process thread and may be unavailable.
		t.Logf("process CPU per frame: empty %v, transcript %v (0 means unavailable)", emptyTiming.cpu, fullTiming.cpu)
		var slowest time.Duration
		var over3ms int
		for range 120 {
			start := time.Now()
			s.Step()
			d := time.Since(start)
			slowest = max(slowest, d)
			if d > 3*time.Millisecond {
				over3ms++
			}
		}
		t.Logf("subsequent 120 transcript frames: max %v, %d frames over 3 ms absolute", slowest, over3ms)
	}
	// a few allocations of noise are allowed; a transcript that allocates per message adds hundreds
	a := testing.AllocsPerRun(30, func() { s.Step() })
	t.Logf("allocations per frame: %v empty, %v with 500 messages", emptyAllocs, a)
	if a > emptyAllocs+16 {
		t.Errorf("an idle frame allocates %v times with a 500-message transcript, %v with an empty panel", a, emptyAllocs)
	}
}
