package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// A subprocess fixture exercises pipes, multiplexed RPC, cancellation and reap;
// it does not need provider credentials or a model connection.
func init() {
	if os.Getenv("AEGO_CODEX_BACKEND_FIXTURE") == "" {
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("codex-cli 0.166.0")
		os.Exit(0)
	}
	if err := codexFixture(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func codexFixture() error {
	mode := os.Getenv("AEGO_CODEX_BACKEND_FIXTURE")
	token := os.Getenv(codexTokenEnv)
	if strings.Contains(strings.Join(os.Args, " "), token) || token == "" {
		return fmt.Errorf("bridge token leaked to argv or missing")
	}
	for _, arg := range codexArgs()[3:] {
		if !strings.Contains(strings.Join(os.Args, " "), arg) {
			return fmt.Errorf("missing app-server restriction %s", arg)
		}
	}
	var mu sync.Mutex
	enc, dec := json.NewEncoder(os.Stdout), json.NewDecoder(os.Stdin)
	send := func(v any) { mu.Lock(); defer mu.Unlock(); _ = enc.Encode(v) }
	notify := func(method string, params any) { send(map[string]any{"method": method, "params": params}) }
	thread, turn, server := "fixture-thread", "", ""
	count := 0
	item := func(method string, v any) {
		notify(method, map[string]any{"threadId": thread, "turnId": turn, "item": v})
	}
	complete := func(status string) {
		notify("turn/completed", map[string]any{"threadId": thread, "turn": map[string]any{"id": turn, "status": status, "durationMs": 17}})
	}
	for {
		var m codexEnvelope
		if err := dec.Decode(&m); err != nil {
			return nil
		}
		reply := func(v any) { send(map[string]any{"id": m.ID, "result": v}) }
		switch m.Method {
		case "initialize":
			reply(map[string]any{"userAgent": "fixture/0.166.0"})
			// A duplicate response must not block the notification reader.
			reply(map[string]any{"userAgent": "fixture/0.166.0"})
		case "initialized":
		case "config/read":
			reply(map[string]any{"config": map[string]any{"mcp_servers": map[string]any{
				"inherited": map[string]any{"url": "https://unused.invalid", "http_headers": map[string]string{"Authorization": "config-secret"}},
			}}})
		case "thread/start":
			var p struct {
				Model, Sandbox, ApprovalPolicy string
				Environments                   []any
				Config                         struct {
					Servers map[string]map[string]any `json:"mcp_servers"`
				}
			}
			if err := json.Unmarshal(m.Params, &p); err != nil {
				return err
			}
			if p.Sandbox != "read-only" || p.ApprovalPolicy != "never" || p.Environments == nil || len(p.Environments) != 0 || p.Model != "fixture-model" {
				return fmt.Errorf("incorrect thread sandbox, environment or model")
			}
			if p.Config.Servers["inherited"]["enabled"] != false {
				return fmt.Errorf("inherited server was not disabled")
			}
			for name, cfg := range p.Config.Servers {
				if strings.HasPrefix(name, "aego_editor_") && cfg["enabled"] == true && cfg["bearer_token_env_var"] == codexTokenEnv && cfg["default_tools_approval_mode"] == "approve" {
					server = name
				}
			}
			if server == "" {
				return fmt.Errorf("editor MCP server was not configured")
			}
			th := map[string]any{"id": thread, "environments": []any{}}
			if mode == "missing-environments" {
				delete(th, "environments")
			}
			reply(map[string]any{"thread": th, "model": "fixture-model", "sandbox": map[string]string{"type": "readOnly"}})
		case "mcpServerStatus/list":
			status := "disabled"
			if mode == "foreign" {
				status = "connected"
			}
			tools := map[string]any{"scene_tree": map[string]any{}, "script_read": map[string]any{}, "project_validate": map[string]any{}, "editor_scene_context": map[string]any{}}
			if mode == "missing-scene-tool" {
				delete(tools, "scene_tree")
			}
			if mode == "empty-catalog" {
				clear(tools)
			}
			if mode == "missing-read" {
				delete(tools, "script_read")
			}
			if mode == "missing-validate" {
				delete(tools, "project_validate")
			}
			if mode == "missing-context" {
				delete(tools, "editor_scene_context")
			}
			reply(map[string]any{"data": []any{
				map[string]any{"name": server, "runtimeStatus": "connected", "tools": tools},
				map[string]string{"name": "inherited", "runtimeStatus": status},
			}})
			if mode == "no-read" {
				time.Sleep(time.Hour)
			}
		case "mcpServer/tool/call":
			var q struct{ ThreadID, Server, Tool string }
			if err := json.Unmarshal(m.Params, &q); err != nil || q.ThreadID != thread || q.Server != server || q.Tool != "editor_scene_context" {
				return fmt.Errorf("wrong MCP thread binding")
			}
			reply(map[string]any{"content": []any{map[string]string{"type": "text", "text": `{"binding":"old-editor"}`}}})
		case "turn/start":
			var p struct {
				ThreadID, Effort string
				Input            []struct{ Text string }
			}
			if err := json.Unmarshal(m.Params, &p); err != nil || p.ThreadID != thread || p.Effort != "high" || len(p.Input) != 1 {
				return fmt.Errorf("incorrect turn or effort")
			}
			count++
			turn = fmt.Sprintf("turn-%d", count)
			if mode == "exit" {
				return fmt.Errorf("auth error: %s", token)
			}
			if mode == "eof-live" {
				os.Stdout.Close()
				time.Sleep(time.Hour)
			}
			if mode == "start-hangs" || mode == "late-start" {
				notify("thread/tokenUsage/updated", map[string]any{"threadId": thread, "tokenUsage": map[string]any{"last": map[string]int{"inputTokens": 1}}})
				if mode == "start-hangs" {
					continue
				}
				time.Sleep(100 * time.Millisecond)
			}
			t := map[string]string{"id": turn, "status": "inProgress"}
			reply(map[string]any{"turn": t})
			notify("turn/started", map[string]any{"threadId": thread, "turn": t})
			if mode == "late-start" {
				continue
			}
			if mode == "interrupt-hangs" || p.Input[0].Text == "wait" {
				notify("item/agentMessage/delta", map[string]string{"threadId": thread, "turnId": turn, "itemId": "waiting", "delta": "waiting"})
				continue
			}
			if p.Input[0].Text == "question" {
				send(map[string]any{"id": "question-1", "method": "item/tool/requestUserInput", "params": map[string]any{
					"threadId": thread, "turnId": turn, "itemId": "question-item", "isBlocking": true,
					"questions": []any{map[string]any{"id": "color", "header": "Color", "question": "Which color?", "isOther": true,
						"options": []any{map[string]string{"label": "Blue", "description": "Choose blue"}}}},
				}})
				continue
			}
			if p.Input[0].Text == "approval" {
				send(map[string]any{"id": "approval-1", "method": "item/fileChange/requestApproval", "params": map[string]string{"threadId": thread, "turnId": turn}})
				continue
			}
			if p.Input[0].Text == "failure" {
				notify("error", map[string]any{"threadId": thread, "turnId": turn, "willRetry": false, "error": map[string]string{"message": "Please run codex login"}})
				complete("failed")
				continue
			}
			if p.Input[0].Text == "flood" {
				for i := 0; i < 10000; i++ {
					notify("item/agentMessage/delta", map[string]string{"threadId": thread, "turnId": turn, "itemId": "flood", "delta": "x"})
				}
				continue
			}
			item("item/started", map[string]any{"id": "tool", "type": "mcpToolCall", "server": server, "tool": "scene_tree", "arguments": map[string]string{"file": "main.aescene"}})
			item("item/completed", map[string]any{"id": "tool", "type": "mcpToolCall", "server": server, "tool": "scene_tree", "status": "failed",
				"result": map[string]any{"content": []any{map[string]string{"type": "text", "text": "fixture rejection"}}, "isError": true}})
			for _, delta := range []string{"Привет", ", ", "мир 👩‍💻\n"} {
				notify("item/agentMessage/delta", map[string]string{"threadId": thread, "turnId": turn, "itemId": turn + "-text", "delta": delta})
			}
			item("item/completed", map[string]string{"type": "agentMessage", "id": turn + "-text", "text": "Привет, мир 👩‍💻\n"})
			item("item/completed", map[string]string{"type": "agentMessage", "id": turn + "-whole", "text": "whole message"})
			notify("thread/tokenUsage/updated", map[string]any{"threadId": thread, "turnId": turn, "tokenUsage": map[string]any{
				"last": map[string]int{"inputTokens": 100, "cachedInputTokens": 70}, "modelContextWindow": 200000,
			}})
			item("item/completed", map[string]string{"id": "compact", "type": "contextCompaction"})
			complete("completed")
		case "turn/interrupt":
			reply(map[string]any{})
			if mode != "interrupt-hangs" {
				complete("interrupted")
			}
		case "":
			if string(m.ID) == `"question-1"` {
				if m.Error == nil {
					item("item/completed", map[string]string{"id": "answer", "type": "agentMessage", "text": string(m.Result)})
					complete("completed")
				}
			} else if string(m.ID) == `"approval-1"` {
				if string(m.Result) != `{"decision":"decline"}` {
					return fmt.Errorf("editor bypass was approved: %s", m.Result)
				}
				complete("completed")
			}
		default:
			return fmt.Errorf("unexpected request %s", m.Method)
		}
	}
}

func startCodexFixture(t *testing.T, mode string, edit func(*Config)) *codexConv {
	t.Helper()
	t.Setenv("AEGO_CODEX_BACKEND_FIXTURE", mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Command: exe, Dir: t.TempDir(), MCPURL: "http://127.0.0.1/fixture", Token: "fixture-token", Model: "fixture-model", Effort: "high"}
	if edit != nil {
		edit(&cfg)
	}
	conv, err := (Codex{}).Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c := conv.(*codexConv)
	t.Cleanup(func() {
		c.Close()
		select {
		case <-c.done:
		case <-time.After(2 * time.Second):
			t.Error("Codex subprocess/workers did not terminate")
		}
	})
	return c
}

func codexEventUntil(t *testing.T, c *codexConv, predicate func(Event) bool) []Event {
	t.Helper()
	var events []Event
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e, ok := <-c.Events():
			if !ok {
				t.Fatalf("Codex closed before expected event: %+v", events)
			}
			events = append(events, e)
			if predicate(e) {
				return events
			}
			if e.Kind == EvError {
				t.Fatalf("Codex: %v", e.Err)
			}
		case <-timer.C:
			t.Fatalf("Codex event timed out: %+v", events)
		}
	}
}

