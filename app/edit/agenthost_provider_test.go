package edit

import (
	"aego/app/edit/agentbridge"
	"errors"
	"testing"

	"aego/app/agentui/backend"
	"aego/editor/core/agentui"
	"aego/editor/shell"
)

type closingAgentConversation struct{ closed int }

func (*closingAgentConversation) Send(string) error            { return nil }
func (*closingAgentConversation) Events() <-chan backend.Event { return nil }
func (*closingAgentConversation) Cancel()                      {}
func (c *closingAgentConversation) Close() error               { c.closed++; return nil }

func TestAgentHostSelectsBackendAndKeepsProviderSettings(t *testing.T) {
	var app shell.App
	app.SetAgentSettings(agentui.ProviderSettings{Provider: "claude", Command: "/bin/custom-claude", Model: "claude-user-model"})
	app.SetAgentSettings(agentui.ProviderSettings{Provider: "codex", Command: "/bin/custom-codex", Model: "codex-user-model", Effort: "high"})
	app.SetAgentSettings(app.AgentSettings("claude"))
	h := newAgentHost(nil, nil, app.AgentSettings, app.SetAgentSettings)
	old := &closingAgentConversation{}
	h.conv = old
	if err := h.SetProvider("codex"); err != nil {
		t.Fatal(err)
	}
	b, err := h.currentBackend()
	if err != nil || b.Name() != "codex" || h.Command() != "/bin/custom-codex" || h.Model() != "codex-user-model" || h.Effort() != "high" {
		t.Fatalf("Codex selection: backend=%v error=%v options=%+v", b, err, h.options())
	}
	if old.closed != 1 || h.Running() {
		t.Fatal("switch retained the old conversation")
	}
	if err := h.SetProvider("claude"); err != nil {
		t.Fatal(err)
	}
	if h.Command() != "/bin/custom-claude" || h.Model() != "claude-user-model" || h.Effort() != "" {
		t.Fatalf("Claude preferences changed: %+v", h.options())
	}
	h.SetCommand("claude")
	if app.AgentSettings("claude").Command != "" || app.AgentSettings("codex").Command != "/bin/custom-codex" {
		t.Fatal("restoring PATH default changed the other provider")
	}
}

func TestAgentHostRejectsBusyOrInvalidSettingsWithoutReset(t *testing.T) {
	var app shell.App
	h := newAgentHost(nil, nil, app.AgentSettings, app.SetAgentSettings)
	c := &closingAgentConversation{}
	h.conv, h.busy = c, true
	if err := h.SetProvider("codex"); err == nil {
		t.Fatal("changed provider during a running turn")
	}
	if err := h.SetModel("other-model"); err == nil {
		t.Fatal("changed model during a running turn")
	}
	h.SetCommand("/another/path")
	if c.closed != 0 || h.Command() != "claude" || h.Provider() != "claude" {
		t.Fatal("rejected change reset or altered the active agent")
	}
	h.busy = false
	for _, provider := range []string{"unknown", "Codex"} {
		if err := h.SetProvider(provider); err == nil {
			t.Fatalf("accepted unknown provider %q", provider)
		}
	}
	if err := h.SetModel("model\nother"); err == nil {
		t.Fatal("accepted a multiline model identifier")
	}
	if err := h.SetEffort("high"); err == nil {
		t.Fatal("accepted a Codex-only setting for Claude")
	}
	if c.closed != 0 || h.Model() != "" {
		t.Fatal("invalid settings changed the session")
	}
}

func TestInjectedCodexUsesItsOwnCommand(t *testing.T) {
	var app shell.App
	app.SetAgentSettings(agentui.ProviderSettings{Provider: "codex", Command: "/test/codex"})
	app.SetAgentSettings(agentui.ProviderSettings{Provider: "claude", Command: "/test/claude"})
	h := newAgentHost(backend.Codex{}, nil, app.AgentSettings, app.SetAgentSettings)
	if h.Command() != "/test/codex" || h.Provider() != "codex" {
		t.Fatalf("injected Codex used another provider: %+v", h.options())
	}
	if err := h.SetProvider("claude"); err == nil {
		t.Fatal("an injected backend should remain fixed")
	}
}

func TestIdlePollDoesNotReopenUnavailableBridge(t *testing.T) {
	attempts := 0
	denied := errors.New("listen denied")
	h := newAgentHost(&backend.Fake{}, func() (*agentbridge.Bridge, error) {
		attempts++
		return nil, denied
	}, nil, nil)
	h.bridgeOpen = func() bool { return false }
	poll := func() {
		for range 60 {
			h.Cards(nil)
			h.Questions(nil)
		}
	}
	poll()
	if attempts != 0 {
		t.Fatalf("idle frames opened a listener %d times", attempts)
	}
	if err := h.CheckConnection(); !errors.Is(err, denied) {
		t.Fatalf("explicit probe: %v", err)
	}
	poll()
	if attempts != 1 {
		t.Fatal("an idle frame retried a failed probe")
	}
	if err := h.CheckConnection(); !errors.Is(err, denied) || attempts != 2 {
		t.Fatal("explicit retry failed", err, attempts)
	}
}
