package backend

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Codex uses the app-server protocol, retaining one thread across turns. The
// editor's MCP bridge owns project access, proposals, questions and Undo.
type Codex struct{}

func (Codex) Name() string { return "codex" }

func codexBinary(cfg Config) string {
	if cfg.Command != "" {
		return cfg.Command
	}
	return "codex"
}

func (Codex) Available(cfg Config) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, codexBinary(cfg), "--version").Output()
	if err != nil {
		return "", fmt.Errorf("Codex CLI is unavailable: %w", err)
	}
	v := strings.TrimSpace(string(out))
	var major, minor, patch int
	fields := strings.Fields(v)
	if len(fields) != 2 {
		return "", fmt.Errorf("unrecognized Codex version: %s", v)
	}
	if _, err = fmt.Sscanf(fields[1], "%d.%d.%d", &major, &minor, &patch); err != nil || major == 0 && minor < 166 {
		return "", fmt.Errorf("Codex CLI 0.166.0 or newer is required; found %s", v)
	}
	return v, nil
}

const codexTokenEnv = "AEGO_CODEX_MCP_TOKEN"

// These are session overrides, never writes to the user's Codex configuration.
// environments:[] below also removes shell/apply_patch environment access. MCP
// maps deep-merge, so inherited servers must be disabled individually at start.
func codexArgs() []string {
	args := []string{"app-server", "--listen", "stdio://"}
	for _, s := range []string{
		"features.shell_tool=false", "features.apps=false", "features.plugins=false",
		"features.hooks=false", "features.multi_agent=false", "features.multi_agent_v2=false",
		"features.browser_use=false", "features.computer_use=false", "features.view_image=false",
		"features.image_generation=false", "features.code_mode=false", "features.code_mode_host=false",
		"features.skill_mcp_dependency_install=false", "features.remote_control=false",
		"web_search=\"disabled\"", "notify=[]",
	} {
		args = append(args, "-c", s)
	}
	return args
}

func (Codex) Start(cfg Config) (Conversation, error) {
	if cfg.MCPURL == "" || cfg.Token == "" {
		return nil, errors.New("Codex needs the editor's authenticated MCP bridge")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &codexConv{cfg: cfg, ctx: ctx, cancel: cancel, events: make(chan Event, 128),
		ready: make(chan struct{}), exited: make(chan struct{}), done: make(chan struct{}),
		pending: map[string]chan codexReply{}, streamed: map[string]bool{}}
	args := codexArgs()
	catalog, prepErr := editorToolCatalog(ctx, cfg)
	if prepErr != nil {
		cancel()
		return nil, prepErr
	}
	started := false
	if catalog != "" {
		defer func() {
			if !started {
				_ = os.Remove(catalog)
			}
		}()
		args = append(args, "-c", "model_catalog_json="+strconv.Quote(catalog))
	}
	c.cmd = exec.Command(codexBinary(cfg), args...)
	c.cmd.Dir = cfg.Dir
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, codexTokenEnv+"=") {
			c.cmd.Env = append(c.cmd.Env, v)
		}
	}
	c.cmd.Env = append(c.cmd.Env, codexTokenEnv+"="+cfg.Token)
	c.cmd.Stderr = &c.stderr
	var err error
	if c.in, err = c.cmd.StdinPipe(); err != nil {
		cancel()
		return nil, err
	}
	out, err := c.cmd.StdoutPipe()
	if err != nil {
		c.in.Close()
		cancel()
		return nil, err
	}
	if err = c.cmd.Start(); err != nil {
		c.in.Close()
		cancel()
		return nil, err
	}
	started = true
	c.work.Add(2)
	go c.read(out)
	go func() {
		defer c.work.Done()
		err := c.initialize()
		c.mu.Lock()
		c.readyErr = err
		c.mu.Unlock()
		if err != nil && c.ctx.Err() == nil {
			c.mu.Lock()
			if c.conn.State == "" {
				c.conn.State = "disconnected"
			}
			c.mu.Unlock()
			c.emit(Event{Kind: EvError, Err: err})
			c.Close()
		}
		close(c.ready)
	}()
	go func() {
		c.work.Wait()
		if catalog != "" {
			_ = os.Remove(catalog)
		}
		close(c.events)
		close(c.done)
	}()
	return c, nil
}