func TestCodexThreadStreamingToolsAndUsage(t *testing.T) {
	var record bytes.Buffer
	c := startCodexFixture(t, "standard", func(cfg *Config) { cfg.Record = &record })
	for turn := 1; turn <= 2; turn++ {
		if err := c.Send("hello"); err != nil {
			t.Fatal(err)
		}
		events := codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvTurnEnd })
		text, whole, starts, results, compactions, usage := "", 0, 0, 0, 0, 0
		for _, e := range events {
			switch e.Kind {
			case EvText:
				if e.Delta {
					text += e.Text
					if e.MessageID != fmt.Sprintf("turn-%d-text", turn) {
						t.Fatal(e)
					}
				} else {
					whole++
					if e.Text != "whole message" {
						t.Fatal(e)
					}
				}
			case EvToolStart:
				starts++
				if e.Tool != "scene_tree" {
					t.Fatal(e)
				}
			case EvToolResult:
				results++
				if !e.Failed || e.Result != "fixture rejection" {
					t.Fatal(e)
				}
			case EvCompact:
				compactions++
			case EvUsage:
				if e.Tokens != 0 {
					usage++
					if e.Tokens != 100 || e.Window != 200000 {
						t.Fatal(e)
					}
				}
			case EvTurnEnd:
				if e.Failed || e.Millis != 17 {
					t.Fatal(e)
				}
			}
		}
		if text != "Привет, мир 👩‍💻\n" || whole != 1 || starts != 1 || results != 1 || compactions != 1 || usage != 1 {
			t.Fatalf("incomplete/duplicated turn: %+v", events)
		}
	}
	c.Close()
	<-c.done
	if strings.Contains(record.String(), "config-secret") || strings.Contains(record.String(), "fixture-token") {
		t.Fatal("credentials appeared in recording")
	}
	if !strings.Contains(record.String(), "item/agentMessage/delta") {
		t.Fatal("conversation was not recorded")
	}
}

