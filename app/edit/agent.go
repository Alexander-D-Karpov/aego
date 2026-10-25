package edit

import (
	"aego/app/edit/agentbridge"
	"aego/editor/core/agentui"
	"aego/engine/docs"
)

// AgentConfig is how the Agent panel starts its bridge in this editor (D341, D342).
func (s *Session) AgentConfig() agentbridge.Config {
	return agentbridge.Config{
		Ctx:            s.App.Context(),
		Modules:        s.Modules,
		OnMain:         s.Engine.Jobs().OnMain,
		Bindings:       s.Bindings,
		Command:        s.App.AgentCommand(),
		ProjectChanged: s.App.ProjectChanged,
		Docs:           func() (*docs.Registry, error) { return s.Docs, nil },
	}
}

// agentHost is the Agent panel's host, nil in a headless editor without a backend.
func (s *Session) agentHost(cfg Config) agentui.Host {
	if cfg.Headless && cfg.AgentBackend == nil {
		return nil
	}
	return newAgentHost(cfg.AgentBackend, s.AgentConfig, func() string { return s.App.AgentCommand() }, func(c string) { s.App.SetAgentCommand(c) })
}
