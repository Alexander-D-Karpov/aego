package editor

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"aego/asset"
	audioimport "aego/asset/importers/audio"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/format/aetext"
	"aego/platform/input"
	"aego/project"
	"aego/render/text"
	"aego/render/texture"
	"aego/ui/core"
)

type testAudioPeaks struct {
	stereo           []float32
	stereoDone       bool
	monoCalls, calls int
	start, end       int64
}

func (*testAudioPeaks) ByID(ids.AssetID) (texture.Region, bool) { return texture.Region{}, false }
func (p *testAudioPeaks) Peaks(ids.AssetID, int) ([]float32, bool) {
	p.monoCalls++
	return []float32{.3}, true
}
func (p *testAudioPeaks) StereoPeaks(ids.AssetID, int) ([]float32, bool) {
	p.calls++
	return p.stereo, p.stereoDone
}
func (p *testAudioPeaks) StereoPeaksWindow(_ ids.AssetID, start, end int64, _ int) ([]float32, bool) {
	p.calls++
	p.start, p.end = start, end
	return p.stereo, p.stereoDone
}

func TestClipPeaksSelectsStereoOnlyForTheDocument(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		document             bool
		channels             int
		stereo               []float32
		stereoDone           bool
		wantStereo, wantMono bool
		wantDone             bool
	}{
		{"stereo document", true, 2, []float32{.8, .2}, true, true, false, true},
		{"pending document", true, 2, nil, false, true, false, false},
		{"unsupported fallback", true, 2, nil, true, true, true, true},
		{"stereo inspector", false, 2, []float32{.8, .2}, true, false, true, true},
		{"mono document", true, 1, []float32{.8, .2}, true, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peaks := &testAudioPeaks{stereo: tc.stereo, stereoDone: tc.stereoDone}
			ed := &context.Editor{Thumbs: peaks}
			v := ClipView{ID: ids.NewAssetID(), Channels: tc.channels, Frames: 48000}
			(&plugin{}).clipPeaks(ed, &v, tc.document)
			if (peaks.calls > 0) != tc.wantStereo || (peaks.monoCalls > 0) != tc.wantMono || v.PeaksDone != tc.wantDone {
				t.Fatalf("wrong waveform route: stereo=%d mono=%d done=%v", peaks.calls, peaks.monoCalls, v.PeaksDone)
			}
			if len(tc.stereo) > 0 && tc.wantStereo && !slices.Equal(v.StereoPeaks, tc.stereo) {
				t.Fatal("stereo amplitudes were not forwarded")
			}
			if tc.document && tc.channels == 2 && len(tc.stereo) > 0 && (!v.WaveNav || peaks.start != 0 || peaks.end != v.Frames) {
				t.Fatal("stereo document did not request its full viewport")
			}
		})
	}
}

func TestMonoDocumentWaveSelectionDoesNotModifyMetadata(t *testing.T) {
	ui := core.New(nil)
	state := &input.State{}
	p := &plugin{clip: clipEdit{drag: -1}}
	ed := &context.Editor{UI: ui, Thumbs: &testAudioPeaks{}}
	v := ClipView{ID: ids.NewAssetID(), Channels: 1, Frames: 48000, Rate: 48000}
	p.clipPeaks(ed, &v, true)
	if len(v.Peaks) == 0 || v.WaveNav {
		t.Fatal("fixture needs a mono waveform without stereo navigation")
	}
	inspector := ClipView{ID: v.ID, Channels: v.Channels, Frames: v.Frames}
	p.clipPeaks(ed, &inspector, false)
	if inspector.WaveSelect || inspector.WaveNav {
		t.Fatal("compact Inspector must not acquire document-selection controls")
	}
	width := float32(320) - 2*ui.Theme.Padding
	at := mathx.Vec2{X: ui.Theme.Padding + width/4, Y: ui.Theme.Padding + waveH/2}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
		ui.Region(mathx.Rect{W: 320, H: 180})
		p.wave(ed, v)
		ui.PopCursor()
		ui.End()
	}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	delta := mathx.Vec2{X: width / 2}
	state.ApplyMouseMove(at.Add(delta), delta)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(at.Add(delta), input.MouseLeft)
	draw()
	if start, end, ok := p.clip.waveSelection(v.Frames); !ok || start != 12000 || end != 36000 {
		t.Fatalf("mono waveform selection = %d..%d, valid=%v", start, end, ok)
	}
	if ed.Docs != nil {
		t.Fatal("view-only selection created or modified document history")
	}
}

