package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/editor/core/agentui"
	"aego/editor/panels/scripteditor"
)

// The test executable supplies the app-server protocol, but tools still cross
// the real authenticated HTTP bridge and execute on the editor's main thread.
// No provider credentials or model service are used by these tests.
func init() {
	if os.Getenv("AEGO_TEST_CODEX_EDITOR_PROCESS") != "1" {
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("codex-cli 0.166.0")
		os.Exit(0)
	}
	if err := runEditorCodexProcess(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

type editorCodexWire struct {
	mu sync.Mutex
	e  *json.Encoder
}

func (w *editorCodexWire) send(v any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.e.Encode(v)
}

func (w *editorCodexWire) notify(method string, params any) {
	w.send(map[string]any{"method": method, "params": params})
}

func runEditorCodexProcess() error {
	w := &editorCodexWire{e: json.NewEncoder(os.Stdout)}
	d := json.NewDecoder(os.Stdin)
	const thread = "codex-editor-integration"
	var server string
	var client *backend.Client
	var cancel context.CancelFunc
	var turn string
	count := 0
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()
	for {
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := d.Decode(&msg); err != nil {
			return nil
		}
		reply := func(result any) { w.send(map[string]any{"id": msg.ID, "result": result}) }
		switch msg.Method {
		case "initialize":
			reply(map[string]any{"userAgent": "codex-editor-test/0.166.0"})
		case "initialized":
		case "config/read":
			reply(map[string]any{"config": map[string]any{"mcp_servers": map[string]any{}}})
		case "thread/start":
			var p struct {
				Config struct {
					Servers map[string]struct {
						URL   string `json:"url"`
						Token string `json:"bearer_token_env_var"`
					} `json:"mcp_servers"`
				} `json:"config"`
			}
			if err := json.Unmarshal(msg.Params, &p); err != nil {
				return err
			}
			for name, cfg := range p.Config.Servers {
				if cfg.URL != "" && strings.HasPrefix(name, "aego_editor_") {
					server = name
					client = backend.NewClient(cfg.URL, os.Getenv(cfg.Token))
				}
			}
			if client == nil || client.Token == "" {
				return fmt.Errorf("no authenticated editor MCP server in thread/start")
			}
			reply(map[string]any{
				"thread": map[string]any{"id": thread, "environments": []any{}}, "model": "codex-editor-test",
				"sandbox": map[string]any{"type": "readOnly"}, "approvalPolicy": "never",
			})
		case "mcpServerStatus/list":
			reply(map[string]any{"data": []any{map[string]any{
				"name": server, "runtimeStatus": "connected", "tools": map[string]any{
					"scene_add_node":       map[string]any{"name": "scene_add_node"},
					"scene_tree":           map[string]any{"name": "scene_tree"},
					"editor_scene_context": map[string]any{"name": "editor_scene_context"}, "script_read": map[string]any{"name": "script_read"},
					"project_validate": map[string]any{"name": "project_validate"},
				},
			}}, "nextCursor": nil})
		case "model/list":
			reply(map[string]any{"data": []any{map[string]any{"model": "codex-editor-test", "inputModalities": []string{"text", "image"}}}})
		case "mcpServer/tool/call":
			var request struct {
				Thread       string `json:"threadId"`
				Server, Tool string
				Arguments    map[string]any
			}
			if err := json.Unmarshal(msg.Params, &request); err != nil {
				return err
			}
			if request.Thread != thread || request.Server != server || request.Tool != "editor_scene_context" {
				return fmt.Errorf("unexpected diagnostic tool request")
			}
			out, err := client.CallRaw(request.Tool, request.Arguments)
			if err != nil {
				return err
			}
			reply(out)
		case "turn/start":
			if client == nil {
				return fmt.Errorf("turn started before editor tools were configured")
			}
			var p struct {
				Thread string                        `json:"threadId"`
				Input  []struct{ Type, Text string } `json:"input"`
			}
			if err := json.Unmarshal(msg.Params, &p); err != nil || p.Thread != thread || len(p.Input) == 0 {
				return fmt.Errorf("invalid turn input")
			}
			count++
			turn = fmt.Sprintf("turn-%d", count)
			if cancel != nil {
				cancel()
			}
			ctx, cancelTurn := context.WithCancel(context.Background())
			cancel = cancelTurn
			t := map[string]any{"id": turn, "status": "inProgress", "items": []any{}}
			reply(map[string]any{"turn": t})
			w.notify("turn/started", map[string]any{"threadId": thread, "turn": t})
			go editorCodexTurn(ctx, cancelTurn, w, client, server, thread, turn, p.Input[0].Text)
		case "turn/interrupt":
			if cancel != nil {
				cancel()
			}
			reply(map[string]any{})
			w.notify("turn/completed", map[string]any{"threadId": thread, "turn": map[string]any{"id": turn, "status": "interrupted", "items": []any{}}})
		default:
			if len(msg.ID) > 0 {
				return fmt.Errorf("unexpected app-server request %q", msg.Method)
			}
		}
	}
}

func editorCodexTurn(ctx context.Context, cancel context.CancelFunc, w *editorCodexWire, client *backend.Client, server, thread, turn, prompt string) {
	defer cancel()
	name := "CodexBridgeNode"
	if strings.Contains(prompt, "reject") {
		name = "RejectedCodexNode"
	} else if strings.Contains(prompt, "cancel") {
		name = "CancelledCodexNode"
	}
	tools := []struct {
		name string
		args map[string]any
	}{
		{"scene_add_node", map[string]any{"file": bridgeScene, "name": name}},
		{"scene_tree", map[string]any{"file": bridgeScene}},
	}
	if strings.Contains(prompt, "Current editor document (project data):") {
		tools[0].name = "script_read"
		tools[0].args = map[string]any{"file": "scripts/player/player.aes"}
		if !strings.Contains(prompt, "UnsavedActor212") || !strings.Contains(prompt, "UNSAVED_SCRIPT_212") {
			return
		}
	}
	for i, tool := range tools {
		if ctx.Err() != nil {
			return
		}
		item := map[string]any{"type": "mcpToolCall", "id": fmt.Sprintf("%s-tool-%d", turn, i), "server": server,
			"tool": tool.name, "arguments": tool.args, "status": "inProgress"}
		w.notify("item/started", map[string]any{"threadId": thread, "turnId": turn, "item": item})
		raw, err := client.CallRaw(tool.name, tool.args)
		if ctx.Err() != nil {
			return
		}
		item["status"] = "completed"
		if err != nil {
			item["status"] = "failed"
			item["error"] = map[string]any{"message": err.Error()}
		} else {
			item["result"] = raw
			var result struct {
				IsError bool `json:"isError"`
			}
			_ = json.Unmarshal(raw, &result)
			if result.IsError {
				item["status"] = "failed"
			}
		}
		w.notify("item/completed", map[string]any{"threadId": thread, "turnId": turn, "item": item})
	}
	message := turn + "-answer"
	prefix, reply := "Готово: ", name
	if strings.Contains(prompt, "Current editor document (project data):") {
		prefix, reply = "Fixture read UnsavedActor212 and its attached script: ", "scripts/player/player.aes."
	}
	for _, text := range []string{prefix, reply} {
		w.notify("item/agentMessage/delta", map[string]any{"threadId": thread, "turnId": turn, "itemId": message, "delta": text})
	}
	w.notify("item/completed", map[string]any{"threadId": thread, "turnId": turn,
		"item": map[string]any{"type": "agentMessage", "id": message, "text": prefix + reply}})
	w.notify("turn/completed", map[string]any{"threadId": thread, "turn": map[string]any{"id": turn, "status": "completed", "items": []any{}}})
}

func codexEditorProcess(t *testing.T) string {
	t.Helper()
	t.Setenv("AEGO_TEST_CODEX_EDITOR_PROCESS", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestCodexPanelToolsUseApprovalHistoryAndDirtyDocument(t *testing.T) {
	command := codexEditorProcess(t)
	s, root := agentSession(t, backend.Codex{})
	s.App.SetAgentSettings(agentui.ProviderSettings{Provider: "codex", Command: command})
	panel := s.App.Agent()
	d := s.App.Context().Docs.Edit()
	before, err := os.ReadFile(filepath.Join(root, bridgeScene))
	if err != nil {
		t.Fatal(err)
	}
	panel.Submit("add a node")
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 })
	if _, depth := d.History.Top(); depth != 0 || !d.Scene().Find(d.Scene().Root(), "CodexBridgeNode").IsZero() {
		t.Fatal("Codex changed the document before Apply")
	}
	panel.Decide(panel.Pending()[0], true)
	stepUntil(t, s, func() bool { return !panel.Busy() })
	if label, depth := d.History.Top(); depth != 1 || !strings.HasPrefix(label, "Agent: ") {
		t.Fatalf("applied Codex edit has history %q at %d", label, depth)
	}
	if d.Scene().Find(d.Scene().Root(), "CodexBridgeNode").IsZero() {
		t.Fatal("the approved edit did not reach the open scene")
	}
	ids, err := agentui.List(root)
	if err != nil || len(ids) != 1 {
		t.Fatalf("session logs %v: %v", ids, err)
	}
	entries, err := agentui.Load(root, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	readDirty := false
	for _, entry := range entries {
		if entry.Kind == "result" && entry.Tool == "scene_tree" && strings.Contains(entry.Text, "CodexBridgeNode") {
			readDirty = true
		}
	}
	if !readDirty {
		t.Fatal("Codex's real scene_tree call did not read the unsaved edit")
	}
	panel.Submit("reject the next proposed node")
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 })
	panel.Decide(panel.Pending()[0], false)
	stepUntil(t, s, func() bool { return !panel.Busy() })
	if _, depth := d.History.Top(); depth != 1 || !d.Scene().Find(d.Scene().Root(), "RejectedCodexNode").IsZero() {
		t.Fatal("a rejected second turn changed the scene or History")
	}
	after, err := os.ReadFile(filepath.Join(root, bridgeScene))
	if err != nil || string(before) != string(after) {
		t.Fatal("Codex wrote the open scene to disk")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if node := d.Scene().Find(d.Scene().Root(), "CodexBridgeNode"); !node.IsZero() && d.Scene().IsAlive(node) {
		t.Fatal("Undo did not remove Codex's approved node")
	}
}

func TestCodexNewSessionCancelsPendingEdit(t *testing.T) {
	command := codexEditorProcess(t)
	s, _ := agentSession(t, backend.Codex{})
	s.App.SetAgentSettings(agentui.ProviderSettings{Provider: "codex", Command: command})
	panel := s.App.Agent()
	d := s.App.Context().Docs.Edit()
	panel.Submit("cancel the pending node")
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 })
	panel.NewSession()
	stepUntil(t, s, func() bool { return !panel.Busy() && len(panel.Pending()) == 0 })
	panel.Submit("add a node in the new session")
	stepUntil(t, s, func() bool { return len(panel.Pending()) == 1 })
	panel.Decide(panel.Pending()[0], true)
	stepUntil(t, s, func() bool { return !panel.Busy() })
	if !d.Scene().Find(d.Scene().Root(), "CancelledCodexNode").IsZero() {
		t.Fatal("the cancelled conversation applied a stale edit")
	}
	if d.Scene().Find(d.Scene().Root(), "CodexBridgeNode").IsZero() {
		t.Fatal("the new conversation did not use the editor bridge")
	}
	if _, depth := d.History.Top(); depth != 1 {
		t.Fatalf("History depth %d, want only the new session's approved edit", depth)
	}
}

