package shell

import (
	"sync"
	"testing"

	"aego/editor/core/agentui"
	"aego/editor/core/state"
)

func TestAgentPreferencesStayWithTheirProvider(t *testing.T) {
	var app App
	app.loadAgentSettings(&state.Editor{AgentCommand: "/opt/claude"})
	claude := app.AgentSettings("")
	if claude.Provider != "claude" || claude.Command != "/opt/claude" {
		t.Fatalf("legacy provider: %+v", claude)
	}
	claude.Model = "claude-user-model"
	app.SetAgentSettings(claude)
	app.SetAgentSettings(agentui.ProviderSettings{Provider: "codex", Command: "/opt/codex", Model: "codex-user-model", Effort: "high"})
	codex := app.AgentSettings("")
	app.SetAgentSettings(app.AgentSettings("claude"))
	if app.AgentSettings("") != claude || app.AgentSettings("codex") != codex {
		t.Fatal("switching providers mixed their settings")
	}
	app.SetAgentSettings(codex)
	saved := state.Default()
	app.saveAgentSettings(saved)
	var reopened App
	reopened.loadAgentSettings(saved)
	if reopened.AgentSettings("") != codex || reopened.AgentSettings("claude") != claude {
		t.Fatal("reopening did not restore provider and its settings")
	}
}

func TestAgentAvailabilityCanReadPreferencesDuringSwitch(t *testing.T) {
	var app App
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = app.AgentSettings("")
			_ = app.AgentCommand()
		}
	}()
	for i := 0; i < 500; i++ {
		provider := "claude"
		if i%2 == 0 {
			provider = "codex"
		}
		app.SetAgentSettings(agentui.ProviderSettings{Provider: provider, Command: "/configured/" + provider})
	}
	wg.Wait()
}
