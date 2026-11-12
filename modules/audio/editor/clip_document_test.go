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
	"aego/project"
	"aego/render/texture"
)

type testAudioPeaks struct {
	stereo           []float32
	stereoDone       bool
	monoCalls, calls int
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
			v := ClipView{ID: ids.NewAssetID(), Channels: tc.channels}
			(&plugin{}).clipPeaks(ed, &v, tc.document)
			if (peaks.calls > 0) != tc.wantStereo || (peaks.monoCalls > 0) != tc.wantMono || v.PeaksDone != tc.wantDone {
				t.Fatalf("wrong waveform route: stereo=%d mono=%d done=%v", peaks.calls, peaks.monoCalls, v.PeaksDone)
			}
			if len(tc.stereo) > 0 && tc.wantStereo && !slices.Equal(v.StereoPeaks, tc.stereo) {
				t.Fatal("stereo amplitudes were not forwarded")
			}
		})
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
	if err != nil || len(batch.Failed) != 0 {
		t.Fatal("audio fixture import", err, batch.Failed)
	}
	ed := &context.Editor{Pipeline: pl, Docs: document.NewManager(document.Options{Root: pj.Root})}
	if err := ed.Docs.Handlers().Register(document.Handler{ID: "audio.clip", Kinds: []asset.ArtifactKind{asset.ArtAudio}, Capabilities: document.View | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	p := &plugin{ed: ed, clip: clipEdit{id: meta.ID}}
	after := meta.Settings.Clone()
	after["gain"] = aetext.Float32(2)
	p.write(ed, meta.Settings, after)
	d := ed.Docs.AssetDocument(meta.ID)
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