func TestCodexRefusesUnsafeInitialization(t *testing.T) {
	for _, tc := range []struct{ mode, reason string }{
		{"missing-environments", "editor-only"}, {"foreign", "another MCP server"},
		{"empty-catalog", "scene_tree"}, {"missing-read", "script_read"}, {"missing-validate", "project_validate"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			c := startCodexFixture(t, tc.mode, nil)
			if err := c.Send("hello"); err != nil {
				t.Fatal(err)
			}
			events := codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvError })
			if err := events[len(events)-1].Err; err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("expected refusal mentioning %q, got %v", tc.reason, err)
			}
		})
	}
}

func TestCodexCancelPendingStartAndMissingCompletion(t *testing.T) {
	for _, mode := range []string{"late-start", "start-hangs", "interrupt-hangs", "no-read"} {
		t.Run(mode, func(t *testing.T) {
			c := startCodexFixture(t, mode, nil)
			prompt := "wait"
			if mode == "no-read" {
				<-c.ready
				prompt = strings.Repeat("x", 2<<20)
			}
			if err := c.Send(prompt); err != nil {
				t.Fatal(err)
			}
			if mode == "no-read" {
				deadline := time.Now().Add(time.Second)
				for {
					c.mu.Lock()
					pending := len(c.pending)
					c.mu.Unlock()
					if pending > 0 {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("turn/start was not issued")
					}
					time.Sleep(time.Millisecond)
				}
			} else {
				codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvText || e.Kind == EvUsage && e.Tokens == 1 })
			}
			c.Cancel()
			events := codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvTurnEnd || e.Kind == EvError })
			last := events[len(events)-1]
			if last.Kind == EvTurnEnd && (!last.Failed || last.Text != "interrupted") {
				t.Fatal(last)
			}
		})
	}
}

