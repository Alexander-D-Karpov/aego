//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/audio/codec/wav"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/assetops"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/platform/sdl"
)

// Synthetic SDL/offscreen interaction with dummy audio, not physical acceptance.
func TestAudioSourceAnalysisNative(t *testing.T) {
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
		pcm[frame*2], pcm[frame*2+1] = .25, -.5
	}
	source := wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})
	const name = "assets/synthetic-analysis.wav"
	id, err := assetops.CreateFile(ed, name, source)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := s.Pipeline.SourceBytes(name + ".meta")
	if err != nil {
		t.Fatal(err)
	}
	art, ok := s.Pipeline.PrimaryArtifact(id)
	if !ok {
		t.Fatal("missing synthetic artifact")
	}
	artifact, err := s.Pipeline.Artifacts().ReadFile(art.Path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ed.OpenAsset(id)
	if err != nil || doc == nil || doc.Handler != "audio.clip" || doc.Dirty() {
		t.Fatal("audio document was not opened cleanly", err)
	}
	panelID := "asset.document/" + id.String()
	if !ws.Visible(panelID) || !ws.ToggleMaximize(panelID) {
		t.Fatal("audio document could not be maximized")
	}
	find := func(query string) (mathx.Rect, string, bool) {
		panel, ok := ws.Tree.PanelRect(panelID)
		if !ok {
			return mathx.Rect{}, "", false
		}
		ui := ed.UI
		for _, item := range s.App.UIList().TestDrawnItems(mathx.Rect{W: ui.Size().X, H: ui.Size().Y}, ui.Font(), query, 0, 0) {
			if strings.Contains(item.Text, query) && (query != "L" || item.Text == query) && panel.Contains(item.Bounds.Center()) && item.Bounds.X >= item.Clip.X &&
				item.Bounds.X+item.Bounds.W <= item.Clip.X+item.Clip.W+.01 && item.Bounds.Y >= item.Clip.Y &&
				item.Bounds.Y+item.Bounds.H <= item.Clip.Y+item.Clip.H+.01 {
				return item.Bounds, item.Text, true
			}
		}
		return mathx.Rect{}, "", false
	}
	deadline := time.Now().Add(12 * time.Second)
	var left mathx.Rect
	for time.Now().Before(deadline) {
		interopStep(t, s)
		var ok bool
		left, _, ok = find("L")
		if ok {
			break
		}
	}
	if left.W <= 0 {
		t.Fatal("stereo waveform did not appear")
	}
	// Drag in the waveform, away from its edge loop handles.
	y := left.Center().Y
	for _, event := range []struct {
		x    float32
		kind int
	}{{left.X + 200, 0}, {left.X + 200, 1}, {left.X + 500, 0}, {left.X + 500, 2}} {
		if !sdl.TestPointerEvent(event.x, y, event.kind) {
			t.Fatal("cannot inject waveform selection")
		}
		interopStep(t, s)
	}
	button, _, ok := find("Analyze source selection")
	if !ok {
		t.Fatal("selected waveform did not show source analysis")
	}
	interopClick(t, s, button.Center().X, button.Center().Y)
	deadline = time.Now().Add(8 * time.Second)
	var first, second string
	for time.Now().Before(deadline) {
		interopStep(t, s)
		_, first, _ = find("Channel 1:")
		_, second, _ = find("Channel 2:")
		if first != "" && second != "" {
			break
		}
	}
	if !strings.Contains(first, "peak 0.25") || !strings.Contains(first, "RMS 0.25") ||
		!strings.Contains(second, "peak 0.5") || !strings.Contains(second, "RMS 0.5") {
		t.Fatalf("unexpected source analysis labels: %q %q", first, second)
	}
	if doc.Dirty() || doc.CanUndo() {
		t.Fatal("analysis changed the document history")
	}
	if got, err := s.Pipeline.SourceBytes(name); err != nil || !bytes.Equal(got, source) {
		t.Fatal("analysis changed the source", err)
	}
	if got, err := s.Pipeline.SourceBytes(name + ".meta"); err != nil || !bytes.Equal(got, metadata) {
		t.Fatal("analysis changed metadata", err)
	}
	if got, err := s.Pipeline.Artifacts().ReadFile(art.Path); err != nil || !bytes.Equal(got, artifact) {
		t.Fatal("analysis changed the artifact", err)
	}
}
