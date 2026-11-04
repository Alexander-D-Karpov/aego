package integration

import (
	"strings"
	"testing"
	"time"

	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
)

type queuedAgentResult struct {
	text   string
	failed bool
	err    error
}

func nextAgentCallback(t *testing.T, queued <-chan func()) func() {
	t.Helper()
	select {
	case run := <-queued:
		return run
	case <-time.After(5 * time.Second):
		t.Fatal("the agent did not queue its main-thread callback")
		return nil
	}
}

func cancelledAgentResult(t *testing.T, result <-chan queuedAgentResult) {
	t.Helper()
	select {
	case got := <-result:
		if got.err != nil || !got.failed || !strings.Contains(got.text, "cancelled") {
			t.Fatalf("cancelled call returned %#v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled call remained blocked")
	}
}

// Reset and provider switching revoke panel calls before queued previews, after
// approval but before application, and before queued questions become visible.
func TestAgentResetRevokesQueuedEditorWork(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	queued := make(chan func(), 8)
	cfg := e.s.AgentConfig()
	cfg.OnMain = func(run func()) { queued <- run }
	b, err := agentbridge.Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	client := backend.NewClient(b.Backend().MCPURL, b.Backend().Token)
	call := func(tool string, args map[string]any) <-chan queuedAgentResult {
		result := make(chan queuedAgentResult, 1)
		go func() {
			text, failed, err := client.Call(tool, args)
			result <- queuedAgentResult{text: text, failed: failed, err: err}
		}()
		return result
	}
	for _, tool := range []string{"editor_scene_context", "scene_tree"} {
		args := map[string]any{"details": true}
		if tool == "scene_tree" {
			args = map[string]any{"file": bridgeScene}
		}
		result := call(tool, args)
		run := nextAgentCallback(t, queued)
		b.RejectAll()
		run()
		cancelledAgentResult(t, result)
	}
	for _, afterApproval := range []bool{false, true} {
		name := "CancelledBeforePreview"
		if afterApproval {
			name = "CancelledBeforeApply"
		}
		result := call("scene_add_node", map[string]any{"file": bridgeScene, "name": name})
		run := nextAgentCallback(t, queued)
		if afterApproval {
			run()
			pending := b.Pending()
			if len(pending) != 1 || !b.Decide(pending[0].ID, true) {
				t.Fatal("no pending edit to approve")
			}
			run = nextAgentCallback(t, queued)
		}
		b.RejectAll()
		run()
		cancelledAgentResult(t, result)
		if len(b.Pending()) != 0 || e.alive(name) {
			t.Fatalf("cancelled work created %q or a pending card", name)
		}
		if _, depth := e.doc.History.Top(); depth != 0 {
			t.Fatalf("cancelled work added an Undo step at depth %d", depth)
		}
	}
	question := call("ask_user", map[string]any{
		"question": "Choose a color", "options": []any{map[string]any{"label": "Blue"}},
	})
	run := nextAgentCallback(t, queued)
	b.RejectAll()
	run()
	cancelledAgentResult(t, question)
	if len(b.Questions()) != 0 {
		t.Fatal("a cancelled call resurrected a question in the new session")
	}

	// The same bridge also serves unrelated CLI requests. Cancelling the panel
	// conversation must not revoke an already queued external command.
	result := make(chan error, 1)
	go func() {
		_, err := b.RunVia("via CLI", true, "scene.add-node", agent.Args{"file": bridgeScene, "name": "ExternalNode"})
		result <- err
	}()
	if cfg.OnExternal != nil {
		nextAgentCallback(t, queued)()
	}
	run = nextAgentCallback(t, queued)
	b.RejectAll()
	run()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("external command remained blocked")
	}
	if !e.alive("ExternalNode") {
		t.Fatal("panel cancellation revoked an independent external command")
	}
	if label, depth := e.doc.History.Top(); depth != 1 || !strings.HasPrefix(label, "via CLI: ") {
		t.Fatalf("external command History is %q at %d", label, depth)
	}
}

func TestChangingConversationRevokesOldPanelBearerOnly(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	prev := e.b.Backend()
	url, proxyToken, err := e.b.StartProxy()
	if err != nil {
		t.Fatal(err)
	}
	e.b.Rebind()
	next := e.b.Backend()
	if prev.Binding == next.Binding || prev.Token == next.Token {
		t.Fatal("stale conversation retained its credential")
	}
	if _, _, err := backend.NewClient(prev.MCPURL, prev.Token).Call("editor_scene_context", nil); err == nil {
		t.Fatal("closed conversation could still read the editor")
	}
	url2, tok2, err := e.b.StartProxy()
	if err != nil || url != url2 || proxyToken != tok2 {
		t.Fatal("CLI proxy was rotated with the chat")
	}
}
