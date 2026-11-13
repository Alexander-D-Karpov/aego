package editor

import (
	"bytes"
	"os"
	"testing"

	"aego/asset"
	"aego/core/mathx"
	"aego/modules/audio"
	"aego/platform/input"
	"aego/ui/core"
)

// A reload can replace the source for the same AssetID between two frames.
// An unfinished loop drag must not commit its old coordinates to the new audio.
func TestAudioDocumentDiscardsSourceRelativeViewOnReload(t *testing.T) {
	for _, documentView := range []bool{false, true} {
		name := "inspector"
		if documentView {
			name = "document"
		}
		t.Run(name, func(t *testing.T) {
			f := newWorkingAudioFixture(t, ".wav", false)
			old, err := f.module.Clip(f.ed.Loader, f.id)
			if err != nil {
				t.Fatal(err)
			}
			state := &input.State{}
			area := mathx.Rect{W: 440, H: 640}
			draw := func() {
				ui := f.ed.UI
				ui.Begin(state, mathx.Vec2{X: 440, Y: 640}, 1)
				ui.Region(area)
				f.plugin.clipInspectorView(f.ed, f.id, documentView, area)
				ui.PopCursor()
				ui.End()
			}
			state.BeginFrame()
			draw()
			f.plugin.clip.waveStart, f.plugin.clip.waveEnd = 1000, 40000
			f.plugin.clip.selectAnchor, f.plugin.clip.selectEnd = 2000, 30000
			state.BeginFrame()
			draw()
			if f.plugin.clip.waveStart != 1000 || f.plugin.clip.selectEnd != 30000 {
				t.Fatal("unchanged source unexpectedly lost view state")
			}
			meta, err := asset.ParseSourceMeta(f.doc.Metadata.Code.Bytes(), f.doc.Metadata.Path)
			if err != nil {
				t.Fatal(err)
			}
			f.plugin.clip.drag = 0
			f.plugin.clip.start, f.plugin.clip.end = 12000, 24000
			f.plugin.clip.loopSettings = meta.Settings.Clone()
			journal, revision := bytes.Clone(f.doc.Metadata.Code.Bytes()), f.doc.Metadata.Code.Rev()

			// Same size/layout and ID, different actual PCM. This is a temporary
			// project only; the user's source is never opened or rewritten.
			replacement := bytes.Clone(f.source)
			replacement[44] ^= 1
			if err := os.WriteFile(f.file, replacement, 0o644); err != nil {
				t.Fatal(err)
			}
			batch, err := f.ed.Pipeline.Import(f.id)
			if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("reload import: %+v %v", batch, err)
			}
			f.ed.ApplyBatch(batch)
			current, err := f.module.Clip(f.ed.Loader, f.id)
			if err != nil || current == old || bytes.Equal(current.Art.Data, old.Art.Data) {
				t.Fatal("fixture did not swap the same-ID source", err)
			}
			artifact, ok := f.ed.Pipeline.PrimaryArtifact(f.id)
			if !ok {
				t.Fatal("reimported artifact is missing")
			}
			artifactBytes, err := f.ed.Pipeline.Artifacts().ReadFile(artifact.Path)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := asset.DecodeAudio(artifactBytes)
			if err != nil || !bytes.Equal(decoded.Data, replacement) {
				t.Fatal("reimported artifact does not contain the new source", err)
			}
			state.BeginFrame()
			draw()
			if f.doc.Metadata.Code.Rev() != revision || !bytes.Equal(f.doc.Metadata.Code.Bytes(), journal) || !f.doc.Dirty() || !f.doc.CanUndo() {
				t.Fatal("stale loop drag changed the shared working history")
			}
			if f.plugin.clip.drag >= 0 || f.plugin.clip.loopSettings != nil || f.plugin.clip.waveStart != 0 ||
				f.plugin.clip.waveEnd != 0 || f.plugin.clip.selectAnchor != 0 || f.plugin.clip.selectEnd != 0 {
				t.Fatal("source reload retained an old waveform selection, viewport or loop drag")
			}
			if got, err := os.ReadFile(f.file); err != nil || !bytes.Equal(got, replacement) {
				t.Fatal("drawing the reloaded view changed the source", err)
			}
			if got, err := os.ReadFile(f.file + asset.MetaSuffix); err != nil || !bytes.Equal(got, f.meta) {
				t.Fatal("drawing the reloaded view changed metadata on disk", err)
			}
			currentArtifact, ok := f.ed.Pipeline.PrimaryArtifact(f.id)
			if got, err := f.ed.Pipeline.Artifacts().ReadFile(artifact.Path); !ok || currentArtifact != artifact || err != nil || !bytes.Equal(got, artifactBytes) {
				t.Fatal("drawing the reloaded view changed the new artifact", err)
			}
		})
	}
}

