package mcp_test

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aego/app/agent"
	"aego/app/agent/mcp"
	"aego/app/docstool"
	"aego/app/run"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/engine/docs"
	"aego/project"
	"aego/sdk"
	"aego/tests/fixture"
)

type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Params json.RawMessage `json:"params"`
}

type client struct {
	t     *testing.T
	in    *io.PipeWriter
	lines chan string
	done  chan error
	mu    sync.Mutex
	notes []message
	seen  []string
	next  int
	dir   string
	p     *project.Project
}

func start(t *testing.T, watch bool) *client {
	t.Helper()
	dir := fixture.Copy(t, "platformer")
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	mods := []sdk.Module{fixture.Platformer{}}
	kit, err := agent.NewKit(p, agent.Options{Mods: mods, Run: run.AgentRunner(mods...)})
	if err != nil {
		t.Fatal(err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{t: t, in: inW, lines: make(chan string, 256), done: make(chan error, 1), dir: dir, p: p}
	go func() {
		err := mcp.Serve(inR, outW, mcp.Options{
			Kit:      kit,
			Docs:     func() (*docs.Registry, error) { return docstool.Registry(p, sdk.AdaptAll(mods)) },
			Watch:    watch,
			Interval: 50 * time.Millisecond,
			Log:      io.Discard,
			Version:  "test",
		})
		outW.Close()
		c.done <- err
	}()
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			c.lines <- sc.Text()
		}
		close(c.lines)
	}()
	t.Cleanup(func() { inW.Close() })
	return c
}

func (c *client) send(v any) {
	c.t.Helper()
	b, _ := json.Marshal(v)
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) rawLine(text string) {
	c.t.Helper()
	if _, err := c.in.Write([]byte(text + "\n")); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) read(timeout time.Duration) (message, bool) {
	c.t.Helper()
	select {
	case line, ok := <-c.lines:
		if !ok {
			return message{}, false
		}
		c.seen = append(c.seen, line)
		var m message
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			c.t.Fatalf("not JSON on stdout: %q", line)
		}
		return m, true
	case <-time.After(timeout):
		return message{}, false
	}
}

func (c *client) call(method string, params any) message {
	c.t.Helper()
	c.next++
	id := c.next
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	for {
		m, ok := c.read(120 * time.Second)
		if !ok {
			c.t.Fatalf("no reply to %s", method)
		}
		if len(m.ID) == 0 || string(m.ID) == "null" && m.Method != "" {
			c.mu.Lock()
			c.notes = append(c.notes, m)
			c.mu.Unlock()
			continue
		}
		var got int
		if json.Unmarshal(m.ID, &got) == nil && got == id {
			return m
		}
		if string(m.ID) == "null" {
			return m
		}
	}
}

func (c *client) notify(method string) {
	c.send(map[string]any{"jsonrpc": "2.0", "method": method})
}

func (c *client) init() {
	c.t.Helper()
	c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test"}})
	c.notify("notifications/initialized")
}

type toolResult struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	IsError    bool            `json:"isError"`
	Structured json.RawMessage `json:"structuredContent"`
}

func (c *client) tool(name string, args map[string]any) toolResult {
	c.t.Helper()
	m := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if m.Error != nil {
		c.t.Fatalf("%s: rpc error %d %s", name, m.Error.Code, m.Error.Message)
	}
	var r toolResult
	if err := json.Unmarshal(m.Result, &r); err != nil {
		c.t.Fatal(err)
	}
	return r
}

func (c *client) ok(name string, args map[string]any) agent.Output {
	c.t.Helper()
	r := c.tool(name, args)
	if r.IsError {
		c.t.Fatalf("%s %v: %s", name, args, r.Content[0].Text)
	}
	var out agent.Output
	if err := json.Unmarshal(r.Structured, &out); err != nil {
		c.t.Fatalf("%s: %v", name, err)
	}
	return out
}

