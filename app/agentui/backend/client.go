package backend

import (
	"bytes"
	"encoding/json"
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
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": c.next.Add(1), "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	if err != nil {
		return "", true, err
	}
	req, err := http.NewRequest(http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return "", true, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", true, fmt.Errorf("mcp: %s", resp.Status)
	}
	var rep struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		return "", true, err
	}
	if rep.Error != nil {
		return rep.Error.Message, true, nil
	}
	var b strings.Builder
	for _, x := range rep.Result.Content {
		b.WriteString(x.Text)
	}
	return b.String(), rep.Result.IsError, nil
}
