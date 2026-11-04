package shell

import (
	"sync"

	"aego/editor/core/agentui"
	"aego/editor/core/state"
)

// Availability checks run off the UI thread while the user can select another CLI.
type agentPreferences struct {
	mu       sync.RWMutex
	provider string
	claude   agentui.ProviderSettings
	codex    agentui.ProviderSettings
}

// AgentSettings returns one provider's preferences; empty selects the current provider.
func (a *App) AgentSettings(provider string) agentui.ProviderSettings {
	p := &a.agentPrefs
	p.mu.RLock()
	defer p.mu.RUnlock()
	if provider == "" {
		provider = p.provider
	}
	if provider == "" {
		provider = "claude"
	}
	s := p.claude
	if provider == "codex" {
		s = p.codex
	}
	s.Provider = provider
	return s
}

// SetAgentSettings selects a provider and changes only its own preferences.
func (a *App) SetAgentSettings(s agentui.ProviderSettings) {
	p := &a.agentPrefs
	p.mu.Lock()
	defer p.mu.Unlock()
	switch s.Provider {
	case "", "claude":
		s.Provider, s.Effort = "claude", ""
		p.claude = s
	case "codex":
		p.codex = s
	default:
		return
	}
	p.provider = s.Provider
}

func (a *App) loadAgentSettings(s *state.Editor) {
	p := &a.agentPrefs
	p.mu.Lock()
	defer p.mu.Unlock()
	p.provider = s.AgentProvider
	if p.provider != "codex" {
		p.provider = "claude"
	}
	p.claude = agentui.ProviderSettings{Provider: "claude", Command: s.AgentCommand, Model: s.AgentClaudeModel}
	p.codex = agentui.ProviderSettings{Provider: "codex", Command: s.AgentCodexCommand, Model: s.AgentCodexModel, Effort: s.AgentCodexEffort}
}

func (a *App) saveAgentSettings(s *state.Editor) {
	p := &a.agentPrefs
	p.mu.RLock()
	defer p.mu.RUnlock()
	s.AgentProvider = p.provider
	if s.AgentProvider == "claude" {
		s.AgentProvider = ""
	}
	s.AgentCommand, s.AgentClaudeModel = p.claude.Command, p.claude.Model
	s.AgentCodexCommand, s.AgentCodexModel, s.AgentCodexEffort = p.codex.Command, p.codex.Model, p.codex.Effort
}
