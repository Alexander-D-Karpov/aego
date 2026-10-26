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
		Run:            agentRun,
		ProxyConfirm:   s.App.ProxyConfirm,
		OnExternal:     s.App.ExternalCall,
	}
}

// agentBridge is the editor's one bridge: the Agent panel's agent and the proxy share its proposals and Undo steps.
func (s *Session) agentBridge() (*agentbridge.Bridge, error) {
	if s.bridge == nil {
		b, err := agentbridge.Start(s.AgentConfig())
		if err != nil {
			return nil, err
		}
		s.bridge = b
	}
	return s.bridge, nil
}

// startProxy serves the CLI and outside agents through the bridge while the editor is open (11b6).
func (s *Session) startProxy() (string, string, error) {
	b, err := s.agentBridge()
	if err != nil {
		return "", "", err
	}
	return b.StartProxy()
}

// agentHost is the Agent panel's host, nil in a headless editor without a backend.
func (s *Session) agentHost(cfg Config) agentui.Host {
	if cfg.Headless && cfg.AgentBackend == nil {
		return nil
	}
	return newAgentHost(cfg.AgentBackend, s.agentBridge, func() string { return s.App.AgentCommand() }, func(c string) { s.App.SetAgentCommand(c) })
}
