package mcp

import (
	"bufio"
	"encoding/json"
	"io"

	"aego/app/agent"
)

// Relay passes an outside agent's MCP session to the open editor, so its changes become the editor's Undo steps, named after the client.
func Relay(in io.Reader, out io.Writer, addr, token string) error {
	client := "agent"
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), maxBody)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		params, _ := msg["params"].(map[string]any)
		switch msg["method"] {
		case "initialize":
			if info, ok := params["clientInfo"].(map[string]any); ok {
				if name, ok := info["name"].(string); ok && name != "" {
					client = name
				}
			}
		case "tools/call":
			if params != nil {
				args, _ := params["arguments"].(map[string]any)
				if args == nil {
					args = map[string]any{}
				}
				args[agent.ViaArg] = "via agent: " + client
				params["arguments"] = args
			}
		}
		body, _ := json.Marshal(msg)
		rep, err := agent.ProxyPost(addr, token, body)
		if err != nil {
			id, ok := msg["id"]
			if !ok {
				continue
			}
			rep, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": "the editor closed; start the MCP server again: " + err.Error()}})
		}
		if len(rep) == 0 {
			continue
		}
		if _, err := out.Write(append(rep, '\n')); err != nil {
			return err
		}
	}
	return sc.Err()
}
