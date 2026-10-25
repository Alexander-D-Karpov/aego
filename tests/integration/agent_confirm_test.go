package integration

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit"
	"aego/app/edit/agentbridge"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/module"
	physset "aego/modules/physics/settings"
	"aego/project"
)

type bridgeEnv struct {
	root string
	s    *edit.Session
	ctx  *context.Editor
	doc  *document.SceneDocument
	b    *agentbridge.Bridge
}

func bridgeSetup(t *testing.T, cfg agentbridge.Config) *bridgeEnv {
	t.Helper()
	root := copyExample(t, "platformer")
	p, err := project.Load(root)
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
	d, err := ctx.Docs.Open(bridgeScene)
	if err != nil {
		t.Fatal(err)
	}
	base := s.AgentConfig()
	base.Timeout = cfg.Timeout
	base.Run = cfg.Run
	b, err := agentbridge.Start(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return &bridgeEnv{root: root, s: s, ctx: ctx, doc: d, b: b}
}

// turn plays one scripted turn, stepping the editor and calling each between steps until it ends.
func (e *bridgeEnv) turn(t *testing.T, steps []backend.Step, each func()) []backend.Event {
	t.Helper()
	conv, err := (&backend.Fake{Turns: [][]backend.Step{steps}}).Start(e.b.Backend())
	if err != nil {
		t.Fatal(err)
	}
	if err := conv.Send("go"); err != nil {
		t.Fatal(err)
	}
	var out []backend.Event
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case ev, ok := <-conv.Events():
			if !ok {
				return out
			}
			out = append(out, ev)
			if ev.Kind == backend.EvError {
				t.Fatal(ev.Err)
			}
			if ev.Kind == backend.EvTurnEnd {
				return out
			}
		default:
			e.s.Step()
			if each != nil {
				each()
			}
		}
	}
	t.Fatal("the turn did not end")
	return nil
}

func results(events []backend.Event) []backend.Event {
	var out []backend.Event
	for _, e := range events {
		if e.Kind == backend.EvToolResult {
			out = append(out, e)
		}
	}
	return out
}

func (e *bridgeEnv) alive(name string) bool {
	sc := e.doc.Scene()
	h := sc.Find(sc.Root(), name)
	return !h.IsZero() && sc.IsAlive(h)
}

func (e *bridgeEnv) approveAll() {
	for _, p := range e.b.Pending() {
		e.b.Decide(p.ID, true)
	}
}

func gravity(t *testing.T, ctx *context.Editor) (float32, bool) {
	t.Helper()
	phys, ok := module.SectionValue[physset.Project](ctx.Sections, physset.ID)
	if !ok {
		t.Fatal("no physics section")
	}
	return phys.Gravity.Y, phys.HasGravity
}

// The acceptance of D342: a platform and a gravity change are two Undo steps, each undone with its bytes.
func TestAgentPlatformAndGravityAreTwoUndoSteps(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	_, hadGravity := gravity(t, e.ctx)
	events := e.turn(t, []backend.Step{
		{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "AgentPlatform"}},
		{Tool: "project_set", Args: map[string]any{"section": "physics", "field": "gravity", "value": "[0, -15]"}},
	}, e.approveAll)
	for _, r := range results(events) {
		if r.Failed {
			t.Fatalf("%s: %s", r.Tool, r.Result)
		}
	}
	steps := e.doc.History.Steps(nil)
	want := []string{"Agent: Add Node", "Agent: Set physics.gravity"}
	if !slices.Equal(steps, want) {
		t.Fatalf("history %v, want %v", steps, want)
	}
	if y, ok := gravity(t, e.ctx); !ok || y != -15 {
		t.Fatalf("gravity y %v set %v", y, ok)
	}
	disk, _ := os.ReadFile(filepath.Join(e.root, project.FileName))
	if !strings.Contains(string(disk), "-15") {
		t.Fatal("project.aego does not have the new gravity")
	}
	if err := e.doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if y, ok := gravity(t, e.ctx); ok != hadGravity || y == -15 {
		t.Fatal("Undo did not restore gravity")
	}
	disk, _ = os.ReadFile(filepath.Join(e.root, project.FileName))
	if strings.Contains(string(disk), "-15") {
		t.Fatal("Undo did not restore project.aego")
	}
	if !e.alive("AgentPlatform") {
		t.Fatal("the first Undo took the platform too")
	}
	if err := e.doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if e.alive("AgentPlatform") {
		t.Fatal("the second Undo did not remove the platform")
	}
}

func TestAgentAutoApplyLeavesRemovalsToTheUser(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	e.b.SetAutoApply(true)
	var asked []*agentbridge.Proposal
	events := e.turn(t, []backend.Step{
		{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "Crate"}},
		{Tool: "scene_remove_node", Args: map[string]any{"file": bridgeScene, "node": "Crate"}},
	}, func() {
		for _, p := range e.b.Pending() {
			asked = append(asked, p)
			e.b.Decide(p.ID, false)
		}
	})
	if len(asked) != 1 || asked[0].Command != "scene.remove-node" || !asked[0].Destructive {
		t.Fatalf("asked %v", asked)
	}
	rs := results(events)
	if len(rs) != 2 || rs[0].Failed || !rs[1].Failed || !strings.Contains(rs[1].Result, "rejected") {
		t.Fatalf("results %+v", rs)
	}
	if !e.alive("Crate") {
		t.Fatal("a rejected removal ran")
	}
	if n := len(e.doc.History.Steps(nil)); n != 1 {
		t.Fatalf("history has %d steps, want 1", n)
	}
}

