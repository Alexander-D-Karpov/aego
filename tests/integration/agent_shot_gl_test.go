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

// shotRun calls scene_run through the bridge, stepping the editor while it waits, and returns the images and the report.
func shotRun(t *testing.T, e *bridgeEnv, args map[string]any) (int, agent.RunReport) {
	t.Helper()
	client := backend.NewClient(e.b.Backend().MCPURL, e.b.Backend().Token)
	var raw json.RawMessage
	var err error
	done := make(chan struct{})
	go func() {
		raw, err = client.CallRaw("scene_run", args)
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
	var out struct {
		Value agent.RunReport `json:"value"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
		t.Fatal(err)
	}
	return images, out.Value
}

// scene_run captures the frames it is asked for: the answer carries them scaled to 640 wide, the full frames stay under .aego/cache/runs.
func TestAgentRunScreenshots(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{Run: run.AgentRunner(combatModule{})})
	images, rep := shotRun(t, e, map[string]any{"file": bridgeScene, "frames": 40, "shot": "20,40"})
	if images != 2 {
		t.Fatalf("the answer has %d images", images)
	}
	for _, s := range rep.Shots {
		if s.Error != "" || s.Width != 640 {
			t.Fatalf("shot %+v", s)
		}
		if _, err := os.Stat(filepath.Join(e.root, filepath.FromSlash(s.Path))); err != nil {
			t.Fatal(err)
		}
	}
}

// A run with screenshots steps like one without: the clock of slow software rendering does not change the game; audio runs on its own clock and stays out (acceptance of 11b6).
func TestAgentRunScreenshotsKeepTheSimulation(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{Run: run.AgentRunner(combatModule{})})
	args := map[string]any{"file": bridgeScene, "frames": 120, "input": "1-110 axis Move 1; 30 press Jump", "watch": "Player"}
	_, plain := shotRun(t, e, args)
	args["shot"] = "60,120"
	_, shot := shotRun(t, e, args)
	same := len(plain.Tracks) == 1 && len(shot.Tracks) == 1 && plain.Counters["gameplay.steps"] == shot.Counters["gameplay.steps"]
	for i := range plain.Tracks[0].At {
		same = same && i < len(shot.Tracks[0].At) && plain.Tracks[0].At[i] == shot.Tracks[0].At[i]
	}
	if !same {
		t.Fatalf("the screenshot run moved the player otherwise: %v in %d steps, without screenshots %v in %d", shot.Tracks, shot.Counters["gameplay.steps"], plain.Tracks, plain.Counters["gameplay.steps"])
	}
}