func TestAudioWaveNavigationClampsAndReachesLateAudio(t *testing.T) {
	const total int64 = 5313600
	start, end := int64(0), total
	for range 5 {
		start, end = adjustAudioWave(start, end, total, waveZoomIn)
	}
	if end-start >= total || end-start < 512 {
		t.Fatalf("invalid zoom span %d..%d", start, end)
	}
	reached100 := false
	for range 100 {
		start, end = adjustAudioWave(start, end, total, wavePanRight)
		reached100 = reached100 || start <= 100*48000 && end >= 100*48000
	}
	if end != total || !reached100 {
		t.Fatalf("could not reach 100 seconds: %d..%d", start, end)
	}
	for range 100 {
		start, end = adjustAudioWave(start, end, total, wavePanLeft)
	}
	if start != 0 {
		t.Fatalf("left pan escaped its bound: %d", start)
	}
	for range 100 {
		start, end = adjustAudioWave(start, end, total, waveZoomOut)
	}
	if start != 0 || end != total {
		t.Fatalf("zoom out did not restore full range: %d..%d", start, end)
	}
	start, end = adjustAudioWave(0, math.MaxInt64, math.MaxInt64, waveZoomOut)
	if start != 0 || end != math.MaxInt64 {
		t.Fatal("large frame count overflowed")
	}
}

func TestAudioWaveZoomButtonUpdatesOnlyViewState(t *testing.T) {
	ui := core.New(nil)
	font := text.NewFont(14, 1, 16, 12)
	for r := rune(32); r < 127; r++ {
		font.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	ui.SetFont(font)
	state := &input.State{}
	p := &plugin{}
	ed := &context.Editor{UI: ui}
	v := ClipView{Frames: 48000, Rate: 48000, WaveStart: 0, WaveEnd: 48000, WaveNav: true}
	at := mathx.Vec2{X: ui.Theme.Padding + 2*(28+4) + 14, Y: ui.Theme.Padding + ui.Theme.RowHeight/2}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
		ui.Region(mathx.Rect{W: 320, H: 180})
		p.waveNavigation(ed, v)
		ui.PopCursor()
		ui.End()
	}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(at, input.MouseLeft)
	draw()
	if p.clip.waveStart != 12000 || p.clip.waveEnd != 36000 {
		t.Fatalf("zoom button produced %d..%d", p.clip.waveStart, p.clip.waveEnd)
	}
	if ed.Docs != nil {
		t.Fatal("navigation created or modified a document")
	}
}

func TestZoomedWaveLoopDragUsesVisibleFrameRange(t *testing.T) {
	ui := core.New(nil)
	state := &input.State{}
	p := &plugin{clip: clipEdit{drag: -1}}
	ed := &context.Editor{UI: ui}
	const rate int64 = 48000
	settings := pipeline.Settings{"loopStart": aetext.Int(100 * rate), "loopEnd": aetext.Int(102 * rate)}
	v := ClipView{Frames: 5313600, Rate: int(rate), WaveStart: 99 * rate, WaveEnd: 103 * rate,
		Settings: settings, StereoPeaks: []float32{.5, .25}, WaveNav: true}
	width := float32(320) - 2*ui.Theme.Padding
	at := mathx.Vec2{X: ui.Theme.Padding + width/4, Y: ui.Theme.Padding + waveH/2}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
		ui.Region(mathx.Rect{W: 320, H: 180})
		p.wave(ed, v)
		ui.PopCursor()
		ui.End()
	}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	delta := mathx.Vec2{X: width / 10}
	state.ApplyMouseMove(at.Add(delta), delta)
	draw()
	want := int64(100*rate) + int64(float64(delta.X/width)*float64(v.WaveEnd-v.WaveStart))
	if p.clip.drag != 0 || p.clip.start < want-2 || p.clip.start > want+2 || p.clip.end != 102*rate {
		t.Fatalf("zoomed drag = %d..%d, want start %d", p.clip.start, p.clip.end, want)
	}
	if _, _, ok := p.clip.waveSelection(v.Frames); ok || p.clip.selecting {
		t.Fatal("loop-handle drag also started a selection")
	}
	if settings.Int("loopStart", -1) != int(100*rate) || ed.Docs != nil {
		t.Fatal("an unfinished drag changed shared metadata")
	}
}