func TestAudioDocumentKeepsViewWhenOnlyClipCacheIsRecreated(t *testing.T) {
	f := newWorkingAudioFixture(t, ".wav", false)
	state := &input.State{}
	area := mathx.Rect{W: 440, H: 640}
	draw := func() {
		ui := f.ed.UI
		ui.Begin(state, mathx.Vec2{X: 440, Y: 640}, 1)
		ui.Region(area)
		f.plugin.clipInspectorView(f.ed, f.id, true, area)
		ui.PopCursor()
		ui.End()
	}
	state.BeginFrame()
	draw()
	old, err := f.module.Clip(f.ed.Loader, f.id)
	if err != nil || len(old.Art.Data) == 0 {
		t.Fatal("could not load the audio source", err)
	}
	f.plugin.clip.waveStart, f.plugin.clip.waveEnd = 1000, 40000
	f.plugin.clip.selectAnchor, f.plugin.clip.selectEnd = 2000, 30000
	journal, revision := bytes.Clone(f.doc.Metadata.Code.Bytes()), f.doc.Metadata.Code.Rev()
	// Replacing the decoder cache need not replace the loaded audio blob. This
	// also happens when an unrelated asset is published by the loader.
	if err := f.ed.Loader.Mount(f.ed.Pipeline.Index(), f.ed.Pipeline.Artifacts()); err != nil {
		t.Fatal(err)
	}
	current, err := f.module.Clip(f.ed.Loader, f.id)
	if err != nil || current == old || len(current.Art.Data) != len(old.Art.Data) || &current.Art.Data[0] != &old.Art.Data[0] {
		t.Fatal("fixture did not recreate only the decoded clip", err)
	}
	state.BeginFrame()
	draw()
	if f.plugin.clip.waveStart != 1000 || f.plugin.clip.waveEnd != 40000 ||
		f.plugin.clip.selectAnchor != 2000 || f.plugin.clip.selectEnd != 30000 {
		t.Fatal("recreating the clip cache discarded an unchanged waveform view")
	}
	if f.doc.Metadata.Code.Rev() != revision || !bytes.Equal(f.doc.Metadata.Code.Bytes(), journal) || !f.doc.Dirty() || !f.doc.CanUndo() {
		t.Fatal("cache recreation changed the shared working history")
	}
	f.checkFiles(t, f.source, f.meta)
}

func TestAudioSourceReloadCancelsCapturedWaveGesture(t *testing.T) {
	for _, gesture := range []string{"loop-start", "loop-end", "selection", "navigator"} {
		t.Run(gesture, func(t *testing.T) {
			state := &input.State{}
			ui := core.New(nil)
			clip := clipEdit{drag: -1}
			first := &audio.Clip{Art: &asset.AudioArtifact{Data: []byte{1}, Rate: 48000, Channels: 1, Frames: 48000}}
			second := &audio.Clip{Art: &asset.AudioArtifact{Data: []byte{2}, Rate: 48000, Channels: 1, Frames: 48000}}
			clip.bindWaveSource(nil, first)
			at := mathx.Vec2{X: 10, Y: 10}
			state.BeginFrame()
			state.ApplyMouseDown(at, input.MouseLeft)
			ui.Begin(state, mathx.Vec2{X: 100, Y: 100}, 1)
			ui.PushID("clip")
			id := ui.ID("wave-selection")
			switch gesture {
			case "loop-start":
				id, clip.drag = core.IDSeq(ui.ID("loop"), 0), 0
			case "loop-end":
				id, clip.drag = core.IDSeq(ui.ID("loop"), 1), 1
			case "selection":
				clip.selecting = true
			case "navigator":
				id, clip.navigating = ui.ID("wave-navigator"), true
			}
			ui.CaptureMouse(id)
			clip.bindWaveSource(nil, second)
			clip.cancelStaleWaveCapture(ui)
			if ui.Active() != 0 || ui.Captured() != 0 || !clip.waveBlocked {
				t.Fatal("reloaded source retained its old mouse capture")
			}
			if _, active := clip.waveDrag(ui, id, mathx.Rect{W: 100, H: 100}); active || ui.Active() != 0 {
				t.Fatal("reloaded source started a gesture from the old press")
			}
			ui.PopID()
			ui.End()
			state.BeginFrame()
			state.ApplyMouseUp(at, input.MouseLeft)
			ui.Begin(state, mathx.Vec2{X: 100, Y: 100}, 1)
			ui.PushID("clip")
			clip.cancelStaleWaveCapture(ui)
			if clip.waveBlocked || ui.Active() != 0 {
				t.Fatal("releasing the old gesture did not re-enable the waveform")
			}
			ui.PopID()
			ui.End()
			state.BeginFrame()
			state.ApplyMouseDown(at, input.MouseLeft)
			ui.Begin(state, mathx.Vec2{X: 100, Y: 100}, 1)
			ui.PushID("clip")
			if _, active := clip.waveDrag(ui, id, mathx.Rect{W: 100, H: 100}); !active {
				t.Fatal("new press could not start a waveform gesture")
			}
			ui.PopID()
			ui.End()
		})
	}
}

