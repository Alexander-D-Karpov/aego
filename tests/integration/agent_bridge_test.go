package integration

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
)

const bridgeScene = "scenes/main.aescene"

// A scripted agent edits the open scene through the editor's MCP server: two Undo steps, unsaved state readable, nothing on disk.
func TestAgentBridgeEditsTheOpenScene(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	d, b := e.doc, e.b
	events := e.turn(t, []backend.Step{
		{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "Ledge"}},
		{Tool: "scene_set", Args: map[string]any{"file": bridgeScene, "node": "Ledge", "path": "transform.position", "value": "[-13, 0]"}},
		{Tool: "scene_tree", Args: map[string]any{"file": bridgeScene}},
		{Text: "done"},
	}, e.approveAll)
	var tree string
	for _, e := range events {
		switch {
		case e.Kind == backend.EvError:
			t.Fatal(e.Err)
		case e.Kind == backend.EvToolResult && e.Failed:
			t.Fatalf("%s failed: %s", e.Tool, e.Result)
		case e.Kind == backend.EvToolResult && e.Tool == "scene_tree":
			tree = e.Result
		}
	}
	if !strings.Contains(tree, "Ledge") {
		t.Fatal("scene_tree did not see the unsaved node")
	}
	steps := d.History.Steps(nil)
	if len(steps) != 2 || !slices.ContainsFunc(steps, func(l string) bool { return strings.HasPrefix(l, "Agent: ") }) {
		t.Fatalf("history %v, want two agent steps", steps)
	}
	sc := d.Scene()
	h := sc.Find(sc.Root(), "Ledge")
	if h.IsZero() || sc.Local(h).Position.X != -13 {
		t.Fatal("the node is not in the editor scene")
	}
	disk, _ := os.ReadFile(filepath.Join(e.root, bridgeScene))
	if strings.Contains(string(disk), "Ledge") {
		t.Fatal("the agent wrote the open scene to disk")
	}
	for range 2 {
		if err := d.History.Undo(); err != nil {
			t.Fatal(err)
		}
	}
	if h := sc.Find(sc.Root(), "Ledge"); !h.IsZero() && sc.IsAlive(h) {
		t.Fatal("two Undo steps did not remove the node")
	}
	resp, err := http.Post(b.Backend().MCPURL, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a call without the token got %d", resp.StatusCode)
	}
	if !strings.HasPrefix(b.Backend().MCPURL, "http://127.0.0.1:") {
		t.Fatalf("the server listens on %s", b.Backend().MCPURL)
	}
}
