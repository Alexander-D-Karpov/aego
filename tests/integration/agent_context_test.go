package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/editor/core/agentui"
)

const contextRecording = "testdata/agent_context.jsonl"

// TestRecordAgentContext records a real Claude Code session against the editor for the context tests; it runs only with AEGO_RECORD_AGENT=1.
func TestRecordAgentContext(t *testing.T) {
	if os.Getenv("AEGO_RECORD_AGENT") != "1" {
		t.Skip("set AEGO_RECORD_AGENT=1 to record a session with the claude CLI")
	}
	e := bridgeSetup(t, agentbridge.Config{})
	rec := &lockedBuffer{}
	cfg := e.b.Backend()
	cfg.Record = rec
	conv, err := backend.Claude{}.Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer conv.Close()
	for _, msg := range []string{
		"Look at scenes/main.aescene: read the tree, then the Player node and its script. Tell me in two sentences how the player moves.",
		"Search the docs for moving platforms and read the most relevant page, then add a node Ledge above the player at [-13, 1] with a SpriteRenderer.",
	} {
		line, _ := json.Marshal(map[string]string{"type": "aego_user", "text": msg})
		rec.Write(append(line, '\n'))
		if err := conv.Send(msg); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Minute)
	turn:
		for time.Now().Before(deadline) {
			select {
			case ev, ok := <-conv.Events():
				if !ok {
					t.Fatal("the CLI ended")
				}
				if ev.Kind == backend.EvError {
					t.Fatal(ev.Err)
				}
				if ev.Kind == backend.EvTurnEnd {
					break turn
				}
			default:
				e.s.Step()
				e.approveAll()
			}
		}
	}
	time.Sleep(time.Second)
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(rec.Bytes()))
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, `"subtype":"hook_`) {
			continue
		}
		out.WriteString(line + "\n")
	}
	if err := os.WriteFile(contextRecording, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lockedBuffer collects the CLI's lines and the recorder's own from two goroutines.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) Bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bytes.Clone(l.b.Bytes())
}

// meterFrom plays a recorded session into a meter and reports, at each later answer, how much the CLI's count and the estimate grew.
func meterFrom(t *testing.T, each func(m *agentui.Meter, grew, est int)) *agentui.Meter {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(contextRecording))
	if err != nil {
		t.Fatal(err)
	}
	m := agentui.NewMeter(agentbridge.Guide, "")
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	measured := false
	first, firstEst := 0, 0
	ours := func() int {
		n := 0
		for _, it := range m.Items() {
			n += it.Est
		}
		return n
	}
	for sc.Scan() {
		line := sc.Bytes()
		var u struct{ Type, Text string }
		if json.Unmarshal(line, &u) == nil && u.Type == "aego_user" {
			m.User(u.Text)
			continue
		}
		for _, ev := range backend.Decode(line) {
			switch ev.Kind {
			case backend.EvText:
				m.Text(ev.Text)
			case backend.EvToolStart:
				m.Call(ev.Tool, ev.CallID, string(ev.Args), ev.Tool)
			case backend.EvToolResult:
				m.Result(ev.Tool, ev.CallID, ev.Result, "", ev.Failed)
			case backend.EvUsage:
				if ev.Tokens == 0 {
					continue
				}
				if !measured {
					first, firstEst = ev.Tokens, ours()
				} else if each != nil {
					each(m, ev.Tokens-first, ours()-firstEst)
				}
				m.Usage(ev.Tokens)
				measured = true
			case backend.EvTurnEnd:
				m.SetWindow(ev.Window)
			}
		}
	}
	return m
}

// The editor's estimate of what the conversation added after the first answer stays within 15 % of what the CLI counted, on a recorded session.
func TestAgentContextEstimateMatchesUsage(t *testing.T) {
	checked := 0
	m := meterFrom(t, func(m *agentui.Meter, grew, est int) {
		t.Logf("grew %d, estimated %d", grew, est)
		if grew > 2000 {
			if d := float64(est-grew) / float64(grew); d > 0.15 || d < -0.15 {
				t.Errorf("estimated %d tokens added where the CLI counted %d (%+.0f %%)", est, grew, d*100)
			}
			checked++
		}
	})
	if checked < 3 {
		t.Fatalf("the recording has %d answers to check", checked)
	}
	var l agentui.Layout
	m.Layout(&l)
	if l.Window != 1_000_000 || l.Rows[agentui.CatScene].Tokens == 0 || l.Rows[agentui.CatCode].Tokens == 0 || l.Rows[agentui.CatDocs].Tokens == 0 {
		t.Fatalf("the layout has no window or misses categories: %+v", l)
	}
}