func TestZoomedLoopDragCanReturnFromOutsideViewport(t *testing.T) {
	ui := core.New(nil)
	state := &input.State{}
	p := &plugin{clip: clipEdit{drag: -1}}
	ed := &context.Editor{UI: ui}
	const rate int64 = 48000
	v := ClipView{Frames: 5313600, Rate: int(rate), WaveStart: 99 * rate, WaveEnd: 103 * rate,
		Settings:    pipeline.Settings{"loopStart": aetext.Int(100 * rate), "loopEnd": aetext.Int(102 * rate)},
		StereoPeaks: []float32{.5, .25}, WaveNav: true}
	width := float32(320) - 2*ui.Theme.Padding
	at := mathx.Vec2{X: ui.Theme.Padding + width/4, Y: ui.Theme.Padding + waveH/2}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
		ui.Region(mathx.Rect{W: 320, H: 180})
		p.wave(ed, v)
		ui.PopCursor()
		ui.End()
	}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	outside := mathx.Vec2{X: -width * 3 / 8}
	state.ApplyMouseMove(at.Add(outside), outside)
	draw()
	if p.clip.start >= v.WaveStart {
		t.Fatalf("loop handle did not move outside viewport: %d", p.clip.start)
	}
	state.BeginFrame()
	back := mathx.Vec2{X: width / 4}
	state.ApplyMouseMove(at.Add(outside).Add(back), back)
	draw()
	want := int64(99*rate) + rate/2
	if p.clip.start < want-2 || p.clip.start > want+2 {
		t.Fatalf("loop handle did not track cursor back into viewport: got %d, want %d", p.clip.start, want)
	}
}

func TestAudioWaveSelectionIsPerTabAndDoesNotEditMetadata(t *testing.T) {
	ui := core.New(nil)
	state := &input.State{}
	p := &plugin{clip: clipEdit{drag: -1}}
	other := &plugin{clip: clipEdit{drag: -1}}
	ed := &context.Editor{UI: ui}
	const rate int64 = 48000
	settings := pipeline.Settings{"loopStart": aetext.Int(100 * rate), "loopEnd": aetext.Int(102 * rate)}
	v := ClipView{Frames: 5313600, Rate: int(rate), WaveStart: 99 * rate, WaveEnd: 103 * rate,
		Settings: settings, StereoPeaks: []float32{.5, .25}, WaveNav: true}
	width := float32(320) - 2*ui.Theme.Padding
	at := mathx.Vec2{X: ui.Theme.Padding + width/8, Y: ui.Theme.Padding + waveH/2}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
		ui.Region(mathx.Rect{W: 320, H: 180})
		p.wave(ed, v)
		ui.PopCursor()
		ui.End()
	}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	delta := mathx.Vec2{X: width / 2}
	state.ApplyMouseMove(at.Add(delta), delta) // Cross the loop-start handle while captured.
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(at.Add(delta), input.MouseLeft)
	draw()
	start, end, ok := p.clip.waveSelection(v.Frames)
	if !ok || start != 99*rate+rate/2 || end != 101*rate+rate/2 || p.clip.selecting {
		t.Fatalf("selection = %d..%d, active=%v", start, end, p.clip.selecting)
	}
	if _, _, ok := other.clip.waveSelection(v.Frames); ok {
		t.Fatal("selection leaked into another document view")
	}
	if settings.Int("loopStart", -1) != int(100*rate) || settings.Int("loopEnd", -1) != int(102*rate) || ed.Docs != nil {
		t.Fatal("view-only selection changed metadata or document history")
	}
	// A viewport change must not convert the selection to view-relative frames.
	v.WaveStart, v.WaveEnd = 100*rate, 102*rate
	state.BeginFrame()
	draw()
	if a, b, valid := p.clip.waveSelection(v.Frames); !valid || a != start || b != end {
		t.Fatalf("selection changed when zooming: %d..%d", a, b)
	}
	// Reverse-drag past the left edge clamps to the viewport without disturbing
	// the loop handles; a later click without dragging clears that view state.
	v.WaveStart, v.WaveEnd = 99*rate, 103*rate
	at = mathx.Vec2{X: ui.Theme.Padding + width*7/8, Y: ui.Theme.Padding + waveH/2}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	delta = mathx.Vec2{X: -width}
	state.ApplyMouseMove(at.Add(delta), delta)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(at.Add(delta), input.MouseLeft)
	draw()
	if a, b, valid := p.clip.waveSelection(v.Frames); !valid || a != 99*rate || b != 102*rate+rate/2 || p.clip.selecting {
		t.Fatalf("reverse selection = %d..%d, active=%v", a, b, p.clip.selecting)
	}
	at = mathx.Vec2{X: ui.Theme.Padding + width/2, Y: ui.Theme.Padding + waveH/2}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(at, input.MouseLeft)
	draw()
	if _, _, valid := p.clip.waveSelection(v.Frames); valid {
		t.Fatal("click did not clear the previous selection")
	}
	if settings.Int("loopStart", -1) != int(100*rate) || settings.Int("loopEnd", -1) != int(102*rate) {
		t.Fatal("selection gesture changed loop metadata")
	}
}

