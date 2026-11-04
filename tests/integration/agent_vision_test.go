package integration

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
)

const probePath = "assets/probe.png"

func visualChallenge(t *testing.T, e *bridgeEnv) (*image.NRGBA, string) {
	t.Helper()
	var entropy [15]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 64, 40))
	kind := "square"
	for i := 0; i < 2; i++ {
		cx := 10 + int(entropy[i]%10) + 32*i
		cy := 9 + int(entropy[i+2]%21)
		circle := int(entropy[4]%2) == i
		if i == 0 && circle {
			kind = "circle"
		}
		c := color.NRGBA{R: 32 + entropy[6+3*i]%190, G: 32 + entropy[7+3*i]%190, B: 32 + entropy[8+3*i]%190, A: 255}
		for y := -6; y <= 6; y++ {
			for x := -6; x <= 6; x++ {
				if !circle || x*x+y*y <= 36 {
					img.SetNRGBA(cx+x, cy+y, c)
				}
			}
		}
	}
	f, err := os.Create(filepath.Join(e.root, probePath))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, img)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.ctx.Pipeline.Scan(); err != nil {
		t.Fatal(err)
	}
	return img, kind
}

type mcpImageResult struct {
	Content []struct {
		Type, MimeType, Text string
		Data                 []byte
	}
	IsError bool
}

func readResult(t *testing.T, e *bridgeEnv, tool string, args map[string]any) mcpImageResult {
	t.Helper()
	server := e.b.Backend()
	client := backend.NewClient(server.MCPURL, server.Token)
	var raw json.RawMessage
	var err error
	done := make(chan struct{})
	go func() { defer close(done); raw, err = client.CallRaw(tool, args) }()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal(tool + " timed out")
		case <-done:
			if err != nil {
				t.Fatal(err)
			}
			var result mcpImageResult
			if err = json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("%s: %s", tool, raw)
			}
			return result
		default:
			e.s.Step()
		}
	}
}
func checkVisualEdit(t *testing.T, e *bridgeEnv, before *image.NRGBA) {
	t.Helper()
	f, err := os.Open(filepath.Join(e.root, probePath))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	after, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if after.Bounds() != before.Bounds() {
		t.Fatal("dimensions changed")
	}
	for y := 0; y < 40; y++ {
		for x := 0; x < 64; x++ {
			want := before.NRGBAAt(x, y)
			if x < 32 && want.A != 0 {
				want = color.NRGBA{255, 255, 255, 255}
			}
			if got := color.NRGBAModel.Convert(after.At(x, y)); got != want {
				t.Fatalf("unexpected edit at %d,%d", x, y)
			}
		}
	}
}
func TestAgentViewPixelsThenEdit(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	img, _ := visualChallenge(t, e)
	e.b.SetAutoApply(true)
	res := readResult(t, e, "asset_view", map[string]any{"asset": probePath, "view": "source"})
	if len(res.Content) != 2 || res.Content[0].Type != "text" || res.Content[1].Type != "image" || res.Content[1].MimeType != "image/png" {
		t.Fatal("expected metadata and a distinct PNG block")
	}
	seen, err := png.Decode(bytes.NewReader(res.Content[1].Data))
	if err != nil {
		t.Fatal(err)
	}
	x, y := -1, -1
	for j := 0; j < 40 && x < 0; j++ {
		for i := 0; i < 32; i++ {
			_, _, _, a := seen.At(i, j).RGBA()
			if a != 0 {
				x, y = i, j
				break
			}
		}
	}
	if x < 0 {
		t.Fatal("no left shape")
	}
	readResult(t, e, "asset_paint", map[string]any{"path": probePath, "replace": true, "ops": []any{map[string]any{"op": "fill", "x": x, "y": y, "color": "#ffffff"}}})
	checkVisualEdit(t, e, img)
}

func TestLiveAgentVision(t *testing.T) {
	provider := os.Getenv("AEGO_LIVE_VISION_PROVIDER")
	if provider == "" {
		t.Skip("opt in with AEGO_LIVE_VISION_PROVIDER=codex or claude")
	}
	e := bridgeSetup(t, agentbridge.Config{})
	img, kind := visualChallenge(t, e)
	e.b.SetAutoApply(true)
	adapter, err := backend.ForProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	cfg := e.b.Backend()
	cfg.Model = os.Getenv("AEGO_LIVE_VISION_MODEL")
	if exe := os.Getenv("AEGO_LIVE_VISION_COMMAND"); exe != "" {
		cfg.Command = exe
	}
	var conv backend.Conversation
	started := make(chan struct{})
	go func() { conv, err = adapter.Start(cfg); close(started) }()
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
	wait := true
	for wait {
		select {
		case <-started:
			wait = false
		case <-deadline.C:
			t.Fatal("provider did not start")
		default:
			e.s.Step()
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conv.Close()
	if err = conv.Send("Use asset_view (source) on assets/probe.png. Identify the shape on the left. Use asset_paint to fill only that shape solid white, preserving all other pixels. Answer with only the original left shape in English: circle or square."); err != nil {
		t.Fatal(err)
	}
	var answer strings.Builder
	var sawImage, edited bool
	for {
		select {
		case <-deadline.C:
			t.Fatal("live vision turn timed out")
		case ev, ok := <-conv.Events():
			if !ok {
				t.Fatal("provider closed before turn completion")
			}
			switch ev.Kind {
			case backend.EvError:
				t.Fatal(ev.Err)
			case backend.EvToolResult:
				if !ev.Failed && len(ev.Images) > 0 {
					sawImage = true
				}
			case backend.EvToolStart:
				if ev.Tool == "asset_paint" {
					edited = sawImage
				}
			case backend.EvText:
				answer.WriteString(ev.Text)
			case backend.EvTurnEnd:
				if !sawImage || !edited || strings.Trim(strings.ToLower(strings.TrimSpace(answer.String())), ". !") != kind {
					t.Fatalf("vision not demonstrated: read=%t edit=%t answer=%q", sawImage, edited, answer.String())
				}
				checkVisualEdit(t, e, img)
				return
			}
		default:
			e.s.Step()
		}
	}
}