func TestProtocol(t *testing.T) {
	c := start(t, false)
	m := c.call("initialize", map[string]any{"protocolVersion": "2025-03-26"})
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Resources struct {
				Subscribe bool `json:"subscribe"`
			} `json:"resources"`
		} `json:"capabilities"`
	}
	json.Unmarshal(m.Result, &init)
	if init.ProtocolVersion != "2025-03-26" || !init.Capabilities.Resources.Subscribe {
		t.Fatalf("initialize %s", m.Result)
	}
	m = c.call("initialize", map[string]any{"protocolVersion": "1999-01-01"})
	json.Unmarshal(m.Result, &init)
	if init.ProtocolVersion != "2025-06-18" {
		t.Fatalf("unknown version answered %s", init.ProtocolVersion)
	}
	if m := c.call("ping", nil); string(m.Result) != "{}" {
		t.Fatalf("ping %s", m.Result)
	}
	if m := c.call("no/such", nil); m.Error == nil || m.Error.Code != -32601 {
		t.Fatalf("unknown method %+v", m)
	}
	c.rawLine(`[{"jsonrpc":"2.0","id":99,"method":"ping"}]`)
	if m, _ := c.read(5 * time.Second); m.Error == nil || m.Error.Code != -32600 {
		t.Fatalf("batch %+v", m)
	}
	m = c.call("tools/list", nil)
	var list struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	json.Unmarshal(m.Result, &list)
	names := map[string]bool{}
	for _, tl := range list.Tools {
		if names[tl.Name] || tl.InputSchema["type"] != "object" {
			t.Fatalf("tool %s", tl.Name)
		}
		names[tl.Name] = true
	}
	for _, cmd := range agent.Commands() {
		if !names[cmd.Tool()] {
			t.Fatalf("no tool %s", cmd.Tool())
		}
	}
	if !names["docs_search"] || !names["docs_read"] || len(names) != len(agent.Commands())+2 {
		t.Fatalf("%d tools", len(names))
	}
	c.rawLine(`{not json`)
	if m, _ := c.read(5 * time.Second); m.Error == nil || m.Error.Code != -32700 {
		t.Fatalf("parse error %+v", m)
	}
	if err := <-c.done; err == nil {
		t.Fatal("Serve did not fail on broken JSON")
	}
}

func typeNamed(t *testing.T, c *client, suffix string) string {
	t.Helper()
	out := c.ok("schema_list", map[string]any{"kind": "component"})
	var list []agent.TypeView
	b, _ := json.Marshal(out.Value)
	json.Unmarshal(b, &list)
	for _, tv := range list {
		if strings.HasSuffix(tv.Name, suffix) {
			return tv.Name
		}
	}
	t.Fatalf("no component %s", suffix)
	return ""
}

func runReport(t *testing.T, c *client, file string) agent.RunReport {
	t.Helper()
	out := c.ok("scene_run", map[string]any{"file": file, "frames": 120})
	var rep agent.RunReport
	b, _ := json.Marshal(out.Value)
	json.Unmarshal(b, &rep)
	return rep
}

