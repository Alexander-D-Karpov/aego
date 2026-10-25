//go:build aego_editor

package docsview

import (
	"encoding/json"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/engine/docs"
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
		req, _ := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs})
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
