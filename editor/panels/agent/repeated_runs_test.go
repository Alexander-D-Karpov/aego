package agent

import (
	"fmt"
	"testing"

	"aego/editor/core/agentui"
)

func TestRepeatedFailedRunsGroupWithoutLosingTranscript(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.Submit("run the scene")
	for i := range 3 {
		id := fmt.Sprint(i)
		p.event(agentui.Event{Kind: agentui.EvToolStart, Tool: "scene_run", CallID: id, Args: `{"file":"main.aescene"}`})
		p.event(agentui.Event{Kind: agentui.EvToolResult, Tool: "scene_run", CallID: id, Result: "same failure", Failed: true})
	}
	if len(p.blocks) != 2 || p.blocks[1].kind != blockFailed || p.blocks[1].repeats != 3 {
		t.Fatalf("identical failures were not grouped: %+v", p.blocks)
	}
	entries, err := agentui.Load(p.d.Root, p.log.ID)
	if err != nil {
		t.Fatal(err)
	}
	results := 0
	for _, e := range entries {
		if e.Kind == "result" && e.Tool == "scene_run" {
			results++
		}
	}
	if results != 3 {
		t.Fatalf("transcript kept %d of 3 results", results)
	}
	old := p.blocksFrom(entries)
	if len(old) != 2 || old[1].repeats != 3 {
		t.Fatalf("reopened transcript lost the group: %+v", old)
	}
	// Different output, different arguments and other tools are never hidden.
	for i, tc := range []struct{ tool, args, result string }{
		{"scene_run", `{"file":"main.aescene"}`, "different failure"},
		{"scene_run", `{"file":"other.aescene"}`, "different failure"},
		{"scene_validate", `{"file":"other.aescene"}`, "different failure"},
	} {
		id := fmt.Sprintf("different-%d", i)
		p.event(agentui.Event{Kind: agentui.EvToolStart, Tool: tc.tool, CallID: id, Args: tc.args})
		p.event(agentui.Event{Kind: agentui.EvToolResult, Tool: tc.tool, CallID: id, Result: tc.result, Failed: true})
	}
	if len(p.blocks) != 5 {
		t.Fatalf("different failures were hidden: %+v", p.blocks)
	}
}
