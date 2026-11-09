//go:build aego_editor

package docsview

import (
	"encoding/json"
	"testing"

	"aego/app/playground"
	"aego/platform/input"
)

func TestTryBridgeControlledFramesPreserveExistingRequests(t *testing.T) {
	a := newHeadless(t)
	if r := a.Call("scriptEnabled", []byte(`{"script":"missing"}`)); r.Error == "" {
		t.Fatal("scriptEnabled accepted a missing boolean")
	}
	if r := a.Call("control", []byte(`{}`)); r.Error == "" {
		t.Fatal("control accepted a missing boolean")
	}
	if r := a.Call("control", []byte(`{"enabled":true}`)); r.Error != "" {
		t.Fatalf("could not enable controlled frames: %s", r.Error)
	}
	if r := a.Call("advance", []byte(`{"frames":1}`)); r.Error == "" {
		t.Fatal("control advanced before loading a running Try")
	}
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("engine.SpriteRenderer")
	if !ok {
		t.Fatal("missing SpriteRenderer Try")
	}
	code := ""
	if e.TryScript != "" {
		code, err = p.TryCode(e.TryScript)
		if err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", raw); r.Error != "" {
		t.Fatal(r.Error)
	}
	before := a.session.State().Frames
	if err := a.Step(3); err != nil || a.session.State().Frames != before {
		t.Fatalf("presentation advanced the controlled world: %v", err)
	}
	for _, request := range []string{`{"frames":0}`, `{"frames":601}`, `{"frames":2,"keys":["F13"]}`, `{"frames":2,"keys":["D","D"]}`} {
		if r := a.Call("advance", []byte(request)); r.Error == "" {
			t.Fatalf("control accepted invalid request %s", request)
		}
	}
	if r := a.Call("advance", []byte(`{"frames":4,"keys":["D"]}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if r := a.Call("control", []byte(`{"enabled":false}`)); r.Error == "" {
		t.Fatal("control disabled while frames were still queued")
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	if got := a.session.State().Frames; got != before+4 {
		t.Fatalf("controlled world advanced %d frames, want 4", got-before)
	}
	if err := a.Step(3); err != nil || a.session.State().Frames != before+4 {
		t.Fatalf("world did not hold after controlled frames: %v", err)
	}
	if r := a.Call("control", []byte(`{"enabled":false}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if r := a.Call("advance", []byte(`{"frames":1}`)); r.Error == "" {
		t.Fatal("advance worked after control was disabled")
	}
	if r := a.Call("scriptEnabled", []byte(`{"script":"missing"}`)); r.Error == "" {
		t.Fatal("control broke the existing scriptEnabled validation")
	}
}

func TestTryBridgeReportsOnlyPresentedViewport(t *testing.T) {
	a := newHeadless(t)
	if r := a.Call("viewport", nil); r.Error == "" {
		t.Fatal("unpresented viewport was accepted")
	}
	if err := a.Step(1); err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Error    string `json:"error"`
		Viewport *struct {
			X, Y, Width, Height       float32
			CanvasWidth, CanvasHeight int
		} `json:"viewport"`
	}
	if err := json.Unmarshal([]byte(a.Reply("viewport", "")), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Error != "" || reply.Viewport == nil || reply.Viewport.Width <= 0 || reply.Viewport.Height <= 0 || reply.Viewport.Y <= 0 || reply.Viewport.CanvasWidth <= 0 || reply.Viewport.CanvasHeight <= 0 {
		t.Fatalf("invalid presented Game View rectangle: %+v", reply)
	}
}

func TestControlledTryMovesRealGuidePlayerAndResets(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("gameplay/motion")
	if !ok {
		t.Fatal("Motion guide is missing")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	if r := a.Call("control", []byte(`{"enabled":true}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	advance := func(payload string, frames int) {
		t.Helper()
		if r := a.Call("advance", []byte(payload)); r.Error != "" {
			t.Fatal(r.Error)
		}
		if err := a.Step(frames); err != nil || a.control.Remaining != 0 {
			t.Fatalf("controlled frames did not finish: %v", err)
		}
	}
	advance(`{"frames":30}`, 30)
	sc := a.session.Controller().Scene()
	player := sc.Find(sc.Root(), "Player")
	if player.IsZero() || !sc.IsAlive(player) {
		t.Fatal("guide Player is missing")
	}
	origin := sc.WorldPosition(player)
	advance(`{"frames":30,"keys":["D"]}`, 30)
	moved := sc.WorldPosition(player)
	if moved.X <= origin.X+0.1 {
		t.Fatalf("controlled D input did not move the real player: %v -> %v", origin, moved)
	}
	if err := a.Step(5); err != nil || sc.WorldPosition(player) != moved {
		t.Fatalf("unrequested presentation frames moved the player: %v", err)
	}
	if r := a.Call("advance", []byte(`{"frames":3,"keys":["D"]}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(1); err != nil || !a.e.Input().KeyDown(input.KeyD) {
		t.Fatalf("controlled key did not become active: %v", err)
	}
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	if a.control.Remaining != 0 || a.e.Input().KeyDown(input.KeyD) {
		t.Fatal("Reset retained queued frames or a synthetic pressed key")
	}
	advance(`{"frames":30}`, 30)
	sc = a.session.Controller().Scene()
	player = sc.Find(sc.Root(), "Player")
	if got := sc.WorldPosition(player); got != origin {
		t.Fatalf("Reset did not restore the guide position: %v != %v", got, origin)
	}
	if r := a.Call("advance", []byte(`{"frames":3,"keys":["D"]}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := a.Step(1); err != nil {
		t.Fatal(err)
	}
	if r := a.Call("set", []byte(`{"component":"invalid","field":"bad","value":1}`)); r.Error == "" {
		t.Fatal("an invalid field edit was accepted")
	}
	if a.control.Remaining != 2 || !a.e.Input().KeyDown(input.KeyD) {
		t.Fatal("a rejected edit unexpectedly cancelled the active input")
	}
	if r := a.Call("play", nil); r.Error != "" || !a.session.State().Paused {
		t.Fatalf("could not pause controlled Try: %s", r.Error)
	}
	if a.control.Remaining != 0 || a.e.Input().KeyDown(input.KeyD) {
		t.Fatal("Pause retained queued frames or a synthetic pressed key")
	}
	if errors := errorsOf(a); len(errors) != 0 {
		t.Fatalf("guide runtime errors: %v", errors)
	}
}

func TestTryViewportExcludesInspectorAtDifferentDPR(t *testing.T) {
	for _, width := range []int{320, 390, 520, 692, 960} {
		a, err := New(Options{Headless: true, FrameTime: 1.0 / 60, Width: width, Height: 560})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.Step(1); err != nil {
			a.Close()
			t.Fatal(err)
		}
		v := a.Call("viewport", nil).Viewport
		if v == nil {
			a.Close()
			t.Fatal("missing viewport")
		}
		r, err := v.pixelRect(width*2, 560*2)
		if err != nil {
			a.Close()
			t.Fatal(err)
		}
		if width == 692 && (v.Width != 292 || r.Max.X != 584) {
			t.Errorf("split viewport includes Inspector: %+v -> %v", v, r)
		}
		if width <= 520 && (r.Max.X != width*2 || r.Max.Y >= 560*2) {
			t.Errorf("stacked viewport includes Inspector: %+v -> %v", v, r)
		}
		if r.Min.Y <= 0 || v.CanvasWidth != width || v.CanvasHeight != 560 {
			t.Errorf("viewport includes toolbar or has incorrect canvas dimensions: %+v -> %v", v, r)
		}
		a.Close()
	}
}