func TestLevelFromScratch(t *testing.T) {
	c := start(t, false)
	c.init()
	const level = "scenes/agent.aescene"
	camera, box, body := typeNamed(t, c, ".Camera2D"), typeNamed(t, c, ".BoxCollider2D"), typeNamed(t, c, ".Body2D")
	c.ok("scene_new", map[string]any{"file": level})
	c.ok("scene_add_node", map[string]any{"file": level, "name": "Camera"})
	c.ok("scene_add_comp", map[string]any{"file": level, "node": "Camera", "comp": camera})
	c.ok("scene_add_node", map[string]any{"file": level, "name": "Ground"})
	c.ok("scene_add_comp", map[string]any{"file": level, "node": "Ground", "comp": body})
	c.ok("scene_set", map[string]any{"file": level, "node": "Ground", "path": body[strings.LastIndexByte(body, '.')+1:] + ".type", "value": "static"})
	c.ok("scene_add_comp", map[string]any{"file": level, "node": "Ground", "comp": box})
	c.ok("scene_set", map[string]any{"file": level, "node": "Ground", "path": box + ".size", "value": "[10, 1]"})
	c.ok("prefab_instantiate", map[string]any{"file": level, "prefab": "prefabs/enemy.aeprefab"})
	v := c.ok("scene_validate", map[string]any{"file": level})
	for _, p := range v.Problems {
		if p.Error {
			t.Fatalf("validate: %+v", v.Problems)
		}
	}
	a, b := runReport(t, c, level), runReport(t, c, level)
	if a.Error != "" || a.Frames != 120 || a.Hash == "" || a.Hash != b.Hash {
		t.Fatalf("runs %+v / %+v", a, b)
	}
	if r := c.tool("scene_set", map[string]any{"file": level, "node": "Ground", "path": box + ".size", "value": "abc"}); !r.IsError || r.Content[0].Text == "" {
		t.Fatalf("a bad value did not fail: %+v", r)
	}
	if m := c.call("tools/call", map[string]any{"name": "no_such_tool"}); m.Error == nil || m.Error.Code != -32602 {
		t.Fatalf("unknown tool %+v", m)
	}
	m := c.call("resources/list", nil)
	if !strings.Contains(string(m.Result), "aego://file/"+level) {
		t.Fatalf("resources %s", m.Result)
	}
	read := func(uri string) (string, *message) {
		m := c.call("resources/read", map[string]any{"uri": uri})
		if m.Error != nil {
			return "", &m
		}
		var r struct {
			Contents []struct {
				Text string `json:"text"`
			} `json:"contents"`
		}
		json.Unmarshal(m.Result, &r)
		return r.Contents[0].Text, nil
	}
	if text, _ := read("aego://file/" + level); !strings.Contains(text, `name="Ground"`) {
		t.Fatalf("scene text %q", text)
	}
	for _, uri := range []string{"aego://schema/" + box, "aego://project/problems"} {
		text, e := read(uri)
		if e != nil || !json.Valid([]byte(text)) {
			t.Fatalf("%s: %v", uri, e)
		}
	}
	if text, _ := read("aego://docs/index"); len(text) < 100 {
		t.Fatalf("docs index %q", text)
	}
	if _, e := read("aego://nothing/here"); e == nil || e.Error.Code != -32002 {
		t.Fatal("unknown resource did not fail with -32002")
	}
	hits := c.ok("docs_search", map[string]any{"query": "Body2D"})
	var found struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	b2, _ := json.Marshal(hits)
	json.Unmarshal(b2, &found)
	r := c.tool("docs_search", map[string]any{"query": "Body2D"})
	json.Unmarshal([]byte(r.Content[0].Text), &found)
	if len(found.Results) == 0 {
		t.Fatal("docs_search found nothing for Body2D")
	}
	page := c.tool("docs_read", map[string]any{"id": found.Results[0].ID})
	if page.IsError || !strings.Contains(page.Content[0].Text, "#") {
		t.Fatalf("docs_read %+v", page)
	}
	for _, line := range c.seen {
		if !json.Valid([]byte(line)) {
			t.Fatalf("stdout line is not JSON: %q", line)
		}
	}
	pl, err := pipeline.Open(c.p, pipeline.NewImporterRegistry(), log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	if pl.Shared() {
		t.Fatal("the server kept the asset index lock between calls")
	}
}

func TestWatchNotifiesSubscribers(t *testing.T) {
	c := start(t, true)
	c.init()
	const scene, other = "scenes/main.aescene", "prefabs/enemy.aeprefab"
	c.call("resources/subscribe", map[string]any{"uri": "aego://file/" + scene})
	time.Sleep(150 * time.Millisecond)
	touch := func(rel string) {
		abs := filepath.Join(c.dir, rel)
		b, _ := os.ReadFile(abs)
		os.WriteFile(abs, append(b, '\n'), 0o644)
	}
	touch(other)
	touch(scene)
	deadline := time.Now().Add(2 * time.Second)
	var got []string
	for time.Now().Before(deadline) {
		m, ok := c.read(100 * time.Millisecond)
		if !ok {
			continue
		}
		if m.Method == "notifications/resources/updated" {
			var p struct {
				URI string `json:"uri"`
			}
			json.Unmarshal(m.Params, &p)
			got = append(got, p.URI)
			if p.URI == "aego://file/"+scene {
				break
			}
		}
	}
	if len(got) == 0 || got[len(got)-1] != "aego://file/"+scene {
		t.Fatalf("updates %v", got)
	}
	for _, u := range got {
		if u == "aego://file/"+other {
			t.Fatalf("an unsubscribed resource was notified: %v", got)
		}
	}
}
