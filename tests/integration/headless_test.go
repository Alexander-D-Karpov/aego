package integration

import (
	"testing"

	"aego/core/log"
	"aego/demos/basic"
	"aego/engine"
)

func newEngine(t *testing.T) (*engine.Engine, *basic.Demo) {
	t.Helper()
	e, err := engine.NewBuilder(engine.Config{
		Title:             suite,
		Headless:          true,
		HeadlessFrameTime: 1.0 / 60,
		ResRoot:           "../../demos/res",
		UserRoot:          t.TempDir(),
		AppID:             "aego-integration",
	}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	return e, basic.New()
}

func TestHeadlessSmoke(t *testing.T) {
	e, demo := newEngine(t)
	defer e.Close()
	for i := 0; i < 600; i++ {
		if !e.Step(demo) {
			t.Fatalf("engine stopped at frame %d: %v", i, e.Err())
		}
	}
	s := e.Stats()
	if s.FrameTime <= 0 {
		t.Fatalf("frame time = %f", s.FrameTime)
	}
	if s.DrawCalls != 1 {
		t.Fatalf("null backend reported %d draw calls, want 1", s.DrawCalls)
	}
}
