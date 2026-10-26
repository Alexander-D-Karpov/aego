//go:build aego_gltest

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/app/run"
)

// scene_run captures the frames it is asked for: the answer carries them scaled to 640 wide, the full frames stay under .aego/cache/runs.
func TestAgentRunScreenshots(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{Run: run.AgentRunner(combatModule{})})
	client := backend.NewClient(e.b.Backend().MCPURL, e.b.Backend().Token)
	var raw json.RawMessage
	var err error
	done := make(chan struct{})
	go func() {
		raw, err = client.CallRaw("scene_run", map[string]any{"file": bridgeScene, "frames": 40, "shot": "20,40"})
		close(done)
	}()
	for waiting := true; waiting; {
		select {
		case <-done:
			waiting = false
		default:
			e.s.Step()
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Content []struct {
			Type, Text, Data, MimeType string
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	images := 0
	for _, c := range res.Content {
		if c.Type == "image" && c.MimeType == "image/png" && len(c.Data) > 1000 {
			images++
		}
	}
	if images != 2 {
		t.Fatalf("the answer has %d images", images)
	}
	var out struct {
		Value agent.RunReport `json:"value"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
		t.Fatal(err)
	}
	for _, s := range out.Value.Shots {
		if s.Error != "" || s.Width != 640 {
			t.Fatalf("shot %+v", s)
		}
		if _, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(s.Path))); err != nil {
			t.Fatal(err)
		}
	}
}
