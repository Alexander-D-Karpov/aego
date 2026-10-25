package integration

import (
	"os"
	"strings"
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/modules/input"
	"aego/project"
)

// A destroyed enemy must not report its own tracks as lost (ANM005).
func TestDeathDoesNotReportLostTracks(t *testing.T) {
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	mem := &log.MemorySink{}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.New(mem)}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	pc := s.App.Play()
	if err := pc.Start(s.App.Context().Doc()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := input.Of(pc.World()).Replay(data); err != nil {
		t.Fatal(err)
	}
	for range runFrames {
		pc.Tick(1.0 / 60)
	}
	sc := pc.Scene()
	sc.Create(sc.Root(), "Late")
	for range 5 {
		pc.Tick(1.0 / 60)
	}
	if err := pc.Seek(runFrames / 3); err != nil {
		t.Fatal(err)
	}
	pc.Pause()
	for range runFrames {
		pc.Tick(1.0 / 60)
	}
	t.Logf("%d records", len(mem.Records()))
	for _, r := range mem.Records() {
		if strings.Contains(r.Msg, "ANM005") {
			t.Fatalf("lost track reported: %s %v", r.Msg, r.Fields)
		}
	}
}