type codexReply struct {
	result json.RawMessage
	err    error
}

type codexEnvelope struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type codexConv struct {
	conn     Connection
	checking bool
	cfg      Config
	cmd      *exec.Cmd
	in       io.WriteCloser
	ctx      context.Context
	cancel   context.CancelFunc
	stderr   codexStderr

	events chan Event
	ready  chan struct{}
	exited chan struct{}
	done   chan struct{}
	work   sync.WaitGroup
	stop   sync.Once
	write  sync.Mutex
	mu     sync.Mutex

	pending  map[string]chan codexReply
	next     uint64
	closed   bool
	readyErr error
	server   string
	thread   string
	model    string
	turn     string
	lastTurn string
	busy     bool
	gen      uint64
	wanted   bool
	sentStop bool
	turnCtx  context.Context
	turnStop context.CancelFunc
	started  time.Time
	lastErr  string
	streamed map[string]bool // read-loop only
}

// The CLI can emit diagnostics indefinitely; retain only the final 16 KiB.
type codexStderr struct {
	mu sync.Mutex
	b  []byte
}

func (b *codexStderr) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	const limit = 16 << 10
	if len(p) >= limit {
		b.b = append(b.b[:0], p[len(p)-limit:]...)
	} else {
		if len(b.b)+len(p) > limit {
			b.b = append(b.b[:0], b.b[len(b.b)+len(p)-limit:]...)
		}
		b.b = append(b.b, p...)
	}
	return n, nil
}

func (b *codexStderr) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.b)
}

func (c *codexConv) safe(s string) string { return strings.ReplaceAll(s, c.cfg.Token, "[redacted]") }

func (c *codexConv) emit(e Event) {
	select {
	case c.events <- e:
	case <-c.ctx.Done():
	}
}

func (c *codexConv) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.write.Lock()
	defer c.write.Unlock()
	_, err = c.in.Write(append(b, '\n'))
	return err
}

func (c *codexConv) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c.mu.Lock()
	c.next++
	id := c.next
	key := strconv.FormatUint(id, 10)
	ch := make(chan codexReply, 1)
	c.pending[key] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, key); c.mu.Unlock() }()
	if err := c.writeJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, fmt.Errorf("Codex %s: %w", method, err)
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("Codex %s: %w", method, r.err)
		}
		return r.result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("Codex %s: %w", method, ctx.Err())
	}
}

