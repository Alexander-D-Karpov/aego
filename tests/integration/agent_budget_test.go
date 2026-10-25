package integration

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"aego/app/agentui/backend"
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
	frame := func(n int) time.Duration {
		start := time.Now()
		for range n {
			s.Step()
		}
		return time.Since(start) / time.Duration(n)
	}
	for range 30 {
		s.Step()
	}
	empty := frame(60)
	emptyAllocs := testing.AllocsPerRun(30, func() { s.Step() })
	panel.Submit("explain the player")
	stepUntil(t, s, func() bool { return !panel.Busy() })
	for range 30 {
		s.Step()
	}
	full := frame(120)
	t.Logf("frame: %v with an empty panel, %v with 500 messages", empty, full)
	if extra := full - empty; extra > 3*time.Millisecond {
		t.Errorf("a 500-message transcript costs %v a frame over an empty panel (%v vs %v); the budget is 3 ms", extra, full, empty)
	}
	if a := testing.AllocsPerRun(30, func() { s.Step() }); a > emptyAllocs {
		t.Errorf("an idle frame allocates %v times with a 500-message transcript, %v with an empty panel", a, emptyAllocs)
	}
}