func TestCodexCancelThenNextTurn(t *testing.T) {
	c := startCodexFixture(t, "standard", nil)
	if err := c.Send("wait"); err != nil {
		t.Fatal(err)
	}
	codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvText })
	if err := c.Send("overlap"); err == nil {
		t.Fatal("overlapping turn accepted")
	}
	c.Cancel()
	codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvTurnEnd })
	if err := c.Send("hello"); err != nil {
		t.Fatal(err)
	}
	codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvTurnEnd })
}

func TestCodexFailureExitAndBackpressure(t *testing.T) {
	for _, mode := range []string{"standard", "exit", "eof-live", "flood"} {
		t.Run(mode, func(t *testing.T) {
			c := startCodexFixture(t, mode, nil)
			prompt := "failure"
			if mode == "flood" {
				prompt = "flood"
			}
			if err := c.Send(prompt); err != nil {
				t.Fatal(err)
			}
			if mode == "flood" {
				codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvText })
				// Leave subsequent events unread: Close must release producer backpressure.
				c.Close()
				return
			}
			events := codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvError })
			if strings.Contains(events[len(events)-1].Err.Error(), "fixture-token") {
				t.Fatal("token leaked in error")
			}
		})
	}
}

func TestCodexNativeQuestionsAndDeniedApproval(t *testing.T) {
	var received = make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("MCP bearer token missing")
		}
		var body struct {
			Params struct {
				Name      string
				Arguments map[string]any
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Params.Name != "ask_user" {
			t.Error("wrong MCP tool")
		}
		received <- body.Params.Arguments
		io.WriteString(w, `{"result":{"content":[{"type":"text","text":"The user chose: Blue"}]}}`)
	}))
	defer server.Close()
	c := startCodexFixture(t, "standard", func(cfg *Config) { cfg.MCPURL = server.URL })
	if err := c.Send("question"); err != nil {
		t.Fatal(err)
	}
	events := codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvTurnEnd })
	if args := <-received; args["question"] != "Which color?" || args["other"] != true {
		t.Fatal(args)
	}
	found := false
	for _, e := range events {
		if e.Kind == EvText && strings.Contains(e.Text, `"color":{"answers":["Blue"]}`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("native answer did not return to Codex: %+v", events)
	}
	if err := c.Send("approval"); err != nil {
		t.Fatal(err)
	}
	codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvTurnEnd })
}

func TestCodexQuestionHTTPIsCancelled(t *testing.T) {
	arrived, cancelled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(arrived)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()
	c := startCodexFixture(t, "standard", func(cfg *Config) { cfg.MCPURL = server.URL })
	if err := c.Send("question"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-arrived:
	case <-time.After(time.Second):
		t.Fatal("question not forwarded")
	}
	c.Cancel()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("question HTTP was not cancelled")
	}
	codexEventUntil(t, c, func(e Event) bool { return e.Kind == EvTurnEnd })
}

func TestCodexAvailabilityAndCancelledClient(t *testing.T) {
	t.Setenv("AEGO_CODEX_BACKEND_FIXTURE", "standard")
	exe, _ := os.Executable()
	if v, err := (Codex{}).Available(Config{Command: exe}); err != nil || v != "codex-cli 0.166.0" {
		t.Fatal(v, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewClient("http://127.0.0.1:1", "token").CallRawContext(ctx, "ask_user", nil); err == nil {
		t.Fatal("cancelled client made a request")
	}
}
