package state

import (
	"strings"
	"testing"
)

func TestAgentProvidersPreserveLegacyAndSeparatePreferences(t *testing.T) {
	legacy, err := Decode([]byte("editor version=1 { agentCommand=\"/old/claude\" }"), EditorFile)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.AgentCommand != "/old/claude" || legacy.AgentProvider != "" || legacy.AgentCodexCommand != "" {
		t.Fatalf("legacy preferences changed: %+v", legacy)
	}
	legacy.AgentProvider = "codex"
	legacy.AgentCodexCommand = "/new/codex"
	legacy.AgentClaudeModel = "claude-configured-model"
	legacy.AgentCodexModel = "codex-configured-model"
	legacy.AgentCodexEffort = "high"
	root := t.TempDir()
	if err := Save(root, legacy); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentCommand != legacy.AgentCommand || got.AgentCodexCommand != legacy.AgentCodexCommand ||
		got.AgentProvider != "codex" || got.AgentClaudeModel != legacy.AgentClaudeModel ||
		got.AgentCodexModel != legacy.AgentCodexModel || got.AgentCodexEffort != "high" {
		t.Fatalf("provider preferences did not round-trip: %+v", got)
	}
	if strings.Contains(string(Encode(Default())), "agent") {
		t.Fatal("default agent preferences should not change legacy editor files")
	}
}
