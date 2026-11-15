package edit

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"aego/core/log"
	"aego/engine/problems"
)

type screenshotErrorSink struct {
	mu      sync.Mutex
	records []log.Record
}

func (s *screenshotErrorSink) Write(r log.Record) {
	if r.Level < log.Error {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, r)
}

// Screenshot generation substitutes its own scene and clip into a copied
// platformer project. Exercise that exact fixture, including the real script,
// so a stopped player is not silently accepted as a valid documentation image.
func TestScreenshotPlatformerFixtureRunsPlayerScript(t *testing.T) {
	p := platformerAuthoringProject(t)
	for _, rel := range []string{"scenes/main.aescene", "assets/anim/player_hurt.aeanim"} {
		name := filepath.Base(rel)
		data, err := os.ReadFile(filepath.Join("..", "..", "tools", "shots", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p.Root, rel), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	sink := &screenshotErrorSink{}
	cfg := headless()
	cfg.Logger = log.New(sink)
	s, err := Open(p, cfg, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	doc, err := s.App.Context().Docs.Open("scenes/main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Step() {
		t.Fatal("editor stopped after opening the screenshot scene")
	}
	if diagnostics, _ := s.App.Context().Problems.Collect(); len(diagnostics) != 0 {
		for _, d := range diagnostics {
			if d.Severity >= problems.Warning {
				t.Errorf("screenshot scene has a diagnostic: %s %s", d.Code, d.Message)
			}
		}
	}
	pc := s.App.Play()
	if err := pc.Start(doc); err != nil {
		t.Fatal(err)
	}
	for range 120 {
		pc.Tick(1.0 / 60)
	}
	pc.Stop()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for _, record := range sink.records {
		if strings.HasPrefix(record.Msg, "script stopped:") {
			t.Errorf("screenshot fixture stopped a script: %s %v", record.Msg, record.Fields)
		}
	}
}
