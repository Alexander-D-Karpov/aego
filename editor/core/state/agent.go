package state

type agentField struct {
	name  string
	value *string
}

func agentFields(s *Editor) []agentField {
	return []agentField{
		{"agentCommand", &s.AgentCommand},
		{"agentProvider", &s.AgentProvider},
		{"agentCodexCommand", &s.AgentCodexCommand},
		{"agentClaudeModel", &s.AgentClaudeModel},
		{"agentCodexModel", &s.AgentCodexModel},
		{"agentCodexEffort", &s.AgentCodexEffort},
	}
}