func TestAudioWaveFrameClampsAndRejectsInvalidGeometry(t *testing.T) {
	r := mathx.Rect{X: 10, W: 200}
	for _, tc := range []struct {
		x    float32
		want int64
	}{{-1, 100}, {10, 100}, {60, 200}, {110, 300}, {210, 500}, {300, 500}} {
		got, ok := audioWaveFrame(tc.x, r, 100, 500)
		if !ok || got != tc.want {
			t.Errorf("x=%v: got %d,%v want %d", tc.x, got, ok, tc.want)
		}
	}
	for _, tc := range []struct {
		x          float32
		r          mathx.Rect
		start, end int64
	}{
		{0, r, -1, 10}, {0, r, 0, 0}, {0, r, 0, 1<<53 + 1},
		{0, mathx.Rect{}, 0, 10}, {float32(math.NaN()), r, 0, 10},
		{0, mathx.Rect{X: math.MaxFloat32, W: math.MaxFloat32}, 0, 10},
	} {
		if _, ok := audioWaveFrame(tc.x, tc.r, tc.start, tc.end); ok {
			t.Errorf("accepted invalid pointer geometry %+v", tc)
		}
	}
}

func TestAudioWaveSelectionRectClipsToVisibleRange(t *testing.T) {
	r := mathx.Rect{X: 10, Y: 20, W: 200, H: 72}
	for _, tc := range []struct {
		a, b int64
		want mathx.Rect
		ok   bool
	}{
		{0, 150, mathx.Rect{X: 10, Y: 20, W: 25, H: 72}, true},
		{200, 400, mathx.Rect{X: 60, Y: 20, W: 100, H: 72}, true},
		{400, 600, mathx.Rect{X: 160, Y: 20, W: 50, H: 72}, true},
		{0, 100, mathx.Rect{}, false}, {500, 600, mathx.Rect{}, false},
	} {
		got, ok := audioWaveSelectionRect(r, 100, 500, tc.a, tc.b)
		if ok != tc.ok || got != tc.want {
			t.Errorf("selection %d..%d: got %+v,%v want %+v,%v", tc.a, tc.b, got, ok, tc.want, tc.ok)
		}
	}
}