// Opt-in only: this test uses the user's installed CLI and calls a real model.
func TestCodexCallsAnEditorTool(t *testing.T) {
	if os.Getenv("AEGO_CODEX_SMOKE") != "1" {
		t.Skip("set AEGO_CODEX_SMOKE=1 to call a real Codex model")
	}
	command := os.Getenv("AEGO_CODEX_COMMAND")
	if command == "" {
		var err error
		command, err = exec.LookPath("codex")
		if err != nil {
			t.Skip("codex is not in PATH")
		}
	}
	e := bridgeSetup(t, agentbridge.Config{})
	cfg := e.b.Backend()
	cfg.Command = command
	be := backend.Codex{}
	if version, err := be.Available(cfg); err != nil {
		t.Fatal(err)
	} else {
		t.Log("codex", version)
	}
	conv, err := be.Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer conv.Close()
	if err := conv.Send("Call scene_tree once for scenes/main.aescene, then answer with the top-level node names only."); err != nil {
		t.Fatal(err)
	}
	called, ended := false, false
	var answer strings.Builder
	for _, event := range drainTurn(t, e.s, conv) {
		switch event.Kind {
		case backend.EvError:
			t.Fatal(event.Err)
		case backend.EvToolStart:
			called = called || event.Tool == "scene_tree"
		case backend.EvToolResult:
			if event.Failed {
				t.Fatalf("the editor tool failed: %s", event.Result)
			}
		case backend.EvText:
			answer.WriteString(event.Text)
		case backend.EvTurnEnd:
			ended = true
		}
	}
	if !called || !ended || !strings.Contains(answer.String(), "Player") {
		t.Fatalf("scene_tree called %v, turn ended %v, answer %q", called, ended, answer.String())
	}
}