func TestAgentProposalExpires(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{Timeout: 300 * time.Millisecond})
	events := e.turn(t, []backend.Step{
		{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "Late"}},
	}, nil)
	rs := results(events)
	if len(rs) != 1 || !rs[0].Failed || !strings.Contains(rs[0].Result, "no answer") {
		t.Fatalf("results %+v", rs)
	}
	ps := e.b.Proposals()
	if len(ps) != 1 || ps[0].State != agentbridge.Expired {
		t.Fatalf("proposals %+v", ps)
	}
	if e.alive("Late") || len(e.doc.History.Steps(nil)) != 0 {
		t.Fatal("an expired proposal changed the scene")
	}
	if e.b.Decide(ps[0].ID, true) {
		t.Fatal("an expired proposal accepted a decision")
	}
}

func TestAgentCardUndoOnlyForTheLastStep(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	e.b.SetAutoApply(true)
	e.turn(t, []backend.Step{
		{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "A"}},
		{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "B"}},
	}, nil)
	ps := e.b.Proposals()
	if len(ps) != 2 || ps[0].State != agentbridge.Applied || ps[1].State != agentbridge.Applied {
		t.Fatalf("proposals %+v", ps)
	}
	if e.b.CanUndo(ps[0].ID) || !e.b.CanUndo(ps[1].ID) {
		t.Fatal("only the last step should be undoable from its card")
	}
	if err := e.b.Undo(ps[1].ID); err != nil {
		t.Fatal(err)
	}
	if e.alive("B") || !e.alive("A") {
		t.Fatal("card Undo removed the wrong node")
	}
	if !e.b.CanUndo(ps[0].ID) {
		t.Fatal("the first step is the last one now")
	}
}

func TestProjectSetFromTheCommandLine(t *testing.T) {
	root := copyExample(t, "platformer")
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := agent.Open(p, agent.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	path := filepath.Join(root, project.FileName)
	before, _ := os.ReadFile(path)
	out, err := agent.Run(sess, "project.set", agent.Args{"section": "physics", "field": "gravity", "value": "[0, -15]", "dry-run": true})
	if err != nil || !out.DryRun || len(out.Changes) != 1 {
		t.Fatalf("dry run %+v %v", out, err)
	}
	if now, _ := os.ReadFile(path); string(now) != string(before) {
		t.Fatal("dry run wrote project.aego")
	}
	if _, err := agent.Run(sess, "project.set", agent.Args{"section": "physics", "field": "gravity", "value": "[0, -15]"}); err != nil {
		t.Fatal(err)
	}
	q, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var phys physset.Project
	if err := physset.Decode(q.Block(physset.ID), &phys); err != nil || phys.Gravity.Y != -15 {
		t.Fatalf("physics block %+v %v", phys, err)
	}
	if _, err := agent.Run(sess, "project.set", agent.Args{"section": "physics", "field": "gravty", "value": "1"}); err == nil || !strings.Contains(err.Error(), "gravty") {
		t.Fatalf("an unknown field gave %v", err)
	}
	got, err := agent.Run(sess, "project.get", agent.Args{"section": "physics"})
	if err != nil || !strings.Contains(got.Text, "gravity") {
		t.Fatalf("project.get %q %v", got.Text, err)
	}
}

// schema_show in the editor lists the script methods of a component, as the CLI does.
func TestAgentSchemaShowHasScriptMethods(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	rs := results(e.turn(t, []backend.Step{
		{Tool: "schema_show", Args: map[string]any{"type": "gameplay.Motion2D"}},
	}, nil))
	if len(rs) != 1 || rs[0].Failed || !strings.Contains(rs[0].Result, "Jump") {
		t.Fatalf("schema_show %+v", rs)
	}
}

// The transaction the Guide shows applies as one proposal and one Undo step: node, component and field together.
func TestAgentSceneApplyIsOneStep(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	events := e.turn(t, []backend.Step{
		{Tool: "scene_apply", Args: map[string]any{"file": bridgeScene, "tx": agentbridge.GuideExample}},
	}, e.approveAll)
	for _, r := range results(events) {
		if r.Failed {
			t.Fatalf("%s: %s", r.Tool, r.Result)
		}
	}
	if ps := e.b.Proposals(); len(ps) != 1 {
		t.Fatalf("%d proposals, want 1", len(ps))
	}
	if steps := e.doc.History.Steps(nil); len(steps) != 1 || steps[0] != "Agent: Add Ledge" {
		t.Fatalf("Undo steps %v, want one named Agent: Add Ledge", steps)
	}
	if !e.alive("Ledge") {
		t.Fatal("the node is not in the scene")
	}
}
