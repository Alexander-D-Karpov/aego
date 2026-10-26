package backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Client calls tools of the editor's MCP server, for the fake backend and tests.
type Client struct {
	URL   string
	Token string
	next  atomic.Int64
	http  http.Client
}

func NewClient(url, token string) *Client {
	return &Client{URL: url, Token: token, http: http.Client{Timeout: 15 * time.Minute}}
}

func (c *Client) Call(tool string, args map[string]any) (string, bool, error) {
	raw, err := c.CallRaw(tool, args)
	if err != nil {
		var rpc rpcErr
		if errors.As(err, &rpc) {
			return string(rpc), true, nil
		}
		return "", true, err
	}
	var res struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", true, err
	}
	var b strings.Builder
	for _, x := range res.Content {
		b.WriteString(x.Text)
	}
	return b.String(), res.IsError, nil
}

// rpcErr is an error the server answered instead of a result.
type rpcErr string

func (e rpcErr) Error() string {
	return string(e)
}

// CallRaw is the result of a tool call as the server sent it, images included.
func (c *Client) CallRaw(tool string, args map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": c.next.Add(1), "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcp: %s", resp.Status)
	}
	var rep struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		return nil, err
	}
	if rep.Error != nil {
		return nil, rpcErr(rep.Error.Message)
	}
	return rep.Result, nil
}
