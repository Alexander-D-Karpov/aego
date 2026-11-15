//go:build aego_editor

package docsview

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/engine/docs"
	"aego/engine/i18n"
	"aego/modules/camera"
)

func newHeadless(t *testing.T) *App {
	t.Helper()
	a, err := New(Options{Headless: true, FrameTime: 1.0 / 60})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

// The component example has its own controlled setup; the guide must retain
// its original falling-crate example and its collision-triggered shake.
func TestCameraShakeGuideStillUsesFallingCrate(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("camera/shake")
	if !ok || !strings.Contains(e.Try, `script "game.demo.Drop"`) || !strings.Contains(e.TryScript, "func OnCollisionEnter") {
		t.Fatal("the camera guide lost its falling-crate scene or collision script")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", req); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	var maxTrauma float32
	for range 400 {
		if err := a.Step(1); err != nil {
			t.Fatal(err)
		}
		if s := camera.Of(a.session.Controller().World()); s != nil {
			maxTrauma = max(maxTrauma, s.Trauma(""))
		}
	}
	if maxTrauma < 0.1 {
		t.Fatalf("the falling crate never triggered a camera shake: max trauma %g", maxTrauma)
	}
	if messages := errorsOf(a); len(messages) > 0 {
		t.Fatalf("guide runtime errors: %v", messages)
	}
}

func tries(t *testing.T) []docs.Entry {
	t.Helper()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	out := p.TryScenes()
	if len(out) < 10 {
		t.Fatalf("only %d try scenes; does the module embed res/docs/try?", len(out))
	}
	return out
}

func errorsOf(a *App) []string {
	var out []string
	for _, l := range a.lines {
		if l.Error {
			out = append(out, l.Text)
		}
	}
	a.lines = nil
	return out
}

func TestTryScenesRun(t *testing.T) {
	a := newHeadless(t)
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tries(t) {
		var code string
		if e.TryScript != "" {
			if code, err = p.TryCode(e.TryScript); err != nil {
				t.Fatalf("%s: %v", e.ID, err)
			}
		}
		req, _ := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
		if r := a.Call("load", req); r.Error != "" {
			t.Fatalf("%s: %s", e.ID, r.Error)
		}
		if err := a.Step(400); err != nil {
			t.Fatalf("%s: %v", e.ID, err)
		}
		st := a.session.State()
		if !st.Running || st.Frames < 30 {
			t.Fatalf("%s: %+v", e.ID, st)
		}
		if len(a.session.Selected()) != 1 {
			t.Fatalf("%s: nothing selected", e.ID)
		}
		for _, msg := range errorsOf(a) {
			t.Errorf("%s: %s", e.ID, msg)
		}
	}
}

// At high render frame rates there may be several Update calls before the first
// physics tick. The actual CameraFollow demo must not teleport an unbuilt body.
// This deterministic headless timing check supplements, not replaces, the
// separate SDL/OpenGL and browser rendering checks.
func TestCameraFollowTryWaitsForPhysicsAtHighFrameRate(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("camera.CameraFollow2D")
	if !ok || entry.Try == "" || entry.TryScript == "" {
		t.Fatal("CameraFollow2D Try scene or script is missing")
	}
	code, err := p.TryCode(entry.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{Headless: true, FrameTime: 0.001})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	request, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "prefabs": entry.TryPrefabs, "artifacts": entry.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", request); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	world := a.session.Controller().World()
	if world == nil || world.Time.Tick() != 0 {
		t.Fatal("expected several render frames before the first physics step")
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("before physics: %v", messages)
	}
	if err := a.Step(100); err != nil {
		t.Fatal(err)
	}
	if world.Time.Tick() < 2 {
		t.Fatal("physics did not advance")
	}
	if messages := errorsOf(a); len(messages) != 0 {
		t.Fatalf("after physics: %v", messages)
	}
	sc := a.session.Controller().Scene()
	if sc == nil {
		t.Fatal("the Try scene stopped before the teleport")
	}
	ball := sc.Find(sc.Root(), "Ball")
	if ball.IsZero() || math.Abs(float64(sc.Local(ball).Position.X+18)) > 0.01 {
		t.Fatal("the ball did not teleport after physics initialized")
	}
}

// A slow frame can contain several fixed updates. The camera must observe the
// starting pose before the teleport so changing SnapDistance remains visible.
func TestCameraFollowTrySnapsWithSeveralPhysicsStepsPerFrame(t *testing.T) {
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("camera.CameraFollow2D")
	if !ok || entry.Try == "" || entry.TryScript == "" {
		t.Fatal("CameraFollow2D Try scene or script is missing")
	}
	code, err := p.TryCode(entry.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{Headless: true, FrameTime: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	request, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "prefabs": entry.TryPrefabs, "artifacts": entry.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if reply := a.Call("load", request); reply.Error != "" {
		t.Fatal(reply.Error)
	}
	// The contract always begins from Reset, after the first engine frame.
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	if err := a.session.Reset(); err != nil {
		t.Fatal(err)
	}
	position := func() float32 {
		t.Helper()
		if err := a.Step(8); err != nil {
			t.Fatal(err)
		}
		if messages := errorsOf(a); len(messages) != 0 {
			t.Fatalf("Try runtime: %v", messages)
		}
		world := a.session.Controller().World()
		if world == nil || world.Time.Tick() <= world.Time.Frame() {
			t.Fatal("expected more physics steps than rendered frames")
		}
		views := a.session.Controller().Views()
		if len(views) != 1 {
			t.Fatalf("expected one game view, got %d", len(views))
		}
		return views[0].Camera.Position.X
	}
	before := position()
	if err := a.session.Set("camera.CameraFollow2D", "SnapDistance", 1); err != nil {
		t.Fatal(err)
	}
	after := position()
	if math.Abs(float64(after-before)) < 4 {
		t.Fatalf("camera did not visibly snap to the teleported target: before=%g after=%g", before, after)
	}
	if err := a.session.Reset(); err != nil {
		t.Fatal(err)
	}
	reset := position()
	if math.Abs(float64(reset-before)) > 0.01 {
		t.Fatalf("Reset did not restore the camera: before=%g reset=%g", before, reset)
	}
}

func TestEditRestartsUndoReset(t *testing.T) {
	a := newHeadless(t)
	var body docs.Entry
	for _, e := range tries(t) {
		if e.ID == "physics.Body2D" {
			body = e
		}
	}
	if body.Try == "" {
		t.Fatal("physics.Body2D has no try scene")
	}
	s := a.session
	if err := s.Load([]byte(body.Try), body.TrySelect); err != nil {
		t.Fatal(err)
	}
	a.Step(10)
	if err := s.Set("physics.BoxCollider2D", "restitution", 0.75); err != nil {
		t.Fatal(err)
	}
	a.Step(2)
	if !strings.Contains(string(s.Text()), "restitution=0.75") {
		t.Fatalf("the scene text did not follow the edit:\n%s", s.Text())
	}
	if st := s.State(); st.Restarts != 1 || !st.CanUndo || !st.Running {
		t.Fatalf("after edit %+v", st)
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	a.Step(2)
	if strings.Contains(string(s.Text()), "restitution=0.75") {
		t.Fatal("undo kept the edit")
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	a.Step(2)
	if st := s.State(); st.CanUndo || st.Restarts != 0 && st.Restarts != 2 {
		t.Fatalf("after reset %+v", st)
	}
}

// The site's theme switch reaches the Try it view (D345).
func TestThemeCall(t *testing.T) {
	a := newHeadless(t)
	if a.ui == nil {
		t.Skip("no UI in headless mode")
	}
	if r := a.Call("theme", []byte(`{"theme":"light"}`)); r.Error != "" || a.ui.Theme.Dark {
		t.Fatalf("theme light: %q, dark %v", r.Error, a.ui.Theme.Dark)
	}
	if a.Call("theme", []byte(`{"theme":"dark"}`)); !a.ui.Theme.Dark {
		t.Fatal("theme dark left the light theme on")
	}
}

// The page language reaches the Try it view (D348).
func TestLangCall(t *testing.T) {
	a := newHeadless(t)
	t.Cleanup(func() { i18n.Default().SetLanguage(i18n.English) })
	if r := a.Call("lang", []byte(`{"lang":"ru"}`)); r.Error != "" || i18n.Default().Language() != i18n.Russian {
		t.Fatalf("lang ru: %q, language %q", r.Error, i18n.Default().Language())
	}
	if got := i18n.Tr("editor.menu.file", "File"); got == "File" {
		t.Fatal("the editor texts stay English after lang ru")
	}
}
