package integration

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"aego/app/agentui/backend"
	"aego/editor/core/agentui"
	"aego/editor/panels/welcome"
)

// This model fixture uses the real Codex CLI and editor MCP bridge.
const codexEditorCodeModeProgram = `
if (typeof process !== "undefined" || typeof require !== "undefined" || typeof fetch !== "undefined") throw Error("unexpected host access");
const forbidden = ALL_TOOLS.filter(t => /(^|__)(exec_command|shell|apply_patch|file_change)($|__)/.test(t.name) || t.name.includes("foreign"));
if (forbidden.length) throw Error("unexpected external tool: " + JSON.stringify(forbidden.map(t => t.name)));
function editorTool(suffix) {
  const found = ALL_TOOLS.filter(t => t.name.endsWith("__" + suffix));
  if (found.length !== 1 || !/^mcp__aego_editor_[0-9a-f]+__/.test(found[0].name)) throw Error("missing or ambiguous editor tool: " + suffix);
  return found[0].name;
}
const treeTool = editorTool("scene_tree");
const addTool = editorTool("scene_add_node");
if (treeTool.slice(0, -"scene_tree".length) !== addTool.slice(0, -"scene_add_node".length)) throw Error("tools from different MCP servers");
const tree = await tools[treeTool]({file: "scenes/main.aescene"});
if (tree.isError || !JSON.stringify(tree).includes("UnsavedCodeModeActor")) throw Error("editor did not return the dirty scene: " + JSON.stringify(tree));
text("DIRTY_SCENE_RESULT: " + JSON.stringify(tree));
const added = await tools[addTool]({file: "scenes/main.aescene", name: "CodeModeProposedNode"});
if (added.isError) throw Error("editor rejected the approved proposal: " + JSON.stringify(added));
text("APPROVED_EDIT_RESULT: " + JSON.stringify(added));
`

