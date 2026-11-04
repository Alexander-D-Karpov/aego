package backend

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Opt in with AEGO_CODEX_PROTOCOL_CLI=/path/to/codex. The actual installed CLI
// talks only to these local model/MCP fixtures, using an isolated config home.
// This tests the published protocol and tool inventory, not a paid model.
func TestCodexRealCLIWithLocalModelAndMCP(t *testing.T) {
	for _, model := range []string{"gpt-5.3-codex", "gpt-6.1-terra"} {
		t.Run(model, func(t *testing.T) { codexLocalTransport(t, model) })
	}
}
func codexLocalTransport(t *testing.T, model string) {
	command := os.Getenv("AEGO_CODEX_PROTOCOL_CLI")
	if command == "" {
		t.Skip("set AEGO_CODEX_PROTOCOL_CLI for the real CLI/local model protocol test")
	}
	var requests, calls, probes, foreign atomic.Int32
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	encodedImage := base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	var screenshot bytes.Buffer
	if err := png.Encode(&screenshot, image.NewNRGBA(image.Rect(0, 0, 9, 7))); err != nil {
		t.Fatal(err)
	}
	screen64 := base64.StdEncoding.EncodeToString(screenshot.Bytes())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/foreign" {
			foreign.Add(1)
			http.Error(w, "disabled server called", 500)
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if r.URL.Path == "/v1/responses" {
			if r.Header.Get("Authorization") != "" {
				t.Error("local model received credentials")
			}
			type toolSpec struct {
				Type, Name string
				Tools      []struct{ Name string }
			}
			var req struct {
				Tools []toolSpec
				Input []struct {
					Type  string
					Tools []toolSpec
				}
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Error(err)
				return
			}
			for _, i := range req.Input {
				if i.Type == "additional_tools" {
					req.Tools = append(req.Tools, i.Tools...)
				}
			}
			namespace := ""
			for _, tool := range req.Tools {
				if tool.Type == "namespace" && strings.HasPrefix(tool.Name, "mcp__aego_editor_") {
					namespace = tool.Name
					seen := map[string]bool{}
					for _, entry := range tool.Tools {
						seen[entry.Name] = true
					}
					if len(seen) != 5 || !seen["editor_scene_context"] || !seen["scene_read"] || !seen["scene_tree"] || !seen["script_read"] || !seen["project_validate"] {
						t.Errorf("wrong editor tools: %+v", tool.Tools)
					}
					continue
				}
				if tool.Type == "namespace" && tool.Name == "functions" {
					for _, item := range tool.Tools {
						switch item.Name {
						case "get_goal", "update_goal", "create_goal", "request_user_input", "list_mcp_resources", "list_mcp_resource_templates", "read_mcp_resource":
						default:
							t.Errorf("unexpected executable tool %s", item.Name)
						}
					}
					continue
				}
				switch tool.Name {
				case "list_mcp_resources", "list_mcp_resource_templates", "read_mcp_resource", "request_user_input", "get_goal", "create_goal", "update_goal":
				default:
					t.Errorf("unexpected tool outside editor bridge: %s (%s)", tool.Name, tool.Type)
				}
			}
			if namespace == "" {
				t.Error("editor MCP namespace missing from model request")
			}
			n := requests.Add(1)
			var item map[string]any
			var deltas []string
			if n == 1 && !strings.Contains(string(raw), "UnsavedCup") {
				t.Error("model did not receive the thread-bound scene snapshot")
			}
			if n == 1 {
				item = map[string]any{"type": "function_call", "id": "fc-local", "call_id": "call-local", "name": "scene_tree", "namespace": namespace, "arguments": "{}"}
			} else {
				if n == 2 && (!bytes.Contains(raw, []byte("fixture scene content")) || !bytes.Contains(raw, []byte("data:image/png;base64,"))) {
					t.Error("MCP text/image did not return to the model")
				}
				if n == 3 && !bytes.Contains(raw, []byte(screen64)) {
					t.Error("user screenshot did not reach the model")
				}
				if n == 3 && !bytes.Contains(raw, []byte("Real Codex ✓")) {
					t.Error("second turn lost prior thread context")
				}
				deltas = []string{"Real ", "Codex ✓", "\n"}
				item = map[string]any{"type": "message", "id": fmt.Sprintf("msg-local-%d", n), "role": "assistant", "phase": "final_answer",
					"content": []any{map[string]string{"type": "output_text", "text": strings.Join(deltas, "")}}}
			}
			w.Header().Set("Content-Type", "text/event-stream")
			event := func(v map[string]any) {
				b, _ := json.Marshal(v)
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", v["type"], b)
			}
			response := fmt.Sprintf("resp-local-%d", n)
			event(map[string]any{"type": "response.created", "response": map[string]string{"id": response}})
			event(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
			for _, delta := range deltas {
				event(map[string]any{"type": "response.output_text.delta", "item_id": item["id"], "output_index": 0, "content_index": 0, "delta": delta})
			}
			event(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
			event(map[string]any{"type": "response.completed", "response": map[string]any{"id": response, "status": "completed", "output": []any{item},
				"usage": map[string]any{"input_tokens": 100, "output_tokens": 10, "total_tokens": 110, "input_tokens_details": map[string]int{"cached_tokens": 30}, "output_tokens_details": map[string]int{"reasoning_tokens": 0}}}})
			return
		}
		if r.Header.Get("Authorization") != "Bearer local-test-token" {
			t.Error("MCP bearer token missing")
		}
		var req codexEnvelope
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Error(err)
			return
		}
		if len(req.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any = map[string]any{}
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "serverInfo": map[string]string{"name": "aego", "version": "fixture"}, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "tools/list":
			var list []any
			for _, name := range []string{"scene_read", "editor_scene_context", "scene_tree", "script_read", "project_validate"} {
				list = append(list, map[string]any{"name": name, "description": "Read editor content", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"details": map[string]string{"type": "boolean"}}}})
			}
			result = map[string]any{"tools": list}
		case "resources/list":
			result = map[string]any{"resources": []any{}}
		case "resources/templates/list":
			result = map[string]any{"resourceTemplates": []any{}}
		case "tools/call":
			var call struct{ Name string }
			if err := json.Unmarshal(req.Params, &call); err != nil {
				t.Error(err)
				return
			}
			if call.Name == "editor_scene_context" {
				probes.Add(1)
				result = map[string]any{"content": []any{map[string]string{"type": "text", "text": `{"binding":"binding-fixture","document":"live.aescene","revision":8,"selected":["UnsavedCup"]}`}}}
				break
			}
			calls.Add(1)
			result = map[string]any{"content": []any{map[string]string{"type": "text", "text": "fixture scene content"}, map[string]string{"type": "image", "mimeType": "image/png", "data": encodedImage}}}
		default:
			t.Errorf("unexpected MCP request %s", req.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()
	home := t.TempDir()
	config := fmt.Sprintf(`model_provider = "fixture"
cli_auth_credentials_store = "file"
[model_providers.fixture]
name = "local fixture"
base_url = %q
wire_api = "responses"
requires_openai_auth = false
supports_websockets = false
[mcp_servers.foreign]
url = %q
`, server.URL+"/v1", server.URL+"/foreign")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", home)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	cfg := Config{Binding: "binding-fixture", Command: command, Dir: home, MCPURL: server.URL + "/mcp", Token: "local-test-token", Model: model, Effort: "medium"}
	version, err := (Codex{}).Available(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("real CLI:", version)
	conv, err := (Codex{}).Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c := conv.(*codexConv)
	defer func() {
		c.Close()
		select {
		case <-c.done:
		case <-time.After(3 * time.Second):
			t.Error("real Codex CLI did not terminate")
		}
	}()
	tools, text := 0, ""
	for i := 0; i < 2; i++ {
		var err error
		if i == 0 {
			err = c.Explain("Use scene_tree then reply briefly.", "live.aescene", 8)
		} else {
			err = c.SendImages("Use scene_tree then reply briefly.", [][]byte{screenshot.Bytes()})
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvTurnEnd }) {
			if event.Kind == EvToolResult {
				tools++
				if event.Failed || event.Result != "fixture scene content" {
					t.Fatalf("MCP result: %+v", event)
				}
			}
			if event.Kind == EvText {
				text += event.Text
			}
		}
	}
	if probes.Load() != 2 || requests.Load() != 3 || calls.Load() != 1 || foreign.Load() != 0 || tools != 1 || text != "Real Codex ✓\nReal Codex ✓\n" {
		t.Fatalf("real CLI transport: requests=%d calls=%d foreign=%d tool events=%d text=%q", requests.Load(), calls.Load(), foreign.Load(), tools, text)
	}
}
