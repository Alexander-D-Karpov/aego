package backend

import (
	"strings"
	"testing"
)

func TestClaudeDoesNotConfuseTextSessionWithSceneAccess(t *testing.T) {
	c := &claudeConv{}
	c.status([]byte(`{"type":"system","subtype":"init","session_id":"one","model":"fixture","mcp_servers":[{"name":"aego","status":"connected"}],"tools":["mcp__aego__scene_tree"]}`))
	if d := c.Connection(); d.Tools != 1 || d.State != "missing tools" || d.Thread != "one" {
		t.Fatal(d)
	}
	c.status([]byte(`{"type":"control_response","response":{"request_id":"aego-status","subtype":"success","response":{"mcpServers":[{"name":"aego","status":"connected","tools":[{"name":"editor_scene_context"},{"name":"scene_tree"},{"name":"script_read"}]}]}}}`))
	if d := c.Connection(); d.State != "tools listed" || d.Tools != 3 {
		t.Fatal(d)
	}
}

func TestCodexRefusesMissingToolAndStaleEditorBinding(t *testing.T) {
	for _, tt := range []struct{ mode, state, reason string }{{"missing-scene-tool", "missing tools", "scene_tree"}, {"missing-context", "missing tools", "editor_scene_context"}, {"old-editor", "call rejected", "different editor session"}} {
		t.Run(tt.mode, func(t *testing.T) {
			c := startCodexFixture(t, tt.mode, func(cfg *Config) { cfg.Binding = "new-editor" })
			evs := codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvError })
			err := evs[len(evs)-1].Err
			if err == nil || !strings.Contains(err.Error(), tt.reason) || c.Connection().State != tt.state {
				t.Fatalf("%+v: %v", c.Connection(), err)
			}
		})
	}
}
