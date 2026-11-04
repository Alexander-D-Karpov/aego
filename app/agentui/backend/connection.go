package backend

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Connection struct {
	State  string
	Thread string
	Model  string
	Vision string
	Tools  int
	Last   string
}

type Diagnostics interface {
	Connection() Connection
	Check()
}

func (c *codexConv) Connection() Connection {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.conn
	d.Thread, d.Model = c.thread, c.model
	if d.State == "" {
		d.State = "starting"
	}
	return d
}

func (c *codexConv) Check() {
	c.mu.Lock()
	if c.checking || c.closed {
		c.mu.Unlock()
		return
	}
	c.checking = true
	c.mu.Unlock()
	go func() {
		defer func() { c.mu.Lock(); c.checking = false; c.mu.Unlock() }()
		select {
		case <-c.ready:
		case <-c.ctx.Done():
			return
		}
		if err := c.checkServers(); err != nil {
			return
		}
		c.checkVision()
	}()
}

func (c *codexConv) setState(state string, tools int) {
	c.mu.Lock()
	c.conn.State, c.conn.Tools = state, tools
	c.mu.Unlock()
}

func (c *codexConv) checkVision() {
	ctx, cancel := context.WithTimeout(c.ctx, 3*time.Second)
	defer cancel()
	raw, err := c.request(ctx, "model/list", map[string]any{"limit": 100, "includeHidden": true})
	if err != nil {
		return
	}
	var r struct {
		Data []struct {
			ID, Model       string
			InputModalities []string
		}
	}
	if json.Unmarshal(raw, &r) != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range r.Data {
		if (m.ID == c.model || m.Model == c.model) && m.InputModalities != nil {
			c.conn.Vision = "unsupported"
			for _, mode := range m.InputModalities {
				if mode == "image" {
					c.conn.Vision = "supported"
				}
			}
			break
		}
	}
}

func (c *codexConv) editorRead(ctx context.Context, details bool) (string, error) {
	raw, err := c.request(ctx, "mcpServer/tool/call", map[string]any{"threadId": c.thread, "server": c.server, "tool": "editor_scene_context", "arguments": map[string]any{"details": details}})
	if err != nil {
		return "", err
	}
	var res struct {
		IsError bool
		Content []struct{ Type, Text string }
	}
	if json.Unmarshal(raw, &res) != nil || res.IsError {
		return "", errors.New("editor context call rejected")
	}
	for _, v := range res.Content {
		if v.Type != "text" {
			continue
		}
		var s struct {
			Binding  string
			Document string
		}
		if json.Unmarshal([]byte(v.Text), &s) != nil || s.Binding != c.cfg.Binding {
			return "", errors.New("the thread reached a different editor session")
		}
		if details && s.Document == "" {
			return "", errors.New("no scene is open")
		}
		return v.Text, nil
	}
	return "", errors.New("editor context call returned no document response")
}

func (c *claudeConv) Connection() Connection {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

func (c *claudeConv) Check() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	id := "aego-status"
	msg := map[string]any{"type": "control_request", "request_id": id, "request": map[string]string{"subtype": "mcp_status"}}
	data, _ := json.Marshal(msg)
	if _, err := c.in.Write(append(data, '\n')); err != nil {
		c.conn.State = "disconnected"
	}
}

func (c *claudeConv) status(line []byte) {
	var msg struct {
		Type, Subtype, Model string
		Session              string `json:"session_id"`
		Tools                []string
		Servers              []struct{ Name, Status string } `json:"mcp_servers"`
		Response             struct {
			Subtype   string
			RequestID string `json:"request_id"`
			Response  struct {
				Servers []struct {
					Name, Status string
					Tools        []struct{ Name string }
				} `json:"mcpServers"`
			}
		}
	}
	if json.Unmarshal(line, &msg) != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case msg.Type == "system" && msg.Subtype == "init":
		c.conn.Thread, c.conn.Model = msg.Session, msg.Model
		c.conn.State = "disconnected"
		for _, s := range msg.Servers {
			if s.Name == "aego" && s.Status == "connected" {
				c.conn.State = "missing tools"
			}
		}
		c.conn.Tools = 0
		required := map[string]bool{"editor_scene_context": false, "scene_tree": false, "script_read": false}
		for _, t := range msg.Tools {
			if strings.HasPrefix(t, ToolPrefix) {
				c.conn.Tools++
				required[strings.TrimPrefix(t, ToolPrefix)] = true
			}
		}
		if c.conn.State == "missing tools" && required["editor_scene_context"] && required["scene_tree"] && required["script_read"] {
			c.conn.State = "tools listed"
		}
	case msg.Type == "control_response" && msg.Response.RequestID == "aego-status":
		if msg.Response.Subtype != "success" {
			c.conn.State = "status unavailable"
			return
		}
		c.conn.State = "disconnected"
		for _, s := range msg.Response.Response.Servers {
			if s.Name != "aego" || s.Status != "connected" {
				continue
			}
			c.conn.State = "missing tools"
			if s.Tools == nil {
				c.conn.State = "connected; inventory unverified"
				return
			}
			c.conn.Tools = len(s.Tools)
			var context, tree, script bool
			for _, t := range s.Tools {
				switch strings.TrimPrefix(t.Name, ToolPrefix) {
				case "editor_scene_context":
					context = true
				case "scene_tree":
					tree = true
				case "script_read":
					script = true
				}
			}
			if context && tree && script {
				c.conn.State = "tools listed"
			}
		}
	}
}
