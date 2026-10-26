package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// editorLock is where the open editor says how to reach its proxy (11b6).
const editorLock = ".aego/editor.lock"

// ViaArg names where a proxied call comes from; the editor shows it in the Undo step and takes it off.
const ViaArg = "_via"

// EditorProxy is the address and token of the open editor's proxy, when the lock names one and something listens there.
func EditorProxy(root string) (string, string, bool) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(editorLock)))
	if err != nil {
		return "", "", false
	}
	var addr, token string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), "=")
		switch k {
		case "proxy":
			addr = v
		case "token":
			token = v
		}
	}
	u, err := url.Parse(addr)
	if addr == "" || token == "" || err != nil {
		return "", "", false
	}
	c, err := net.DialTimeout("tcp", u.Host, 300*time.Millisecond)
	if err != nil {
		return "", "", false
	}
	c.Close()
	return addr, token, true
}

// ProxyPost sends one JSON-RPC message to the editor's proxy and returns its answer; a notification has none.
func ProxyPost(addr, token string, msg []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, addr, bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return bytes.TrimSpace(buf.Bytes()), nil
	case http.StatusAccepted:
		return nil, nil
	}
	return nil, fmt.Errorf("the editor answered %s", resp.Status)
}

// proxyRun runs a command in the open editor: its change becomes an Undo step there, named "via CLI".
func proxyRun(addr, token string, c *Command, a Args) (*Output, error) {
	args := map[string]any{}
	for k, v := range a {
		args[k] = v
	}
	args[ViaArg] = "via CLI"
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": c.Tool(), "arguments": args}})
	raw, err := ProxyPost(addr, token, msg)
	if err != nil {
		return nil, errProxyDown{err}
	}
	var rep struct {
		Result struct {
			Content []struct {
				Type, Text string
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &rep); err != nil {
		return nil, err
	}
	if rep.Error != nil {
		return nil, errors.New(rep.Error.Message)
	}
	text := ""
	for _, part := range rep.Result.Content {
		if part.Type == "text" {
			text += part.Text
		}
	}
	if rep.Result.IsError {
		return nil, errors.New(text)
	}
	var out struct {
		Output
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return &Output{OK: true, Text: text}, nil
	}
	o := out.Output
	var s string
	switch {
	case c.Name == "scene.run":
		var rep RunReport
		if json.Unmarshal(out.Value, &rep) == nil {
			r := RunOutput(rep)
			r.Notes = append(r.Notes, o.Notes...)
			return r, nil
		}
	case len(out.Value) > 0 && json.Unmarshal(out.Value, &s) == nil:
		o.Value, o.Text = s, s
	case len(out.Value) > 0:
		o.Value = out.Value
	}
	return &o, nil
}

// errProxyDown is the editor not answering; the CLI then runs the command itself.
type errProxyDown struct {
	err error
}

func (e errProxyDown) Error() string {
	return "the editor did not answer: " + e.err.Error()
}