// Dropping a result in the Context popup starts the next message in a new session whose history leaves the result out.
func TestAgentContextDropRebuildsTheSession(t *testing.T) {
	fake := &backend.Fake{Turns: [][]backend.Step{
		{{Tool: "scene_tree", Args: map[string]any{"file": bridgeScene}}, {Text: "The scene has a player."}},
		{{Text: "ok"}},
	}}
	s, _ := agentSession(t, fake)
	panel := s.App.Agent()
	panel.Submit("what is in the scene")
	stepUntil(t, s, func() bool { return !panel.Busy() })
	m := panel.Meter()
	var tree *agentui.Item
	for i := range m.Items() {
		if it := &m.Items()[i]; it.Tool == "scene_tree" {
			tree = it
		}
	}
	if tree == nil || tree.Cat != agentui.CatScene {
		t.Fatal("the scene tree is not in the Scene category of the context")
	}
	m.Drop(tree.ID, true)
	panel.Submit("go on")
	stepUntil(t, s, func() bool { return !panel.Busy() })
	sent := fake.Sent()
	if len(sent) != 2 {
		t.Fatalf("sent %d messages", len(sent))
	}
	if !strings.Contains(sent[1], "This continues an earlier session") || !strings.Contains(sent[1], "The scene has a player.") || !strings.HasSuffix(sent[1], "go on") {
		t.Fatalf("the second message is not a rebuilt history:\n%s", sent[1])
	}
	if strings.Contains(sent[1], `"children"`) || strings.Contains(sent[1], "Result of") {
		t.Fatalf("the dropped tree went with the next message:\n%s", sent[1])
	}
	if n, _ := m.Dropped(); n != 0 {
		t.Fatal("the meter still has the dropped result")
	}
}

// New Session with Summary asks the agent for a summary and sends it, with the nodes the session changed, with the next message.
func TestAgentContextSummaryStartsANewSession(t *testing.T) {
	fake := &backend.Fake{Turns: [][]backend.Step{
		{{Tool: "scene_rename", Args: map[string]any{"file": bridgeScene, "node": "Player", "name": "Hero"}}, {Text: "Renamed."}},
		{{Text: "The user asked to rename the player; I renamed Player to Hero."}},
		{{Text: "ok"}},
	}}
	s, _ := agentSession(t, fake)
	panel := s.App.Agent()
	panel.Submit("rename the player to Hero")
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 || !panel.Busy() })
	for _, id := range panel.Pending() {
		panel.Decide(id, true)
	}
	stepUntil(t, s, func() bool { return !panel.Busy() })
	panel.SummarizeSession()
	stepUntil(t, s, func() bool { return panel.Seed() != "" })
	seed := panel.Seed()
	if !strings.Contains(seed, "renamed Player to Hero") || !strings.Contains(seed, "Nodes changed in that session: Hero") && !strings.Contains(seed, "Nodes changed in that session: Player") {
		t.Fatalf("the summary misses the changed node:\n%s", seed)
	}
	panel.Submit("continue")
	stepUntil(t, s, func() bool { return !panel.Busy() })
	sent := fake.Sent()
	if last := sent[len(sent)-1]; !strings.HasPrefix(last, "Summary of the previous session") || !strings.HasSuffix(last, "continue") {
		t.Fatalf("the new session did not start from the summary:\n%s", last)
	}
}

// The read limits trim a scene tree to its first nodes and say how many are left out.
func TestAgentReadBudgetTrimsTheTree(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	e.b.SetBudgets(agentui.Budgets{TreeNodes: 5, TreeDepth: 8, DocResults: 3, ScriptLines: 50})
	ev := results(e.turn(t, []backend.Step{
		{Tool: "scene_tree", Args: map[string]any{"file": bridgeScene}},
		{Tool: "script_read", Args: map[string]any{"file": "scripts/player/player.aes"}},
		{Tool: "docs_search", Args: map[string]any{"query": "body"}},
	}, nil))
	if len(ev) != 3 || ev[0].Failed {
		t.Fatalf("the reads failed: %+v", ev)
	}
	if !strings.Contains(ev[0].Result, "more nodes are left out") || !strings.Contains(ev[0].Result, `"more":`) {
		t.Fatalf("the tree is not trimmed:\n%s", ev[0].Result)
	}
	if !strings.Contains(ev[1].Result, "lines 1-50 of") {
		t.Fatalf("the script is not read in parts:\n%.300s", ev[1].Result)
	}
	var hits struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal([]byte(ev[2].Result), &hits); err != nil || len(hits.Results) != 3 {
		t.Fatalf("docs_search gave %d results: %v", len(hits.Results), err)
	}
}
