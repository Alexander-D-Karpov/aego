//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/audio/codec/wav"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/assetops"
	"aego/editor/core/document"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
)

// Exercise the real document factory and workspace with synthetic PCM and
// dummy audio. This is an offscreen test, not physical display/audio acceptance.
func TestAudioDocumentWaveNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bundle := i18n.Default()
	oldLanguage := bundle.Language()
	defer bundle.SetLanguage(oldLanguage)
	bundle.SetLanguage(i18n.English)
	p := newProject(t)
	s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	interopStep(t, s)
	ed, ws := s.App.Context(), s.App.Workspace()
	if !ws.Switch(workspace.Audio) {
		t.Fatal("Audio workspace is unavailable")
	}
	pcm := make([]float32, 48000*2)
	for frame := range 48000 {
		pcm[frame*2] = .6 * float32(math.Sin(float64(frame)*.03))
		pcm[frame*2+1] = .2 * float32(math.Sin(float64(frame)*.05))
	}
	source := wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})
	const name = "assets/generated-audio.wav"
	id, err := assetops.CreateFile(ed, name, source)
	if err != nil {
		t.Fatal("create and import synthetic audio", err)
	}
	metadata, err := s.Pipeline.SourceBytes(name + ".meta")
	if err != nil {
		t.Fatal(err)
	}
	art, ok := s.Pipeline.PrimaryArtifact(id)
	if !ok {
		t.Fatal("missing imported audio artifact")
	}
	artifact, err := s.Pipeline.Artifacts().ReadFile(art.Path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ed.OpenAsset(id)
	if err != nil || doc == nil || doc.Handler != "audio.clip" || doc.Source != nil || doc.Metadata == nil ||
		!doc.Capabilities.Has(document.EditMetadata) || doc.Dirty() {
		t.Fatal("audio did not open as a clean shared document", err)
	}
	panelID := "asset.document/" + id.String()
	if !ws.Visible(panelID) || !ws.ToggleMaximize(panelID) {
		t.Fatal("audio document did not open or maximize")
	}
	findText := func(text string) (mathx.Rect, bool) {
		t.Helper()
		panel, ok := ws.Tree.PanelRect(panelID)
		if !ok {
			return mathx.Rect{}, false
		}
		ui := ed.UI
		for _, item := range s.App.UIList().TestDrawnItems(mathx.Rect{W: ui.Size().X, H: ui.Size().Y}, ui.Font(), text, 0, 0) {
			if item.Text == text && panel.Contains(item.Bounds.Center()) && item.Bounds.X >= item.Clip.X &&
				item.Bounds.X+item.Bounds.W <= item.Clip.X+item.Clip.W+.01 && item.Bounds.Y >= item.Clip.Y &&
				item.Bounds.Y+item.Bounds.H <= item.Clip.Y+item.Clip.H+.01 {
				return item.Bounds, true
			}
		}
		return mathx.Rect{}, false
	}
	deadline := time.Now().Add(12 * time.Second)
	var left, right mathx.Rect
	for time.Now().Before(deadline) {
		interopStep(t, s)
		var hasLeft, hasRight bool
		left, hasLeft = findText("L")
		right, hasRight = findText("R")
		if hasLeft && hasRight {
			break
		}
	}
	if right.Y-left.Y < 100 {
		t.Fatalf("central stereo waveform did not use its available height: L=%+v R=%+v", left, right)
	}
	var beforeVisible string
	for _, item := range s.App.UIList().TestDrawnItems(mathx.Rect{W: ed.UI.Size().X, H: ed.UI.Size().Y}, ed.UI.Font(), "Visible:", 0, 0) {
		if strings.HasPrefix(item.Text, "Visible:") {
			beforeVisible = item.Text
			break
		}
	}
	plus, ok := findText("+")
	if !ok || beforeVisible == "" {
		t.Fatal("wave navigation controls are unavailable")
	}
	interopClick(t, s, plus.Center().X, plus.Center().Y)
	interopStep(t, s)
	changed := false
	for _, item := range s.App.UIList().TestDrawnItems(mathx.Rect{W: ed.UI.Size().X, H: ed.UI.Size().Y}, ed.UI.Font(), "Visible:", 0, 0) {
		if strings.HasPrefix(item.Text, "Visible:") && item.Text != beforeVisible {
			changed = true
		}
	}
	if !changed {
		t.Fatal("zoom did not change the visible frame range")
	}
	again, err := ed.OpenAsset(id)
	if err != nil || again != doc || doc.Dirty() || doc.CanUndo() || ed.Docs.AssetDocument(id) != doc {
		t.Fatal("navigation changed or duplicated the shared document", err)
	}
	checkFiles := func() {
		t.Helper()
		if got, err := s.Pipeline.SourceBytes(name); err != nil || !bytes.Equal(got, source) {
			t.Fatal("opening or zooming changed the audio source", err)
		}
		if got, err := s.Pipeline.SourceBytes(name + ".meta"); err != nil || !bytes.Equal(got, metadata) {
			t.Fatal("opening or zooming changed metadata", err)
		}
		if got, err := s.Pipeline.Artifacts().ReadFile(art.Path); err != nil || !bytes.Equal(got, artifact) {
			t.Fatal("opening or zooming changed the imported artifact", err)
		}
	}
	checkFiles()
	if path := os.Getenv("AEGO_AUDIO_WORKSPACE_SHOT"); path != "" {
		var shot *image.RGBA
		var shotErr error
		s.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, shotErr = img, err })
		interopStep(t, s)
		if shotErr != nil || shot == nil {
			t.Fatal("audio workspace capture failed", shotErr)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, shot); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	checkFiles()
}
