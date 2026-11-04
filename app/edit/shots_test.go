package edit

import (
	"testing"

	"aego/app/agentui/backend"
)

func TestDocumentationAgentNeedsNoExternalCLIAndCannotSend(t *testing.T) {
	t.Setenv("PATH", "/no-external-agent")
	if got := screenshotConfig(Config{}, ""); got.AgentBackend != nil {
		t.Fatal("normal editor configuration was replaced")
	}
	cfg := screenshotConfig(Config{}, t.TempDir())
	if version, err := cfg.AgentBackend.Available(backend.Config{Command: "not-installed"}); err != nil || version == "" {
		t.Fatalf("documentation presentation depends on an external CLI: %q %v", version, err)
	}
	if conversation, err := cfg.AgentBackend.Start(backend.Config{}); err == nil || conversation != nil {
		t.Fatal("documentation backend must not start or send to an agent")
	}
	explicit := &backend.Fake{}
	if got := screenshotConfig(Config{AgentBackend: explicit}, t.TempDir()); got.AgentBackend != explicit {
		t.Fatal("explicit caller backend was replaced")
	}
}