func (c *codexConv) initialize() error {
	_, err := c.request(c.ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "aego_editor", "title": "Aego Editor", "version": "1"},
		"capabilities": map[string]bool{"experimentalApi": true},
	})
	if err != nil {
		return err
	}
	if err := c.writeJSON(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return err
	}
	raw, err := c.request(c.ctx, "config/read", map[string]any{"includeLayers": false, "cwd": c.cfg.Dir})
	if err != nil {
		return err
	}
	// Only names are retained. The full response can contain credentials and is
	// deliberately excluded from Record and all diagnostics.
	var config struct {
		Config struct {
			Servers map[string]json.RawMessage `json:"mcp_servers"`
		} `json:"config"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return errors.New("Codex returned an invalid configuration response")
	}
	servers := map[string]any{}
	for name := range config.Config.Servers {
		servers[name] = map[string]bool{"enabled": false}
	}
	for {
		var nonce [6]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return err
		}
		server := "aego_editor_" + hex.EncodeToString(nonce[:])
		if _, found := servers[server]; !found {
			c.mu.Lock()
			c.server = server
			c.mu.Unlock()
			break
		}
	}
	servers[c.server] = map[string]any{"url": c.cfg.MCPURL, "bearer_token_env_var": codexTokenEnv,
		"enabled": true, "required": true, "startup_timeout_sec": 15, "tool_timeout_sec": 660,
		// The bridge itself presents mutation approvals and applies History. Codex
		// may call this one trusted server; all other tool paths stay disabled.
		"default_tools_approval_mode": "approve"}
	params := map[string]any{"cwd": c.cfg.Dir, "sandbox": "read-only", "approvalPolicy": "never",
		"environments": []any{}, "ephemeral": true, "allowProviderModelFallback": false,
		"developerInstructions": c.cfg.System + "\nThe editor MCP namespace for THIS thread is mcp__" + c.server + ". For project tasks use its scene_tree, script_read and project_validate tools. Discover the editor tools if they are deferred. Do not claim the editor is offline without an actual tool error; if a call fails report the tool and error without secrets. Never bypass editor approvals with disk or shell.", "config": map[string]any{"mcp_servers": servers}}
	if c.cfg.Model != "" {
		params["model"] = c.cfg.Model
	}
	raw, err = c.request(c.ctx, "thread/start", params)
	if err != nil {
		return err
	}
	var started struct {
		Thread struct {
			ID           string          `json:"id"`
			Environments json.RawMessage `json:"environments"`
		} `json:"thread"`
		Model   string `json:"model"`
		Sandbox struct {
			Type string `json:"type"`
		} `json:"sandbox"`
	}
	if json.Unmarshal(raw, &started) != nil || started.Thread.ID == "" {
		return errors.New("Codex did not create a thread")
	}
	var environments []json.RawMessage
	if len(started.Thread.Environments) == 0 || string(started.Thread.Environments) == "null" ||
		json.Unmarshal(started.Thread.Environments, &environments) != nil || len(environments) != 0 || started.Sandbox.Type != "readOnly" {
		return errors.New("Codex did not acknowledge editor-only access; update Codex CLI to 0.166.0 or newer")
	}
	c.mu.Lock()
	c.thread, c.model = started.Thread.ID, started.Model
	c.mu.Unlock()
	if err := c.checkServers(); err != nil {
		return err
	}
	if c.cfg.Binding != "" {
		c.checkVision()
	}
	c.emit(Event{Kind: EvUsage, Model: started.Model})
	return nil
}

func (c *codexConv) checkServers() error {
	deadline := time.Now().Add(15 * time.Second)
	cursor := ""
	found := false
	for page := 0; page < 128; page++ {
		params := map[string]any{"threadId": c.thread, "detail": "toolsAndAuthOnly", "limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.request(c.ctx, "mcpServerStatus/list", params)
		if err != nil {
			return err
		}
		var result struct {
			Data []struct {
				Name       string                     `json:"name"`
				Status     string                     `json:"runtimeStatus"`
				Tools      map[string]json.RawMessage `json:"tools"`
				ToolsError *string                    `json:"toolsError"`
			} `json:"data"`
			Next string `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &result) != nil {
			return errors.New("Codex returned invalid MCP server status")
		}
		pending := false
		for _, s := range result.Data {
			if s.Name == c.server {
				if (s.Status == "starting" || s.Status == "notStarted") && time.Now().Before(deadline) {
					pending = true
					continue
				}
				if s.Status != "connected" {
					c.setState("disconnected", 0)
					return errors.New("Codex could not connect to the editor's MCP bridge")
				}
				if s.ToolsError != nil {
					c.setState("missing tools", len(s.Tools))
					return errors.New("Codex could not list the editor's MCP tools for this thread")
				}
				required := []string{"scene_tree", "script_read", "project_validate"}
				if c.cfg.Binding != "" {
					required = append(required, "editor_scene_context")
				}
				if err := verifyEditorCatalog(s.Tools, required); err != nil {
					c.setState("missing tools", len(s.Tools))
					return err
				}
				c.setState("tools listed", len(s.Tools))
				found = true
			} else if s.Status != "disabled" {
				return fmt.Errorf("Codex left another MCP server enabled (%s); refusing to bypass editor history", s.Name)
			}
		}
		if pending {
			c.setState("starting", 0)
			select {
			case <-c.ctx.Done():
				return c.ctx.Err()
			case <-time.After(150 * time.Millisecond):
			}
			page--
			cursor = ""
			found = false
			continue
		}
		if result.Next == "" {
			if !found {
				c.setState("disconnected", 0)
				return errors.New("Codex did not load the editor's MCP bridge")
			}
			if c.cfg.Binding != "" {
				_, err := c.editorRead(c.ctx, false)
				c.mu.Lock()
				if err != nil {
					c.conn.State = "call rejected"
					c.conn.Last = "editor_scene_context: rejected"
				} else {
					c.conn.State = "verified"
					c.conn.Last = "editor_scene_context: OK"
				}
				c.mu.Unlock()
				return err
			}
			return nil
		}
		if result.Next == cursor {
			break
		}
		cursor = result.Next
	}
	return errors.New("Codex MCP server pagination did not terminate")
}

