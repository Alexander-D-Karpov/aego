package engine

import (
	"testing"

	"aego/gpu"
	"aego/platform"
)

type finishProbe struct {
	testApp
	texture gpu.TextureHandle
	calls   int
	readErr error
}

func (a *finishProbe) Init(e *Engine) error {
	a.testApp.Init(e)
	var err error
	a.texture, err = e.GPU().CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA8})
	return err
}

func (a *finishProbe) FrameFinished() {
	a.calls++
	a.readErr = a.e.GPU().ReadPixels(a.texture, 0, 0, 1, 1, make([]byte, 4))
}

func TestFrameFinisherRunsAfterEndFrameAndNotWhenMinimized(t *testing.T) {
	e, plat := newTestEngine(t)
	defer e.Close()
	a := &finishProbe{}
	if !e.Step(a) || a.calls != 1 || a.readErr != nil {
		t.Fatalf("finish calls=%d, readback error=%v", a.calls, a.readErr)
	}
	plat.Push(platform.Event{Kind: platform.EvWindowMinimized})
	e.Step(a)
	if a.calls != 1 {
		t.Fatal("finisher ran for a minimized frame")
	}
}
