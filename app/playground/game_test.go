package playground

import (
	"strings"
	"testing"
)

func newHeadless(t *testing.T) *Playground {
	t.Helper()
	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	p.SetGame(GameConfig{Headless: true, FrameTime: 1.0 / 60})
	t.Cleanup(func() {
		if p.game != nil {
			p.game.Close()
		}
	})
	return p
}

func TestSceneExamplesRun(t *testing.T) {
	p := newHeadless(t)
	n := 0
	for _, ex := range p.Examples() {
		if !ex.Scene {
			continue
		}
		n++
		res := p.Play(ex.ID, map[string]string{"main.aes": ex.Code})
		if !res.Started {
			t.Fatalf("%s: %s %+v", ex.ID, res.Error, res.Diagnostics)
		}
		if err := p.Game().Step(180); err != nil {
			t.Fatalf("%s: %v", ex.ID, err)
		}
		poll := p.Poll()
		if len(poll.Faults) > 0 {
			t.Fatalf("%s: faults %+v", ex.ID, poll.Faults)
		}
		for _, l := range poll.Log {
			if l.Level == "ERROR" {
				t.Errorf("%s: %s", ex.ID, l.Text)
			}
		}
	}
	if n == 0 {
		t.Fatal("app/playground/examples has no scenes")
	}
}

func TestEditedScriptErrorsStopPlay(t *testing.T) {
	p := newHeadless(t)
	for _, ex := range p.Examples() {
		if !ex.Scene {
			continue
		}
		bad := strings.Replace(ex.Code, "func ", "func func ", 1)
		res := p.Play(ex.ID, map[string]string{"main.aes": bad})
		if res.Started || len(res.Diagnostics) == 0 {
			t.Fatalf("%s: a broken script started", ex.ID)
		}
		return
	}
}

func TestBench(t *testing.T) {
	p := newHeadless(t)
	if res := p.Bench(1000, true); !res.Started {
		t.Fatal(res.Error)
	}
	if err := p.Game().Step(30); err != nil {
		t.Fatal(err)
	}
	s := p.Poll().Stats
	if !s.Running || s.Sprites != 1000 || s.Frames < 30 {
		t.Fatalf("bench stats %+v", s)
	}
	p.Stop()
	if p.Poll().Stats.Running {
		t.Fatal("stop left the game running")
	}
}
