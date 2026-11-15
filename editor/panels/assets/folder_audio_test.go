package assets

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"aego/asset"
	audioimport "aego/asset/importers/audio"
	"aego/asset/pipeline"
	"aego/audio/codec/wav"
	"aego/core/mathx"
	"aego/editor/core/preview"
	"aego/editor/core/services"
	"aego/editor/core/tasks"
	"aego/platform/input"
	"aego/project"
	"aego/ui/core"
)

func TestFolderAudioPreviewOnlyUsesReadyWaveform(t *testing.T) {
	proj, err := project.Create(t.TempDir(), "Waveforms", false)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(proj.AssetsDir(), "audio", "tone.wav")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	write := func(value float32) {
		t.Helper()
		pcm := make([]float32, 1024)
		for i := range pcm {
			pcm[i] = value
		}
		if err := os.WriteFile(file, wav.Encode(pcm, 44100, 1, wav.EncodeOptions{Float: true}), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(0.75)
	registry := pipeline.NewImporterRegistry()
	if err := registry.Register(audioimport.New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(proj, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	importAll := func() {
		t.Helper()
		batch, err := pl.ImportAll()
		if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Fatalf("audio import: batch=%+v error=%v", batch, err)
		}
	}
	importAll()
	id, ok := pl.Lookup("assets/audio/tone.wav")
	if !ok {
		t.Fatal("imported sound is missing")
	}
	q := tasks.New(tasks.Deps{Submit: func(fn func()) { fn() }, OnMain: func(fn func()) { fn() }})
	thumbs := preview.New(preview.Deps{Pipeline: pl, Tasks: q, CacheRoot: t.TempDir()})
	t.Cleanup(thumbs.Close)
	q.Tick() // finish old-thumbnail cleanup, unrelated to the audio preview
	if q.Busy() {
		t.Fatal("thumbnail cleanup did not finish")
	}
	ui := core.New(nil)
	in := &input.State{}
	p := New(Deps{Thumbs: thumbs})
	p.f = &services.Frame{UI: ui}
	info := pipeline.AssetInfo{ID: id, Kind: asset.ArtAudio, Name: "tone.wav", Path: "assets/audio/tone.wav"}
	p.items = []pipeline.AssetInfo{info}
	drawFolder := func() {
		t.Helper()
		in.BeginFrame()
		ui.Begin(in, mathx.Vec2{X: 300, Y: 300}, 1)
		p.folderStack("assets/audio", mathx.Rect{W: 120, H: 120})
		ui.End()
	}
	drawFolder()
	if q.Busy() {
		t.Fatalf("a closed folder queued audio decoding: %+v", q.Tasks())
	}
	// The actual audio tile must still request and display the real waveform.
	in.BeginFrame()
	ui.Begin(in, mathx.Vec2{X: 300, Y: 300}, 1)
	if p.wave(&info, mathx.Rect{W: 100, H: 50}) || !q.Busy() {
		t.Fatal("opening the audio tile did not request a waveform")
	}
	ui.End()
	q.Tick()
	in.BeginFrame()
	ui.Begin(in, mathx.Vec2{X: 300, Y: 300}, 1)
	if !p.wave(&info, mathx.Rect{W: 100, H: 50}) {
		t.Fatal("the generated waveform was not available")
	}
	ui.End()
	old := slices.Clone(thumbs.CachedPeaks(id, waveCols))
	if len(old) != waveCols || old[0] < 0.7 {
		t.Fatalf("cached waveform does not match the imported sound: %v", old)
	}
	drawFolder()
	if q.Busy() {
		t.Fatal("an already rendered folder started another waveform job")
	}
	// A changed source must not make a folder display stale waveform data or
	// start decoding it again until the actual tile is opened.
	write(0.25)
	importAll()
	if stale := thumbs.CachedPeaks(id, waveCols); len(stale) != 0 {
		t.Fatalf("changed audio retained a stale waveform: %v", stale)
	}
	drawFolder()
	if q.Busy() {
		t.Fatal("a changed sound was eagerly decoded by its closed folder")
	}
	in.BeginFrame()
	ui.Begin(in, mathx.Vec2{X: 300, Y: 300}, 1)
	if p.wave(&info, mathx.Rect{W: 100, H: 50}) || !q.Busy() {
		t.Fatal("the changed audio tile did not request a fresh waveform")
	}
	ui.End()
	q.Tick()
	if fresh := thumbs.CachedPeaks(id, waveCols); len(fresh) != waveCols || fresh[0] > 0.3 || slices.Equal(fresh, old) {
		t.Fatalf("the changed sound was not decoded afresh: %v", fresh)
	}
}