// The inventory must belong to this thread; a connected server can expose no tools.
func verifyEditorCatalog(tools map[string]json.RawMessage, required []string) error {
	for _, want := range required {
		seen := false
		for key, raw := range tools {
			var tool *struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &tool) == nil && tool != nil && (tool.Name == want || (tool.Name == "" && key == want)) {
				seen = true
				break
			}
		}
		if !seen {
			return fmt.Errorf("the Aego MCP is connected but %s is missing from this thread's catalog (%d tools). Reconnect or start a new Agent session; Auto-apply does not grant tool access", want, len(tools))
		}
	}
	return nil
}

func (c *codexConv) Send(text string) error { return c.send(text, "", 0, nil) }
func (c *codexConv) Explain(text, document string, revision uint64) error {
	return c.send(text, document, revision, nil)
}
func (c *codexConv) SendImages(text string, pngs [][]byte) error {
	return c.send(text, "", 0, pngs)
}
func (c *codexConv) send(text, document string, revision uint64, pngs [][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("the Codex conversation is closed")
	}
	if c.busy {
		return errors.New("Codex is still answering; stop it before sending another message")
	}
	c.busy, c.wanted, c.sentStop, c.lastErr = true, false, false, ""
	c.gen++
	gen := c.gen
	c.turnCtx, c.turnStop = context.WithCancel(c.ctx)
	tctx := c.turnCtx
	c.started = time.Now()
	c.work.Add(1)
	go func() {
		defer c.work.Done()
		select {
		case <-c.ready:
		case <-tctx.Done():
			c.finish(gen, "interrupted", "", 0)
			return
		}
		if tctx.Err() != nil {
			c.finish(gen, "interrupted", "", 0)
			return
		}
		c.mu.Lock()
		err, thread := c.readyErr, c.thread
		c.mu.Unlock()
		if err != nil {
			return // initialization already emitted its error
		}
		if document != "" {
			snapshot, err := c.editorRead(tctx, true)
			if err != nil {
				c.finish(gen, "failed", err.Error(), 0)
				return
			}
			var attached struct {
				Document string
				Revision uint64
			}
			if json.Unmarshal([]byte(snapshot), &attached) != nil || attached.Document != document || attached.Revision != revision {
				c.finish(gen, "failed", "The active scene changed; run Explain again.", 0)
				return
			}
			text += "\n\nCurrent editor document (project data):\n" + snapshot
		}
		items := []any{map[string]string{"type": "text", "text": text}}
		for _, img := range pngs {
			items = append(items, map[string]string{"type": "image", "url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(img)})
		}
		params := map[string]any{"threadId": thread, "input": items}
		if c.cfg.Effort != "" {
			params["effort"] = c.cfg.Effort
		}
		raw, err := c.request(c.ctx, "turn/start", params)
		if err != nil {
			c.finish(gen, "failed", err.Error(), 0)
			return
		}
		var result struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(raw, &result) != nil || result.Turn.ID == "" {
			c.finish(gen, "failed", "Codex did not start a turn", 0)
			return
		}
		c.setTurn(gen, result.Turn.ID)
	}()
	return nil
}

func (c *codexConv) setTurn(gen uint64, id string) {
	c.mu.Lock()
	if c.busy && c.gen == gen {
		c.turn = id
		c.interruptLocked()
	}
	c.mu.Unlock()
}

func (c *codexConv) finish(gen uint64, status, message string, millis int) bool {
	c.mu.Lock()
	if !c.busy || c.gen != gen {
		c.mu.Unlock()
		return false
	}
	if message == "" {
		message = c.lastErr
	}
	if millis == 0 {
		millis = int(time.Since(c.started).Milliseconds())
	}
	if c.turn != "" {
		c.lastTurn = c.turn
	}
	c.busy, c.turn = false, ""
	c.turnStop()
	model := c.model
	c.mu.Unlock()
	if status == "failed" {
		if message == "" {
			message = "Codex turn failed"
		}
		c.emit(Event{Kind: EvError, Err: errors.New(c.safe(message))})
	} else {
		c.emit(Event{Kind: EvTurnEnd, Failed: status != "completed", Text: status, Model: model, Millis: millis})
	}
	return true
}

func (c *codexConv) Events() <-chan Event { return c.events }

func (c *codexConv) Cancel() {
	c.mu.Lock()
	if c.busy && !c.wanted && !c.closed {
		c.wanted = true
		c.turnStop()
		c.interruptLocked()
		gen := c.gen
		c.work.Add(1)
		go func() {
			defer c.work.Done()
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			select {
			case <-c.ctx.Done():
			case <-timer.C:
				// Some servers acknowledge interrupt without completing the turn;
				// others never return its ID. Neither may leave the panel stuck.
				if c.finish(gen, "interrupted", "", 0) {
					c.Close()
				}
			}
		}()
	}
	c.mu.Unlock()
}

// Called with mu held, so an interrupt before turn/start's reply is not lost.
func (c *codexConv) interruptLocked() {
	if !c.wanted || c.sentStop || c.turn == "" || c.closed {
		return
	}
	c.sentStop = true
	thread, turn, gen := c.thread, c.turn, c.gen
	c.work.Add(1)
	go func() {
		defer c.work.Done()
		ctx, cancel := context.WithTimeout(c.ctx, 3*time.Second)
		defer cancel()
		if _, err := c.request(ctx, "turn/interrupt", map[string]string{"threadId": thread, "turnId": turn}); err != nil {
			c.mu.Lock()
			active := c.busy && c.gen == gen
			c.mu.Unlock()
			if active && c.ctx.Err() == nil {
				c.emit(Event{Kind: EvError, Err: err})
				c.Close()
			}
		}
	}()
}

func (c *codexConv) Close() error {
	c.stop.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		c.cancel()
		c.in.Close()
		go func() {
			select {
			case <-c.exited:
			case <-time.After(500 * time.Millisecond):
				c.cmd.Process.Kill()
			}
		}()
	})
	return nil
}

