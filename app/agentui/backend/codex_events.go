package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type codexItem struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Text      string          `json:"text"`
	Server    string          `json:"server"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Only the stdout reader calls notification, so streamed needs no extra lock.
func (c *codexConv) notification(m codexEnvelope) bool {
	var p struct {
		ThreadID string    `json:"threadId"`
		TurnID   string    `json:"turnId"`
		ItemID   string    `json:"itemId"`
		Delta    string    `json:"delta"`
		Item     codexItem `json:"item"`
		Turn     struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Duration int    `json:"durationMs"`
			Error    *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		TokenUsage struct {
			Last struct {
				Input int `json:"inputTokens"`
			} `json:"last"`
			Window int `json:"modelContextWindow"`
		} `json:"tokenUsage"`
	}
	if json.Unmarshal(m.Params, &p) != nil {
		return false
	}
	c.mu.Lock()
	thread, turn, gen, active, model, server, last := c.thread, c.turn, c.gen, c.busy, c.model, c.server, c.lastTurn
	c.mu.Unlock()
	if m.Method == "mcpServer/startupStatus/updated" && thread != "" && p.ThreadID == thread {
		var n struct{ Name, Status string }
		if json.Unmarshal(m.Params, &n) == nil && n.Name == server {
			switch n.Status {
			case "failed", "cancelled":
				c.setState("disconnected", 0)
			case "starting":
				c.setState("starting", 0)
			}
		}
		return false
	}
	if thread == "" || p.ThreadID != thread {
		return false
	}
	if p.Turn.ID != "" {
		p.TurnID = p.Turn.ID
	}
	if !active || p.TurnID != "" && (p.TurnID == last || turn != "" && p.TurnID != turn) {
		return false
	}
	switch m.Method {
	case "turn/started":
		clear(c.streamed)
		c.setTurn(gen, p.Turn.ID)
	case "item/agentMessage/delta":
		if p.Delta != "" {
			c.streamed[p.ItemID] = true
			c.emit(Event{Kind: EvText, Text: p.Delta, Delta: true, MessageID: p.ItemID})
		}
	case "item/started", "item/completed":
		i, complete := p.Item, m.Method == "item/completed"
		switch i.Type {
		case "agentMessage":
			if complete && !c.streamed[i.ID] && i.Text != "" {
				c.emit(Event{Kind: EvText, Text: i.Text, MessageID: i.ID})
			}
		case "mcpToolCall":
			if i.Server != server {
				c.emit(Event{Kind: EvError, Err: errors.New("Codex attempted to use an MCP server outside the editor bridge")})
				c.Close()
				return false
			}
			if !complete {
				c.emit(Event{Kind: EvToolStart, Tool: i.Tool, CallID: i.ID, Args: i.Arguments})
			} else {
				text, failed := codexToolText(i.Result)
				if i.Error != nil {
					text = i.Error.Message
				}
				c.emit(Event{Kind: EvToolResult, Tool: i.Tool, CallID: i.ID, Result: text, Images: toolImages(i.Result),
					Failed: failed || i.Status == "failed" || i.Error != nil})
			}
		case "contextCompaction":
			if complete {
				c.emit(Event{Kind: EvCompact})
			}
		case "commandExecution", "fileChange":
			c.emit(Event{Kind: EvError, Err: errors.New("Codex attempted project access outside the editor bridge")})
			c.Close()
			return false
		default:
			return false
		}
	case "thread/tokenUsage/updated":
		// inputTokens already includes the cached part; do not count it twice.
		c.emit(Event{Kind: EvUsage, Tokens: p.TokenUsage.Last.Input, Window: p.TokenUsage.Window, Model: model})
	case "turn/completed":
		message := ""
		if p.Turn.Error != nil {
			message = p.Turn.Error.Message
		}
		c.finish(gen, p.Turn.Status, message, p.Turn.Duration)
	case "error":
		// Retrying errors are not terminal. turn/completed supplies the outcome.
		c.mu.Lock()
		c.lastErr = p.Error.Message
		c.mu.Unlock()
	default:
		return false
	}
	return true
}

func codexToolText(raw json.RawMessage) (string, bool) {
	var result struct {
		Content    json.RawMessage `json:"content"`
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return "", false
	}
	text := textOf(result.Content)
	if text == "" && len(result.Structured) > 0 && string(result.Structured) != "null" {
		text = string(result.Structured)
	}
	return text, result.IsError
}

// Server requests must receive an answer, including requests this client does
// not support. A question waits off the reader so Stop and replies still flow.
func (c *codexConv) serverRequest(m codexEnvelope) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.work.Add(1)
	ctx, thread, turn, active := c.turnCtx, c.thread, c.turn, c.busy
	c.mu.Unlock()
	go func() {
		defer c.work.Done()
		var result any
		var err error
		switch m.Method {
		case "item/tool/requestUserInput":
			var p codexQuestions
			if json.Unmarshal(m.Params, &p) != nil || !active || ctx == nil ||
				p.ThreadID != thread || turn != "" && p.TurnID != turn {
				err = errors.New("the editor has no matching active turn for this question")
			} else {
				result, err = c.ask(ctx, p)
			}
		case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
			result = map[string]string{"decision": "decline"}
		case "item/permissions/requestApproval":
			result = map[string]any{"permissions": map[string]any{}, "scope": "turn"}
		case "mcpServer/elicitation/request":
			result = map[string]string{"action": "decline"}
		default:
			err = fmt.Errorf("unsupported Codex client request %s", m.Method)
		}
		reply := map[string]any{"id": m.ID, "result": result}
		if err != nil {
			delete(reply, "result")
			reply["error"] = map[string]any{"code": -32603, "message": c.safe(err.Error())}
		}
		if err := c.writeJSON(reply); err != nil && c.ctx.Err() == nil {
			c.emit(Event{Kind: EvError, Err: err})
			c.Close()
		}
	}()
}

type codexQuestions struct {
	ThreadID  string `json:"threadId"`
	TurnID    string `json:"turnId"`
	ItemID    string `json:"itemId"`
	Questions []struct {
		ID       string                                `json:"id"`
		Question string                                `json:"question"`
		Other    bool                                  `json:"isOther"`
		Secret   bool                                  `json:"isSecret"`
		Options  []struct{ Label, Description string } `json:"options"`
	} `json:"questions"`
}

func (c *codexConv) ask(ctx context.Context, p codexQuestions) (any, error) {
	answers := map[string]any{}
	client := NewClient(c.cfg.MCPURL, c.cfg.Token)
	for _, q := range p.Questions {
		if q.ID == "" || q.Secret || len(q.Options) == 0 || len(q.Options) > 6 {
			return nil, errors.New("editor questions need one to six non-secret choices; use ask_user with those options")
		}
		options := make([]any, 0, len(q.Options))
		for _, option := range q.Options {
			options = append(options, map[string]string{"label": option.Label, "detail": option.Description})
		}
		args := map[string]any{"question": q.Question, "options": options, "other": q.Other, "multiple": false}
		encoded, _ := json.Marshal(args)
		id := p.ItemID + "/" + q.ID
		c.emit(Event{Kind: EvToolStart, Tool: "ask_user", CallID: id, Args: encoded})
		raw, err := client.CallRawContext(ctx, "ask_user", args)
		answer, failed := codexToolText(raw)
		if err != nil {
			answer, failed = err.Error(), true
		}
		c.emit(Event{Kind: EvToolResult, Tool: "ask_user", CallID: id, Result: answer, Failed: failed})
		if failed {
			return nil, errors.New(answer)
		}
		answer = strings.TrimPrefix(strings.TrimPrefix(answer, "The user chose: "), "The user answered: ")
		answers[q.ID] = map[string]any{"answers": []string{answer}}
	}
	return map[string]any{"answers": answers}, nil
}
