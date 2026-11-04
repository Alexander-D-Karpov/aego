//go:build aego_gltest

package integration

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"strings"
	"testing"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/app/run"
)

func TestAgentCompareGameShotsAndCameraIdentity(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{Run: run.AgentRunner(combatModule{})})
	call := func(tool string, args map[string]any) (bool, []string, [][]byte) {
		t.Helper()
		client := backend.NewClient(e.b.Backend().MCPURL, e.b.Backend().Token)
		var raw json.RawMessage
		var err error
		done := make(chan struct{})
		go func() { raw, err = client.CallRaw(tool, args); close(done) }()
		for {
			select {
			case <-done:
				goto received
			default:
				e.s.Step()
			}
		}
	received:
		if err != nil {
			t.Fatal(err)
		}
		var rsp struct {
			IsError bool
			Content []struct{ Type, Text, Data, MimeType string }
		}
		if err := json.Unmarshal(raw, &rsp); err != nil {
			t.Fatal(err)
		}
		var texts []string
		var pictures [][]byte
		for _, c := range rsp.Content {
			switch c.Type {
			case "text":
				texts = append(texts, c.Text)
			case "image":
				if c.MimeType != "image/png" {
					t.Fatal(c.MimeType)
				}
				d, err := base64.StdEncoding.DecodeString(c.Data)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := png.DecodeConfig(bytes.NewReader(d)); err != nil {
					t.Fatal(err)
				}
				pictures = append(pictures, d)
			}
		}
		return rsp.IsError, texts, pictures
	}
	a := map[string]any{"file": bridgeScene, "camera": "Camera", "frames": 4, "seed": 41, "shot": "4", "scale": 300}
	shot := func() (string, []byte) {
		t.Helper()
		bad, text, pics := call("scene_run", a)
		if bad || len(pics) != 1 {
			t.Fatal(text)
		}
		var out struct {
			Value struct {
				Shots []struct {
					CaptureID string
					Frame     int
				}
			}
		}
		if err := json.Unmarshal([]byte(text[0]), &out); err != nil || len(out.Value.Shots) != 1 || out.Value.Shots[0].CaptureID == "" || out.Value.Shots[0].Frame != 4 {
			t.Fatal(text, err)
		}
		return out.Value.Shots[0].CaptureID, pics[0]
	}
	before, oldPNG := shot()
	ev := results(e.turn(t, []backend.Step{{Tool: "scene_set", Args: map[string]any{"file": bridgeScene, "node": "BackdropStartLeft", "path": "SpriteRenderer.tint", "value": "#ff1030ff"}}}, e.approveAll))
	if len(ev) != 1 || ev[0].Failed {
		t.Fatal(ev)
	}
	after, newPNG := shot()
	if before == after || bytes.Equal(oldPNG, newPNG) {
		t.Fatal("unsaved edit did not change Game View")
	}
	bad, text, image := call("scene_compare_frames", map[string]any{"before": before, "after": after})
	if bad || len(image) != 2 || !bytes.Equal(image[0], oldPNG) || !bytes.Equal(image[1], newPNG) {
		t.Fatal("comparison must deliver both original PNGs in order", text)
	}
	e.turn(t, []backend.Step{{Tool: "scene_set", Args: map[string]any{"file": bridgeScene, "node": "Camera", "path": "Camera2D.zoom", "value": "7"}}}, e.approveAll)
	moved, _ := shot()
	bad, text, _ = call("scene_compare_frames", map[string]any{"before": before, "after": moved})
	if !bad || !strings.Contains(strings.Join(text, " "), "settings differ") {
		t.Fatal("compared captures with different views", text)
	}
	a["camera"] = "NoSuchCamera"
	bad, text, _ = call("scene_run", a)
	if !bad || !strings.Contains(strings.Join(text, " "), "NoSuchCamera") {
		t.Fatal("a missing camera returned a successful capture", text)
	}
}