func TestExplainQuickActionReadsDirtySceneAndScriptWithoutWriting(t *testing.T) {
	command := codexEditorProcess(t)
	s, root := agentSession(t, backend.Codex{})
	s.App.SetAgentSettings(agentui.ProviderSettings{Provider: "codex", Command: command})
	ctx := s.App.Context()
	// Measure writes by the read tools independently of the user's time-based script autosave.
	code := ctx.Code.(*scripteditor.Host)
	code.Autosave = false
	const scriptPath = "scripts/player/player.aes"
	before := map[string][]byte{}
	for _, name := range []string{bridgeScene, scriptPath} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		before[name] = data
	}
	d := ctx.Doc()
	sc := d.Scene()
	n := sc.Find(sc.Root(), "Player")
	if n.IsZero() {
		t.Fatal("missing Player")
	}
	sc.SetName(n, "UnsavedActor212")
	ctx.Select(sc.ID(n))
	script, err := ctx.Docs.OpenScript(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script.Code.Replace(0, 0, "// UNSAVED_SCRIPT_212\n")
	p := s.App.Agent()
	p.Recheck()
	s.App.Workspace().Focus("agent")
	s.App.Workspace().ToggleMaximize("agent")
	stepUntil(t, s, func() bool { return p.ExplainRect().W > 0 })
	clickTile(t, s, center(p.ExplainRect()), 0)
	stepUntil(t, s, func() bool { return !p.Busy() })
	ids, err := agentui.List(root)
	if err != nil || len(ids) != 1 {
		t.Fatalf("sessions %v %v", ids, err)
	}
	entries, err := agentui.Load(root, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	var read, tree bool
	for _, entry := range entries {
		if entry.Kind == "result" && entry.Tool == "script_read" && strings.Contains(entry.Text, "UNSAVED_SCRIPT_212") {
			read = true
		}
		if entry.Kind == "result" && entry.Tool == "scene_tree" && strings.Contains(entry.Text, "UnsavedActor212") {
			tree = true
		}
	}
	if !tree || !read {
		t.Fatalf("missing current scene or script in live tool results: %+v", entries)
	}
	if _, n := d.History.Top(); n != 0 || len(p.Pending()) != 0 {
		t.Fatal("read-only quick action changed History")
	}
	for name, want := range before {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, want) {
			t.Fatalf("read-only quick action changed %s on disk", name)
		}
	}
	if !script.Dirty() {
		t.Fatal("reading the document discarded its unsaved status")
	}
	// Positive control: with automatic saving enabled, the editor saves the same buffer
	// at its normal idle deadline even when there is no agent turn.
	code.Autosave = true
	now := s.Engine.Platform().Now()
	code.Tick(now)
	code.Tick(now + scripteditor.AutosaveDelay + 0.01)
	data, err := os.ReadFile(filepath.Join(root, scriptPath))
	if err != nil {
		t.Fatal(err)
	}
	if script.Dirty() || !bytes.Equal(data, script.Code.Bytes()) {
		t.Fatal("normal script autosave did not write the edited buffer")
	}
}
