//go:build aego_gltest

package edit

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/asset"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/capture"
	"aego/editor/core/command"
	"aego/editor/core/workspace"
	"aego/editor/panels/sceneview"
	"aego/modules/render2d"
	"aego/scene"
)

func TestSceneViewCaptureAfterEndFrameGL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := newProject(t)
	const path = "scenes/capture.aescene"
	writeScene(t, p, path, func(sc *scene.Scene) {
		n := sc.Create(sc.Root(), "Red sprite")
		sprite := scene.Add[render2d.SpriteRenderer](sc, n)
		*sprite = render2d.DefaultSpriteRenderer()
		sprite.Sprite = asset.NewRef[asset.Texture](asset.BuiltinWhite)
		sprite.Tint = mathx.Color{R: 1, A: 1}
		sprite.Size = mathx.V2(4, 4)
	})
	s, err := Open(p, Config{Width: 800, Height: 600, Hidden: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	if _, err := ed.Docs.Open(path); err != nil {
		t.Fatal(err)
	}
	ed.Settings.Grid = false
	for range 3 {
		s.Step()
	}
	if err := ed.Commands.Run(sceneview.CommandCapture); err != nil {
		t.Fatal(err)
	}
	s.Step()
	dir := filepath.Join(p.Root, capture.Dir)
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("Scene View capture: entries=%v err=%v", entries, err)
	}
	f, err := os.Open(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	image, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	red := 0
	bounds := image.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := image.At(x, y).RGBA()
			if r > 0xa000 && g < 0x5000 && b < 0x5000 {
				red++
			}
		}
	}
	if red < 100 {
		t.Fatalf("capture has only %d red sprite pixels", red)
	}
}

// The public capture and the on-screen viewport must have the same vertical
// orientation. Two colors catch a vertical flip that a single red object does
// not detect; this tests the actual GL render target and after-frame readback.
func TestSceneViewCaptureOrientationMatchesDisplayGL(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := newProject(t)
	const path = "scenes/capture-orientation.aescene"
	writeScene(t, p, path, func(sc *scene.Scene) {
		cam := sc.Create(sc.Root(), "Camera")
		*scene.Add[render2d.Camera2D](sc, cam) = render2d.DefaultCamera2D()
		for _, item := range []struct {
			name string
			y    float32
			col  mathx.Color
		}{{"Red top", -2, mathx.Color{R: 1, A: 1}}, {"Blue bottom", 2, mathx.Color{B: 1, A: 1}}} {
			n := sc.Create(sc.Root(), item.name)
			pose := sc.Local(n)
			pose.Position.Y = item.y
			sc.SetLocal(n, pose)
			sprite := scene.Add[render2d.SpriteRenderer](sc, n)
			*sprite = render2d.DefaultSpriteRenderer()
			sprite.Sprite = asset.NewRef[asset.Texture](asset.BuiltinWhite)
			sprite.Tint, sprite.Size = item.col, mathx.V2(2, 2)
		}
	})
	s, err := Open(p, Config{Width: 800, Height: 600, Hidden: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Step() {
		t.Fatal("editor stopped during startup")
	}
	ed := s.App.Context()
	doc, err := ed.Docs.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ed.Settings.Grid = false
	if err := ed.Commands.Run(command.EditSelectAll); err != nil {
		t.Fatal(err)
	}
	if err := ed.Commands.Run(command.SceneFrame); err != nil {
		t.Fatal(err)
	}
	ed.Select()
	ws := s.App.Workspace()
	ws.Focus(workspace.Scene)
	if !ws.ToggleMaximize(workspace.Scene) {
		t.Fatal("could not maximize Scene View")
	}
	for range 35 {
		if !s.Step() {
			t.Fatal("editor stopped while framing the scene")
		}
	}
	var displayed *image.RGBA
	var readErr error
	s.Engine.CaptureFrame(func(img *image.RGBA, err error) { displayed, readErr = img, err })
	if !s.Step() || readErr != nil || displayed == nil {
		t.Fatalf("capture displayed frame: %v", readErr)
	}
	encoded, _, gotPath, err := s.App.CaptureAgentView("scene")
	if err != nil || gotPath != path {
		t.Fatalf("capture Scene View: %q, %v", gotPath, err)
	}
	view, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.Commands.Run(sceneview.CommandCapture); err != nil {
		t.Fatal(err)
	}
	if !s.Step() {
		t.Fatal("editor stopped while saving Scene View")
	}
	dir := filepath.Join(p.Root, capture.Dir)
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("saved Scene View: %v, %v", entries, err)
	}
	f, err := os.Open(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := png.Decode(f)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("decode saved Scene View: %v, %v", err, closeErr)
	}
	// Compare ordering to the actual presented editor frame instead of assuming
	// a world or graphics-backend coordinate convention.
	order := func(img image.Image) bool {
		t.Helper()
		var redY, blueY, redN, blueN int64
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, blue, _ := img.At(x, y).RGBA()
				if r > 0xa000 && g < 0x3000 && blue < 0x3000 {
					redY += int64(y)
					redN++
				}
				if blue > 0xa000 && r < 0x3000 && g < 0x3000 {
					blueY += int64(y)
					blueN++
				}
			}
		}
		if redN < 100 || blueN < 100 {
			t.Fatalf("capture lacks the two colored markers: red=%d blue=%d", redN, blueN)
		}
		return redY*blueN < blueY*redN
	}
	want := order(displayed)
	if got := order(view); got != want {
		t.Errorf("Agent Scene View capture is vertically flipped: display red-above-blue=%v, capture=%v", want, got)
	}
	if got := order(saved); got != want {
		t.Errorf("saved Scene View is vertically flipped: display red-above-blue=%v, saved=%v", want, got)
	}
	if err := s.App.Play().Start(doc); err != nil {
		t.Fatal(err)
	}
	ws.ToggleMaximize(workspace.Scene)
	ws.Focus(workspace.Game)
	if !ws.ToggleMaximize(workspace.Game) {
		t.Fatal("could not maximize Game View")
	}
	for range 10 {
		if !s.Step() {
			t.Fatal("editor stopped while showing Game View")
		}
	}
	displayed = nil
	s.Engine.CaptureFrame(func(img *image.RGBA, err error) { displayed, readErr = img, err })
	if !s.Step() || readErr != nil || displayed == nil {
		t.Fatalf("capture displayed Game View: %v", readErr)
	}
	encoded, _, gotPath, err = s.App.CaptureAgentView("game")
	if err != nil || gotPath != path {
		t.Fatalf("capture Game View: %q, %v", gotPath, err)
	}
	view, err = png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	want = order(displayed)
	if got := order(view); got != want {
		t.Errorf("Agent Game View capture is vertically flipped: display red-above-blue=%v, capture=%v", want, got)
	}
}