func TestAudioMetadataUsesSharedJournalWithoutEditingBinarySource(t *testing.T) {
	pj, err := project.Create(t.TempDir(), "Audio", false)
	if err != nil {
		t.Fatal(err)
	}
	// A short PCM fixture exercises the existing importer, no audio device.
	wav := make([]byte, 44+32)
	copy(wav[0:], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 16000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 32)
	const path = "assets/click.wav"
	if err := os.WriteFile(filepath.Join(pj.Root, path), wav, 0644); err != nil {
		t.Fatal(err)
	}
	imp := audioimport.New()
	meta := asset.NewSourceMeta("audio", imp.Version(), imp.Defaults())
	meta.Settings["loopStart"], meta.Settings["loopEnd"] = aetext.Int(5), aetext.Int(11)
	meta.Settings["extension_note"] = aetext.String("keep")
	encoded := asset.MarshalSourceMeta(meta)
	if err := os.WriteFile(filepath.Join(pj.Root, path+".meta"), encoded, 0644); err != nil {
		t.Fatal(err)
	}
	registry := pipeline.NewImporterRegistry()
	if err := registry.Register(imp); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(pj, registry, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("audio fixture import: batch=%+v error=%v", batch, err)
	}
	ed := &context.Editor{Pipeline: pl, Docs: document.NewManager(document.Options{Root: pj.Root})}
	if err := ed.Docs.Handlers().Register(document.Handler{ID: "audio.clip", Kinds: []asset.ArtifactKind{asset.ArtAudio}, Capabilities: document.View | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	p := &plugin{ed: ed, clip: clipEdit{id: meta.ID, drag: -1}}
	// Drive the real handle out of a zoomed viewport and back, then release.
	// This must create one shared metadata edit, not touch disk or source PCM.
	ui := core.New(nil)
	ed.UI = ui
	state := &input.State{}
	wave := ClipView{ID: meta.ID, Frames: 16, Rate: 8000, WaveStart: 4, WaveEnd: 12,
		Settings: meta.Settings.Clone(), StereoPeaks: []float32{.5, .25}, WaveNav: true}
	width := float32(320) - 2*ui.Theme.Padding
	drawWave := func() {
		ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
		ui.Region(mathx.Rect{W: 320, H: 180})
		p.wave(ed, wave)
		ui.PopCursor()
		ui.End()
	}
	d, err := ed.DocumentForAsset(meta.ID)
	if err != nil || d.Metadata == nil || d.Dirty() {
		t.Fatal("cannot open clean audio metadata", err)
	}
	selectAt := mathx.Vec2{X: ui.Theme.Padding + width/4, Y: ui.Theme.Padding + waveH/2}
	state.BeginFrame()
	state.ApplyMouseDown(selectAt, input.MouseLeft)
	drawWave()
	state.BeginFrame()
	selectDelta := mathx.Vec2{X: width / 2}
	state.ApplyMouseMove(selectAt.Add(selectDelta), selectDelta)
	drawWave()
	state.BeginFrame()
	state.ApplyMouseUp(selectAt.Add(selectDelta), input.MouseLeft)
	drawWave()
	if a, b, ok := p.clip.waveSelection(wave.Frames); !ok || a != 6 || b != 10 || d.Dirty() || d.CanUndo() {
		t.Fatalf("view selection changed the shared document: %d..%d dirty=%v", a, b, d.Dirty())
	}
	if raw, err := os.ReadFile(ed.Docs.Abs(path + ".meta")); err != nil || !bytes.Equal(raw, encoded) {
		t.Fatal("view selection wrote metadata to disk", err)
	}
	if raw, err := os.ReadFile(ed.Docs.Abs(path)); err != nil || !bytes.Equal(raw, wav) {
		t.Fatal("view selection changed encoded audio", err)
	}
	at := mathx.Vec2{X: ui.Theme.Padding + width/8, Y: ui.Theme.Padding + waveH/2}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	drawWave()
	state.BeginFrame()
	outside := mathx.Vec2{X: -width / 4}
	state.ApplyMouseMove(at.Add(outside), outside)
	drawWave()
	state.BeginFrame()
	back := mathx.Vec2{X: width * 3 / 8}
	state.ApplyMouseMove(at.Add(outside).Add(back), back)
	drawWave()
	state.BeginFrame()
	state.ApplyMouseUp(at.Add(outside).Add(back), input.MouseLeft)
	drawWave()
	state.BeginFrame()
	drawWave() // Finish the released gesture on the next frame.
	d = ed.Docs.AssetDocument(meta.ID)
	if d == nil || d.Metadata == nil || !d.Dirty() {
		t.Fatal("zoomed loop drag did not create a shared metadata edit")
	}
	dragged, err := asset.ParseSourceMeta(d.Metadata.Code.Bytes(), d.Metadata.Path)
	if err != nil || dragged.Settings.Int("loopStart", -1) != 6 || dragged.Settings.Int("loopEnd", -1) != 11 {
		t.Fatalf("zoomed loop drag produced unexpected metadata: %+v, %v", dragged, err)
	}
	if raw, err := os.ReadFile(ed.Docs.Abs(path + ".meta")); err != nil || !bytes.Equal(raw, encoded) {
		t.Fatal("zoomed loop drag wrote metadata to disk", err)
	}
	if err := ed.UndoAsset(d); err != nil || d.Dirty() {
		t.Fatal("zoomed loop drag did not undo as one shared step", err)
	}
	if a, b, ok := p.clip.waveSelection(wave.Frames); !ok || a != 6 || b != 10 {
		t.Fatal("loop drag or undo changed the independent view selection")
	}
	after := meta.Settings.Clone()
	after["gain"] = aetext.Float32(2)
	p.write(ed, meta.Settings, after)
	d = ed.Docs.AssetDocument(meta.ID)
	if d == nil || d.Metadata == nil || d.Source != nil || !d.Dirty() || !d.Metadata.ExplicitSave {
		t.Fatal("audio metadata has a private history or binary text source")
	}
	if raw, _ := os.ReadFile(ed.Docs.Abs(path + ".meta")); !bytes.Equal(raw, encoded) {
		t.Fatal("audio control silently saved metadata")
	}
	if err := ed.UndoAsset(d); err != nil || d.Dirty() {
		t.Fatal("audio edit is not one shared Undo step", err)
	}
	if err := ed.RedoAsset(d); err != nil {
		t.Fatal(err)
	}
	if err := ed.SaveAsset(d); err != nil {
		t.Fatal(err)
	}
	saved, err := asset.ParseSourceMeta(d.Metadata.Code.Bytes(), d.Metadata.Path)
	if err != nil || saved.Settings.Float("gain", 0) != 2 || saved.Settings.String("extension_note", "") != "keep" {
		t.Fatal("audio save lost settings or unknown semantic data", err)
	}
	if raw, _ := os.ReadFile(ed.Docs.Abs(path)); !bytes.Equal(raw, wav) {
		t.Fatal("metadata save modified the binary audio source")
	}
	// Two visible controls may have captured the same older settings. A change
	// in one field must survive a later edit of a different field; a stale edit
	// of the same field must not overwrite the newer value.
	loop := saved.Settings.Clone()
	loop["loopStart"], loop["loopEnd"] = aetext.Int(1), aetext.Int(8)
	p.write(ed, saved.Settings, loop)
	gain := saved.Settings.Clone()
	gain["gain"] = aetext.Float64(3)
	p.write(ed, saved.Settings, gain)
	combined, err := asset.ParseSourceMeta(d.Metadata.Code.Bytes(), d.Metadata.Path)
	if err != nil || combined.Settings.Int("loopStart", -1) != 1 || combined.Settings.Int("loopEnd", -1) != 8 || combined.Settings.Float("gain", 0) != 3 {
		t.Fatal("independent audio controls overwrote one another", err)
	}
	stale := saved.Settings.Clone()
	stale["gain"] = aetext.Float64(4)
	beforeStale := bytes.Clone(d.Metadata.Code.Bytes())
	p.write(ed, saved.Settings, stale)
	if !bytes.Equal(beforeStale, d.Metadata.Code.Bytes()) {
		t.Fatal("a stale audio control overwrote the newer gain")
	}
	if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
		t.Fatal("the stale audio control was not reported")
	}
	// The waveform must retain the start-of-drag preimage, not silently adopt
	// another view's newer end handle when the mouse is released.
	loopBeforeDrag := combined.Settings.Clone()
	otherLoop := loopBeforeDrag.Clone()
	otherLoop["loopEnd"] = aetext.Int(6)
	p.write(ed, loopBeforeDrag, otherLoop)
	currentLoop, err := asset.ParseSourceMeta(d.Metadata.Code.Bytes(), d.Metadata.Path)
	if err != nil || currentLoop.Settings.Int("loopEnd", -1) != 6 {
		t.Fatal("could not prepare a concurrent loop edit", err)
	}
	loopDisk := bytes.Clone(d.Metadata.Code.Bytes())
	p.clip.drag, p.clip.start, p.clip.end = 0, 4, 8
	p.clip.loopSettings = loopBeforeDrag
	p.commitLoop(ed, ClipView{ID: meta.ID, Frames: 16, Settings: currentLoop.Settings})
	if !bytes.Equal(loopDisk, d.Metadata.Code.Bytes()) {
		t.Fatal("releasing a stale loop drag overwrote the opposite handle")
	}
	d.Metadata.Conflict = true
	before := bytes.Clone(d.Metadata.Code.Bytes())
	after["gain"] = aetext.Float32(3)
	p.write(ed, saved.Settings, after)
	if !bytes.Equal(before, d.Metadata.Code.Bytes()) {
		t.Fatal("audio Inspector ignored an unresolved external conflict")
	}
}

func TestClipDocumentWidthsStayCompactAndFitNarrowPanels(t *testing.T) {
	for _, tc := range []struct{ available, wave, controls float32 }{
		{-1, 0, 0}, {180, 180, 180}, {400, 400, 400},
		{700, 700, 440}, {1600, 900, 440},
	} {
		wave, controls := clipDocumentWidths(tc.available)
		if wave != tc.wave || controls != tc.controls {
			t.Errorf("width %v: got waveform %v and controls %v, want %v and %v", tc.available, wave, controls, tc.wave, tc.controls)
		}
	}
}

func TestStereoWaveRectKeepsChannelsAndColumnsInsideTheWaveform(t *testing.T) {
	for _, width := range []float32{1, 180, 320, 900} {
		r := mathx.Rect{X: 10, Y: 20, W: width, H: 72}
		for ch := range 2 {
			for col := range 256 {
				peak, ok := stereoWaveRect(r, 256, col, ch, 1)
				if !ok || peak.X < r.X || peak.X+peak.W > r.X+r.W+.0001 ||
					peak.Y < r.Y+float32(ch)*r.H/2 || peak.Y+peak.H > r.Y+float32(ch+1)*r.H/2 {
					t.Fatalf("width %v channel %d column %d escaped: %+v", width, ch, col, peak)
				}
			}
		}
	}
	r := mathx.Rect{W: 100, H: 72}
	for _, bad := range []float32{float32(math.NaN()), float32(math.Inf(1))} {
		if _, ok := stereoWaveRect(r, 2, 0, 0, bad); ok {
			t.Fatal("accepted a non-finite waveform amplitude")
		}
	}
	if _, ok := stereoWaveRect(mathx.Rect{}, 2, 0, 0, 1); ok {
		t.Fatal("accepted an empty waveform")
	}
}

func TestAudioLoopMergeRejectsChangedOppositeHandle(t *testing.T) {
	before := pipeline.Settings{"loopStart": aetext.Int(0), "loopEnd": aetext.Int(16), "gain": aetext.Float64(1)}
	current := before.Clone()
	current["loopEnd"] = aetext.Int(8)
	otherView := before.Clone()
	otherView["loopStart"] = aetext.Int(10)
	snapshot := current.Clone()
	if changed, err := mergeClipSettings(current, before, otherView); err == nil || changed {
		t.Fatal("the opposite loop handle changed during this edit", err)
	}
	for key, value := range snapshot {
		got, ok := current[key]
		if !equalClipSetting(got, ok, value, true) {
			t.Fatal("a rejected loop edit changed another setting", key)
		}
	}
}