func (c *codexConv) read(r io.Reader) {
	defer c.work.Done()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	for sc.Scan() {
		var m codexEnvelope
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			c.emit(Event{Kind: EvError, Err: errors.New("Codex returned malformed app-server JSON")})
			c.Close()
			break
		}
		if m.Method == "" {
			c.mu.Lock()
			ch := c.pending[string(m.ID)]
			delete(c.pending, string(m.ID))
			c.mu.Unlock()
			if ch != nil {
				r := codexReply{result: m.Result}
				if m.Error != nil {
					r.err = errors.New(c.safe(m.Error.Message))
				}
				select {
				case ch <- r:
				case <-c.ctx.Done():
				}
			}
			continue
		}
		if len(m.ID) > 0 && string(m.ID) != "null" {
			c.serverRequest(m)
			continue
		}
		// Never record config/account responses or unrelated notifications. Config
		// can contain secrets; conversation events contain the user's transcript.
		if c.notification(m) && c.cfg.Record != nil {
			_, _ = io.WriteString(c.cfg.Record, c.safe(string(sc.Bytes()))+"\n")
		}
	}
	if c.ctx.Err() == nil {
		// Once stdout is gone the protocol cannot recover. Reap even a child
		// that closes stdout while continuing to run.
		c.cmd.Process.Kill()
	}
	err := c.cmd.Wait()
	close(c.exited)
	if c.ctx.Err() == nil {
		message := "Codex app-server ended"
		if sc.Err() != nil {
			message += ": " + sc.Err().Error()
		} else if err != nil {
			message += ": " + err.Error()
		}
		if s := strings.TrimSpace(c.stderr.String()); s != "" {
			message += "\n" + s
		}
		c.emit(Event{Kind: EvError, Err: errors.New(c.safe(message))})
	}
	c.Close()
}
