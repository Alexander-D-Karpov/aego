package agent

import (
	"fmt"
	"strings"
	"testing"

	"aego/core/ids"
	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/engine/base"
	"aego/scene"
)

type quickDocs struct {
	doc  *document.SceneDocument
	rows []context.ScriptProblem
}

func (d *quickDocs) Doc() *document.SceneDocument { return d.doc }
func (d *quickDocs) Documents() *document.Manager { return nil }
func (d *quickDocs) ScriptProblems(dst []context.ScriptProblem) ([]context.ScriptProblem, uint64) {
	return append(dst, d.rows...), 1
}
func (*quickDocs) FirstError(string) (context.ScriptProblem, int, bool) {
	return context.ScriptProblem{}, 0, false
}

type quickSelection struct {
	services.Selection
	ids []ids.NodeID
}

func (s quickSelection) Selected() []ids.NodeID { return s.ids }

type quickSceneHost struct {
	*providerTestHost
	explained []string
}

func (h *quickSceneHost) ExplainScene(text string) error {
	h.explained = append(h.explained, text)
	return nil
}
func (*quickSceneHost) CheckConnection() error          { return nil }
func (*quickSceneHost) Diagnostics() agentui.Connection { return agentui.Connection{} }

func TestProblemQuickActionsDraftCurrentDiagnosticsWithoutSending(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	docs := &quickDocs{rows: []context.ScriptProblem{
		{Category: context.ProblemScript, Error: true, Block: true, Code: "AES101", Path: "scripts/player.aes", Line: 7, Col: 4, Node: ids.NodeID{Lo: 3}, Message: "bad expression"},
		{Category: context.ProblemScene, Error: true, Block: true, Code: "SCENE001", Message: "missing camera"},
		{Category: context.ProblemScript, Error: false, Code: "WARNING", Message: "warning"},
	}}
	p.d.Docs = docs
	p.draftProblems(false)
	for _, s := range []string{"AES101", "scripts/player.aes", `"line":7`, `"column":4`, ids.NodeID{Lo: 3}.String(), "bad expression"} {
		if !strings.Contains(p.input, s) {
			t.Fatalf("missing %q in draft: %s", s, p.input)
		}
	}
	if strings.Contains(p.input, "SCENE001") || strings.Contains(p.input, "WARNING") || len(h.sent) != 0 {
		t.Fatal("script action included unrelated diagnostics or sent automatically")
	}
	old := p.input
	p.draftProblems(true)
	if p.input != old || len(h.sent) != 0 {
		t.Fatal("quick action overwrote an existing draft or sent it")
	}
	p.input = ""
	docs.rows[1].Message = "camera changed before click"
	p.draftProblems(true)
	if !strings.Contains(p.input, "SCENE001") || !strings.Contains(p.input, "camera changed before click") || strings.Contains(p.input, "WARNING") || len(h.sent) != 0 {
		t.Fatalf("play-blocker draft did not use current diagnostics: %s", p.input)
	}
}

func TestExplainScenePrefersBoundHostAndFallbackIsBounded(t *testing.T) {
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	sc := scene.New(types, nil, scene.Options{})
	var first scene.NodeHandle
	for i := 0; i < 15; i++ {
		h := sc.Create(sc.Root(), fmt.Sprintf("node-%02d", i))
		if i == 0 {
			first = h
		}
	}
	sc.Flush()
	m := document.NewManager(document.Options{Root: t.TempDir(), Types: types})
	doc := m.New("scenes/current.aescene", sc)
	docs := &quickDocs{doc: doc}
	sel := quickSelection{ids: []ids.NodeID{sc.ID(first)}}

	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.d.Docs, p.d.Selection = docs, sel
	p.ExplainScene()
	if len(h.sent) != 1 || !strings.Contains(h.sent[0], "scenes/current.aescene") || !strings.Contains(h.sent[0], "node-00") || !strings.Contains(h.sent[0], sc.ID(first).String()) || strings.Contains(h.sent[0], "node-12") {
		t.Fatalf("fallback snapshot missing or unbounded: %v", h.sent)
	}

	bound := &quickSceneHost{providerTestHost: newProviderTestHost()}
	p2 := New(Deps{Host: bound, Docs: docs, Selection: sel, Root: t.TempDir(), Notices: &context.Editor{}})
	t.Cleanup(p2.Close)
	p2.ExplainScene()
	if len(bound.explained) != 1 || len(bound.sent) != 0 || strings.Contains(bound.explained[0], "sample_nodes") {
		t.Fatalf("bound SceneHost was bypassed: %v %v", bound.explained, bound.sent)
	}
}

func TestTurnToolCountersReset(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.Submit("first")
	p.event(agentui.Event{Kind: agentui.EvToolStart, Tool: "scene_tree", CallID: "a"})
	p.event(agentui.Event{Kind: agentui.EvToolStart, Tool: "script_read", CallID: "b"})
	p.event(agentui.Event{Kind: agentui.EvToolResult, Tool: "scene_tree", CallID: "a"})
	p.event(agentui.Event{Kind: agentui.EvToolResult, Tool: "script_read", CallID: "b", Failed: true})
	p.event(agentui.Event{Kind: agentui.EvTurnEnd})
	if p.toolCalls != 2 || p.toolFailures != 1 || p.meta == "" {
		t.Fatalf("wrong tool summary: %d/%d %q", p.toolCalls, p.toolFailures, p.meta)
	}
	p.Submit("second")
	if p.toolCalls != 0 || p.toolFailures != 0 {
		t.Fatal("new turn inherited previous counters")
	}
	p.event(agentui.Event{Kind: agentui.EvToolStart, Tool: "scene_tree", CallID: "c"})
	p.NewSession()
	if p.toolCalls != 0 || p.toolFailures != 0 || p.meta != "" {
		t.Fatal("new session inherited previous counters")
	}
}

func TestEmptyPanelOffersOnlyCurrentDiagnosticActions(t *testing.T) {
	p := providerPanel(t, newProviderTestHost())
	source := &quickDocs{}
	p.d.Docs = source
	if errors, blocked := p.inlineProblems(); errors || blocked {
		t.Fatal("offered a diagnostic action without a matching issue")
	}
	source.rows = []context.ScriptProblem{{Category: context.ProblemScene, Block: true}}
	if errors, blocked := p.inlineProblems(); errors || !blocked {
		t.Fatal("scene blocker should offer only the Play diagnosis")
	}
	source.rows = []context.ScriptProblem{{Category: context.ProblemScript, Error: true}}
	if errors, blocked := p.inlineProblems(); !errors || blocked {
		t.Fatal("script error should offer only the script diagnosis")
	}
	source.rows = nil
	if errors, blocked := p.inlineProblems(); errors || blocked {
		t.Fatal("resolved diagnostics remained visible")
	}
}
