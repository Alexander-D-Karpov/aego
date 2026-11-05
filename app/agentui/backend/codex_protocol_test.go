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

// The real CLI tests its published protocol and tool inventory against local model/MCP fixtures with isolated config, not a paid model.
func TestCodexRealCLIWithLocalModelAndMCP(t *testing.T) {
	for _, model := range []string{"gpt-5.3-codex", "gpt-5.5", "gpt-6.1-terra", "gpt-5.6-sol"} {
		t.Run(model, func(t *testing.T) { codexLocalTransport(t, model) })
	}
}
func codexLocalTransport(t *testing.T, model string) {
	command := os.Getenv("AEGO_CODEX_PROTOCOL_CLI")
	if command == "" {
		t.Skip("set AEGO_CODEX_PROTOCOL_CLI for the real CLI/local model protocol test")
	}
	var requests, calls, probes, foreign, patchRejected atomic.Int32
	codeMode := model == "gpt-6.1-terra" || model == "gpt-5.6-sol"
	deferred := model == "gpt-5.5"
	home := t.TempDir()
	sentinel := filepath.Join(home, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("untouched\n"), 0600); err != nil {
		t.Fatal(err)
	}
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
					Type   string
					CallID string `json:"call_id"`
					Tools  []toolSpec
					Output json.RawMessage
				}
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Error(err)
				return
			}
			n := requests.Add(1)
			if deferred && n == 6 {
				for _, input := range req.Input {
					output := strings.ToLower(string(input.Output))
					if input.Type == "custom_tool_call_output" && input.CallID == "patch-local" &&
						(strings.Contains(output, "error") || strings.Contains(output, "denied") || strings.Contains(output, "no usable execution environment")) {
						patchRejected.Store(1)
					}
				}
			}
			for _, i := range req.Input {
				if i.Type == "additional_tools" || i.Type == "tool_search_output" {
					req.Tools = append(req.Tools, i.Tools...)
				}
			}
			namespace := ""
			for _, tool := range req.Tools {
				if deferred && tool.Type == "tool_search" {
					continue
				}
				if tool.Type == "namespace" && strings.HasPrefix(tool.Name, "mcp__aego_editor_") {
					namespace = tool.Name
					seen := map[string]bool{}
					for _, entry := range tool.Tools {
						seen[entry.Name] = true
					}
					if !seen["scene_tree"] || !deferred && (len(seen) != 5 || !seen["editor_scene_context"] || !seen["scene_read"] || !seen["script_read"] || !seen["project_validate"]) {
						t.Errorf("wrong editor tools: %+v", tool.Tools)
					}
					continue
				}
				if tool.Type == "namespace" && tool.Name == "functions" {
					for _, item := range tool.Tools {
						switch item.Name {
						case "get_goal", "update_goal", "create_goal", "request_user_input", "list_mcp_resources", "list_mcp_resource_templates", "read_mcp_resource":
						case "exec", "wait", "request_user_input_async":
							if !codeMode {
								t.Errorf("unexpected code-mode tool %s", item.Name)
							}
						default:
							t.Errorf("unexpected executable tool %s", item.Name)
						}
					}
					continue
				}
				switch tool.Name {
				case "list_mcp_resources", "list_mcp_resource_templates", "read_mcp_resource", "request_user_input", "get_goal", "create_goal", "update_goal":
				case "clock":
					if !codeMode || tool.Type != "namespace" {
						t.Errorf("unexpected namespace: %s (%s)", tool.Name, tool.Type)
					}
				case "apply_patch":
					if !deferred || tool.Type != "custom" {
						t.Errorf("unexpected patch tool: %s (%s)", tool.Name, tool.Type)
					}
				default:
					t.Errorf("unexpected tool outside editor bridge: %s (%s)", tool.Name, tool.Type)
				}
			}
			if namespace == "" && !codeMode && (!deferred || n > 1 && n < 5) {
				t.Error("editor MCP namespace missing from model request")
			}
			var item map[string]any
			var deltas []string
			if n == 1 && !strings.Contains(string(raw), "UnsavedCup") {
				t.Error("model did not receive the thread-bound scene snapshot")
			}
			if deferred && n == 5 {
				item = map[string]any{"type": "custom_tool_call", "id": "patch-local", "call_id": "patch-local", "name": "apply_patch", "input": "*** Begin Patch\n*** Update File: sentinel.txt\n@@\n-untouched\n+modified\n*** End Patch"}
			} else if n == 1 && deferred {
				item = map[string]any{"type": "tool_search_call", "id": "search-local", "call_id": "search-local", "execution": "client", "arguments": map[string]any{"query": "scene_tree"}}
			} else if n == 1 || n == 2 && deferred {
				if codeMode {
					code := `if (typeof process !== "undefined" || typeof require !== "undefined" || typeof fetch !== "undefined") throw Error("unexpected host access");
let imported = false;
try { await import("node:fs"); imported = true; } catch (_) {}
if (imported) throw Error("unexpected filesystem import");
const forbidden = ALL_TOOLS.filter(t => /(^|__)(exec_command|shell|apply_patch|file_change)($|__)/.test(t.name) || t.name.includes("foreign"));
if (forbidden.length) throw Error("unexpected external tool: " + JSON.stringify(forbidden.map(t => t.name)));
const tool = ALL_TOOLS.find(t => t.name.endsWith("__scene_tree"));
if (!tool) throw Error("editor scene_tree missing");
const result = await tools[tool.name]({});
for (const part of result.content || []) {
  if (part.type === "text") text(part.text);
  if (part.type === "image") image(part);
}`
					item = map[string]any{"type": "custom_tool_call", "id": "fc-local", "call_id": "call-local", "name": "exec", "namespace": "functions", "input": code}
				} else {
					item = map[string]any{"type": "function_call", "id": "fc-local", "call_id": "call-local", "name": "scene_tree", "namespace": namespace, "arguments": "{}"}
				}
			} else {
				firstResult, secondTurn := int32(2), int32(3)
				if deferred {
					firstResult, secondTurn = 3, 4
				}
				if n == firstResult && (!bytes.Contains(raw, []byte("fixture scene content")) || !bytes.Contains(raw, []byte("data:image/png;base64,"))) {
					for _, input := range req.Input {
						if len(input.Output) != 0 {
							t.Logf("tool output: %s", input.Output)
						}
					}
					t.Error("MCP text/image did not return to the model")
				}
				if n == secondTurn && !bytes.Contains(raw, []byte(screen64)) {
					t.Error("user screenshot did not reach the model")
				}
				if n == secondTurn && !bytes.Contains(raw, []byte("Real Codex ✓")) {
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
	config := fmt.Sprintf(`model_provider = "fixture"
model = %q
cli_auth_credentials_store = "file"
[features]
code_mode = { enabled = true, excluded_tool_namespaces = [], direct_only_tool_namespaces = ["functions"] }
code_mode_host = false
shell_tool = true
multi_agent_v2 = true
[agents]
enabled = true
[model_providers.fixture]
name = "local fixture"
base_url = %q
wire_api = "responses"
requires_openai_auth = false
supports_websockets = false
[mcp_servers.foreign]
url = %q
`, model, server.URL+"/v1", server.URL+"/foreign")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", home)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	cfg := Config{Binding: "binding-fixture", Command: command, Dir: home, MCPURL: server.URL + "/mcp", Token: "local-test-token", Model: model, Effort: "medium"}
	if model == "gpt-5.6-sol" {
		cfg.Model = "" // Exercise the CLI's configured default model.
	}
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
	wantRequests := int32(3)
	if deferred {
		wantRequests++
	}
	if probes.Load() != 2 || requests.Load() != wantRequests || calls.Load() != 1 || foreign.Load() != 0 || tools != 1 || text != "Real Codex ✓\nReal Codex ✓\n" {
		t.Fatalf("real CLI transport: requests=%d calls=%d foreign=%d tool events=%d text=%q", requests.Load(), calls.Load(), foreign.Load(), tools, text)
	}
	if deferred {
		if err := c.Send("Attempt a direct patch."); err != nil {
			t.Fatal(err)
		}
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		finished, blockedEvent := false, false
		for !finished {
			select {
			case event, ok := <-c.events:
				if ok && event.Kind == EvError && event.Err != nil && strings.Contains(event.Err.Error(), "project access outside the editor bridge") {
					blockedEvent = true
				}
				if !ok || event.Kind == EvError || event.Kind == EvTurnEnd {
					finished = true
				}
			case <-timer.C:
				t.Fatal("direct patch test timed out")
			}
		}
		c.Close()
		select {
		case <-c.done:
		case <-time.After(3 * time.Second):
			t.Fatal("Codex did not terminate after the patch attempt")
		}
		contents, err := os.ReadFile(sentinel)
		if err != nil || string(contents) != "untouched\n" || requests.Load() < 5 || !blockedEvent && patchRejected.Load() != 1 {
			t.Fatalf("direct patch was not confirmed blocked: contents=%q error=%v requests=%d event=%v rejected=%d", contents, err, requests.Load(), blockedEvent, patchRejected.Load())
		}
	}
}