// Opt in with the actual installed CLI; no user credentials or external model are used.
func TestCodexCodeModeUsesRealEditorBridge(t *testing.T) {
	command := os.Getenv("AEGO_CODEX_PROTOCOL_CLI")
	if command == "" {
		t.Skip("set AEGO_CODEX_PROTOCOL_CLI to run the real CLI/local model integration test")
	}
	command, err := exec.LookPath(command)
	if err != nil {
		t.Fatal(err)
	}
	command, err = filepath.Abs(command)
	if err != nil {
		t.Fatal(err)
	}

	// Isolate CLI state and editor preferences; agentSession separately copies the project.
	home := t.TempDir()
	codexHome := filepath.Join(home, "codex")
	tmp := filepath.Join(home, "tmp")
	for _, dir := range []string{codexHome, tmp} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for k, v := range map[string]string{
		"HOME": home, "CODEX_HOME": codexHome, "TMPDIR": tmp,
		"XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_CACHE_HOME": filepath.Join(home, "cache"), "XDG_DATA_HOME": filepath.Join(home, "data"),
		"OPENAI_API_KEY": "", "CODEX_API_KEY": "", "OPENAI_BASE_URL": "", "CHATGPT_BASE_URL": "",
		"HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": "", "NO_PROXY": "*", "OTEL_SDK_DISABLED": "true",
	} {
		t.Setenv(k, v)
	}

	var requests, foreign, outputReceived atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Method != http.MethodPost {
			foreign.Add(1)
			t.Errorf("unexpected local model request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("local model received an Authorization header")
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
		if err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		type toolSpec struct {
			Type, Name string
			Tools      []struct{ Name string }
		}
		var req struct {
			Model string
			Tools []toolSpec
			Input []struct {
				Type   string
				Tools  []toolSpec
				Output json.RawMessage
			}
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("decode model request: %v", err)
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Model != "gpt-6.1-terra" {
			t.Errorf("model = %q, want code-mode gpt-6.1-terra", req.Model)
		}
		for _, input := range req.Input {
			if input.Type == "additional_tools" {
				req.Tools = append(req.Tools, input.Tools...)
			}
		}
		n := requests.Add(1)
		var item map[string]any
		var deltas []string
		switch n {
		case 1:
			execAdvertised := false
			for _, tool := range req.Tools {
				if tool.Type == "namespace" && tool.Name == "functions" {
					for _, entry := range tool.Tools {
						execAdvertised = execAdvertised || entry.Name == "exec"
					}
				}
			}
			if !execAdvertised {
				t.Errorf("functions.exec not advertised in code-mode tools: %+v", req.Tools)
			}
			item = map[string]any{"type": "custom_tool_call", "id": "fc-editor", "call_id": "call-editor", "name": "exec", "namespace": "functions", "input": codexEditorCodeModeProgram}
		case 2:
			for _, input := range req.Input {
				if input.Type == "custom_tool_call_output" && strings.Contains(string(input.Output), "DIRTY_SCENE_RESULT:") &&
					strings.Contains(string(input.Output), "UnsavedCodeModeActor") && strings.Contains(string(input.Output), "APPROVED_EDIT_RESULT:") {
					outputReceived.Store(1)
				}
			}
			if outputReceived.Load() == 0 {
				t.Errorf("real code-mode output did not contain both editor results: %+v", req.Input)
			}
			deltas = []string{"Local editor fixture completed."}
			item = map[string]any{"type": "message", "id": "msg-editor", "role": "assistant", "phase": "final_answer",
				"content": []any{map[string]string{"type": "output_text", "text": strings.Join(deltas, "")}}}
		default:
			t.Errorf("unexpected model request %d", n)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		event := func(v map[string]any) {
			b, _ := json.Marshal(v)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", v["type"], b)
		}
		response := fmt.Sprintf("resp-editor-%d", n)
		event(map[string]any{"type": "response.created", "response": map[string]string{"id": response}})
		event(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
		for _, delta := range deltas {
			event(map[string]any{"type": "response.output_text.delta", "item_id": item["id"], "output_index": 0, "content_index": 0, "delta": delta})
		}
		event(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
		event(map[string]any{"type": "response.completed", "response": map[string]any{"id": response, "status": "completed", "output": []any{item},
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 10, "total_tokens": 110, "input_tokens_details": map[string]int{"cached_tokens": 0}, "output_tokens_details": map[string]int{"reasoning_tokens": 0}}}})
	}))
	t.Cleanup(model.Close)
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
`, model.URL+"/v1", model.URL+"/foreign")
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	version, err := (backend.Codex{}).Available(backend.Config{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("real CLI:", version)

	s, root := agentSession(t, backend.Codex{})
	s.App.SetAgentSettings(agentui.ProviderSettings{Provider: "codex", Command: command, Model: "gpt-6.1-terra", Effort: "medium"})
	// Finish the checklist before measuring project writes.
	welcomePanel, ok := s.App.Workspace().Panel(welcome.ID)
	if !ok {
		t.Fatal("getting-started panel is missing")
	}
	checklist, ok := welcomePanel.(*welcome.Panel)
	if !ok {
		t.Fatal("unexpected getting-started panel type")
	}
	for _, id := range []string{"node", "attach", "play", "tweak", "reload", "error", "hint.errors"} {
		checklist.Mark(id)
	}
	// Wait for thumbnail caching and debounced workspace persistence before measuring writes.
	deadline := time.Now().Add(20 * time.Second)
	var idleSince time.Time
	for {
		s.Step()
		if s.App.Tasks().Busy() {
			idleSince = time.Time{}
		} else if idleSince.IsZero() {
			idleSince = time.Now()
		} else if time.Since(idleSince) >= 1500*time.Millisecond {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("editor background tasks did not become idle")
		}
		time.Sleep(time.Millisecond)
	}
	d := s.App.Context().Docs.Edit()
	scene := d.Scene()
	actor := scene.Find(scene.Root(), "Player")
	if actor.IsZero() {
		t.Fatal("fixture scene has no Player")
	}
	scene.SetName(actor, "UnsavedCodeModeActor")
	d.MarkDirty()
	if !d.Dirty() {
		t.Fatal("fixture scene must be dirty before the turn")
	}
	diskScene, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(bridgeScene)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(diskScene), "UnsavedCodeModeActor") || strings.Contains(string(diskScene), "CodeModeProposedNode") {
		t.Fatal("the scene markers must not already exist on disk")
	}
	before := codexProjectSnapshot(t, root)
	panel := s.App.Agent()
	panel.Submit("Read the dirty scene and propose adding CodeModeProposedNode.")

	// Include the panel transcript if initialization or a tool call fails.
	logs := func() []agentui.Entry {
		ids, err := agentui.List(root)
		if err != nil || len(ids) != 1 {
			t.Fatalf("session logs %v: %v", ids, err)
		}
		entries, err := agentui.Load(root, ids[0])
		if err != nil {
			t.Fatal(err)
		}
		return entries
	}
	wait := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(45 * time.Second)
		for !done() {
			if !panel.Busy() || time.Now().After(deadline) {
				t.Fatalf("waiting for %s (busy=%v, model requests=%d): %+v", what, panel.Busy(), requests.Load(), logs())
			}
			s.Step()
			time.Sleep(time.Millisecond)
		}
	}
	wait("the approval card", func() bool { return len(panel.Pending()) != 0 })
	if len(panel.Pending()) != 1 {
		t.Fatalf("pending cards %v, want one", panel.Pending())
	}
	card := panel.Pending()[0]
	if _, depth := d.History.Top(); depth != 0 || !scene.Find(scene.Root(), "CodeModeProposedNode").IsZero() {
		t.Fatal("the proposed node changed the scene before approval")
	}
	panel.Decide(card, true)
	wait("turn completion", func() bool { return !panel.Busy() })
	if label, depth := d.History.Top(); depth != 1 || !strings.HasPrefix(label, "Agent: ") {
		t.Fatalf("approved edit history = %q at depth %d", label, depth)
	}
	if scene.Find(scene.Root(), "CodeModeProposedNode").IsZero() {
		t.Fatal("approved proposal did not change the live scene")
	}
	if requests.Load() != 2 || foreign.Load() != 0 || outputReceived.Load() != 1 {
		t.Fatalf("model requests=%d foreign requests=%d editor output=%d", requests.Load(), foreign.Load(), outputReceived.Load())
	}
	var dirtyRead, pending, applied, answer bool
	for _, entry := range logs() {
		switch {
		case entry.Kind == "error":
			t.Fatalf("editor agent error: %s", entry.Text)
		case entry.Kind == "result" && entry.Failed:
			t.Fatalf("editor tool %s failed: %s", entry.Tool, entry.Text)
		case entry.Kind == "result" && entry.Tool == "scene_tree" && strings.Contains(entry.Text, "UnsavedCodeModeActor"):
			dirtyRead = true
		case entry.Kind == "card" && entry.Card == card && strings.Contains(entry.Text, "scene.add-node") && strings.Contains(entry.Text, "CodeModeProposedNode"):
			pending = pending || entry.State == "pending"
			applied = applied || entry.State == "applied"
		case entry.Kind == "text" && strings.Contains(entry.Text, "Local editor fixture completed."):
			answer = true
		}
	}
	if !dirtyRead || !pending || !applied || !answer {
		t.Fatalf("missing live dirty read/card/final answer: read=%v pending=%v applied=%v answer=%v; log=%+v", dirtyRead, pending, applied, answer, logs())
	}
	if err := panel.UndoCard(card); err != nil {
		t.Fatalf("Undo from the approval card: %v", err)
	}
	if node := scene.Find(scene.Root(), "CodeModeProposedNode"); !node.IsZero() && scene.IsAlive(node) {
		t.Fatal("Undo did not remove the approved node")
	}
	if !d.Dirty() || scene.Find(scene.Root(), "UnsavedCodeModeActor").IsZero() {
		t.Fatal("Undo lost the original unsaved scene edit")
	}
	after := codexProjectSnapshot(t, root)
	if !reflect.DeepEqual(before, after) {
		var changed []string
		for path, state := range before {
			if after[path] != state {
				changed = append(changed, path)
			}
		}
		for path := range after {
			if _, ok := before[path]; !ok {
				changed = append(changed, path)
			}
		}
		sort.Strings(changed)
		t.Fatalf("unexpected project file writes (excluding agent transcript): %v", changed)
	}
}

type codexProjectFile struct {
	Hash    [32]byte
	Mode    fs.FileMode
	ModTime time.Time
}

// Snapshot project files except the expected agent transcript.
func codexProjectSnapshot(t *testing.T, root string) map[string]codexProjectFile {
	t.Helper()
	files := map[string]codexProjectFile{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == agentui.Dir && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected non-regular project file %s (%s)", rel, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = codexProjectFile{Hash: sha256.Sum256(data), Mode: info.Mode(), ModTime: info.ModTime()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
