package integration

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/app/agentui/backend"
	"aego/editor/core/capture"
	"aego/editor/panels/sceneview"
)

// Scene View screenshots must be read after the GPU has ended the frame.
func TestSceneViewCaptureAfterFrame(t *testing.T) {
	s, root := agentSession(t, &backend.Fake{})
	for range 3 {
		s.Step()
	}
	if err := s.App.Context().Commands.Run(sceneview.CommandCapture); err != nil {
		t.Fatal(err)
	}
	s.Step()
	entries, err := os.ReadDir(filepath.Join(root, capture.Dir))
	if err != nil || len(entries) != 1 {
		t.Fatalf("Scene View screenshot: files=%v, err=%v", entries, err)
	}
	f, err := os.Open(filepath.Join(root, capture.Dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil || img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatalf("invalid Scene View screenshot: %v", err)
	}
}