// Exercise the real Inspector draw path while a loop handle still owns the
// mouse. Releasing that old press must not start or commit a new drag.
func TestAudioReloadWhileLoopHandleIsCaptured(t *testing.T) {
	f := newWorkingAudioFixture(t, ".wav", false)
	state, ui := &input.State{}, f.ed.UI
	area := mathx.Rect{W: 440, H: 640}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: 440, Y: 640}, 1)
		ui.Region(area)
		f.plugin.clipInspectorView(f.ed, f.id, true, area)
		ui.PopCursor()
		ui.End()
	}
	state.BeginFrame()
	draw()
	meta, err := asset.ParseSourceMeta(f.doc.Metadata.Code.Bytes(), f.doc.Metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	f.plugin.clip.drag, f.plugin.clip.start, f.plugin.clip.end = 0, 12000, 24000
	f.plugin.clip.loopSettings = meta.Settings.Clone()
	journal, revision := bytes.Clone(f.doc.Metadata.Code.Bytes()), f.doc.Metadata.Code.Rev()
	at := mathx.Vec2{X: 20, Y: 20}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	ui.Begin(state, mathx.Vec2{X: 440, Y: 640}, 1)
	ui.PushID("clip/" + f.id.String())
	ui.CaptureMouse(core.IDSeq(ui.ID("loop"), 0))
	ui.PopID()
	ui.End()
	replacement := bytes.Clone(f.source)
	replacement[44] ^= 1
	if err := os.WriteFile(f.file, replacement, 0o644); err != nil {
		t.Fatal(err)
	}
	batch, err := f.ed.Pipeline.Import(f.id)
	if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("reload import: %+v %v", batch, err)
	}
	f.ed.ApplyBatch(batch)
	state.BeginFrame()
	state.ApplyMouseMove(at.Add(mathx.Vec2{X: 5}), mathx.Vec2{X: 5})
	draw()
	if ui.Active() != 0 || ui.Captured() != 0 || f.plugin.clip.drag >= 0 {
		t.Fatal("old loop handle retained the mouse after source reload")
	}
	state.BeginFrame()
	state.ApplyMouseUp(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	draw()
	if f.doc.Metadata.Code.Rev() != revision || !bytes.Equal(f.doc.Metadata.Code.Bytes(), journal) || !f.doc.Dirty() || !f.doc.CanUndo() {
		t.Fatal("releasing the old loop handle changed the shared history")
	}
	if got, err := os.ReadFile(f.file); err != nil || !bytes.Equal(got, replacement) {
		t.Fatal("reloaded source changed during the gesture", err)
	}
	if got, err := os.ReadFile(f.file + asset.MetaSuffix); err != nil || !bytes.Equal(got, f.meta) {
		t.Fatal("reloaded gesture changed metadata on disk", err)
	}
}

func TestAudioSourceReloadDoesNotReleaseOtherControls(t *testing.T) {
	state := &input.State{}
	state.BeginFrame()
	state.ApplyMouseDown(mathx.Vec2{X: 10, Y: 10}, input.MouseLeft)
	ui := core.New(nil)
	ui.Begin(state, mathx.Vec2{X: 100, Y: 100}, 1)
	ui.PushID("clip")
	other := ui.ID("gain")
	ui.CaptureMouse(other)
	clip := clipEdit{waveBlocked: true, drag: -1}
	clip.cancelStaleWaveCapture(ui)
	if ui.Active() != other || ui.Captured() != other {
		t.Fatal("stale waveform cleanup released another control")
	}
	ui.PopID()
	ui.End()
}
