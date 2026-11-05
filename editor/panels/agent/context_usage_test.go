package agent

import (
	"strings"
	"testing"

	"aego/editor/core/agentui"
)

func TestUsageEventUpdatesContextWindow(t *testing.T) {
	p := &Panel{meter: agentui.NewMeter("guide", "tools")}
	p.meterEvent(agentui.Event{Kind: agentui.EvUsage, Tokens: 250_000, Window: 400_000, Model: "large"})
	l := p.contextLayout()
	if l.Window != 400_000 || l.Used != 250_000 || l.Fraction() != 0.625 {
		t.Fatalf("usage event lost the model window: %+v", l)
	}
}

func TestCodexCompactionWithoutTokenCountPreservesTranscript(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.Submit("keep this task")
	p.event(agentui.Event{Kind: agentui.EvText, Text: "work already done"})
	p.event(agentui.Event{Kind: agentui.EvUsage, Tokens: 250_000, Window: 400_000})
	p.event(agentui.Event{Kind: agentui.EvCompact})
	if h.resets != 0 || len(p.blocks) != 3 || !strings.Contains(p.blocks[2].text, "compacted") || strings.Contains(p.blocks[2].text, "0 %") {
		t.Fatalf("compaction reset the session or invented a percentage: %+v", p.blocks)
	}
	p.event(agentui.Event{Kind: agentui.EvUsage, Tokens: 80_000, Window: 400_000})
	if l := p.contextLayout(); l.Used != 80_000 || l.Window != 400_000 {
		t.Fatalf("post-compaction usage: %+v", l)
	}
	entries, err := agentui.Load(p.d.Root, p.log.ID)
	if err != nil || len(entries) < 4 || entries[1].Text != "keep this task" || entries[2].Text != "work already done" {
		t.Fatalf("transcript was lost during compaction: %+v, %v", entries, err)
	}
}
