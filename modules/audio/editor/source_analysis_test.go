package editor

import (
	"bytes"
	"math"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"aego/asset"
	"aego/audio/codec"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/modules/audio"
	"aego/platform/input"
)

func sourceAnalysisFixture(t *testing.T) (*workingAudioFixture, ClipView) {
	return sourceAnalysisFixtureWithExt(t, ".wav")
}

func sourceAnalysisFixtureWithExt(t *testing.T, ext string) (*workingAudioFixture, ClipView) {
	t.Helper()
	f := newWorkingAudioFixture(t, ext, false)
	clip, err := f.module.Clip(f.ed.Loader, f.id)
	if err != nil || clip == nil || clip.Art == nil {
		t.Fatal("cannot load audio for analysis", err)
	}
	f.plugin.clip.bindWaveSource(f.ed.Docs, clip)
	f.plugin.clip.selectAnchor, f.plugin.clip.selectEnd = 1000, 4000
	t.Cleanup(func() {
		f.plugin.cancelSourceAnalysis()
		if ch := f.plugin.analysis.result; ch != nil {
			select {
			case <-ch:
			case <-time.After(5 * time.Second):
				t.Error("source analysis worker did not stop")
			}
		}
	})
	return f, ClipView{ID: f.id, source: clip, Path: "assets/tone" + ext, Rate: clip.Art.Rate,
		Channels: clip.Art.Channels, Frames: clip.Art.Frames}
}

func waitSourceAnalysis(t *testing.T, p *plugin) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		p.pollSourceAnalysis()
		if p.analysis.running == nil && p.analysis.waiting == nil {
			return
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("source analysis did not finish")
}

func TestSourceAnalysisUnavailableLocalized(t *testing.T) {
	bundle := i18n.Default()
	if err := bundle.LoadOnce("audio-source-analysis-error-test", os.DirFS("../res"), "i18n", ""); err != nil {
		t.Fatal(err)
	}
	previous := bundle.Language()
	defer bundle.SetLanguage(previous)
	for _, tc := range []struct{ language, want string }{
		{i18n.English, "Analysis requires a selected WAV or OGG source of at most 64 MiB."},
		{i18n.Russian, "Для анализа нужно выделение WAV или OGG размером не более 64 МиБ."},
	} {
		bundle.SetLanguage(tc.language)
		p := &plugin{}
		err := p.startSourceAnalysis(nil, ClipView{})
		if err == nil || err.Error() != tc.want || p.analysis.running != nil {
			t.Errorf("%s: got %v, want %q without a worker", tc.language, err, tc.want)
		}
	}
}

func TestSourceAnalysisDoesNotPublishOrInterruptPreview(t *testing.T) {
	f, v := sourceAnalysisFixture(t)
	if err := f.module.Preview(f.ed.Loader, f.id, true); err != nil {
		t.Fatal(err)
	}
	epoch := f.module.PreviewEpoch()
	journal, revision := bytes.Clone(f.doc.Metadata.Code.Bytes()), f.doc.Metadata.Code.Rev()
	if err := f.plugin.startSourceAnalysis(f.ed, v); err != nil {
		t.Fatal(err)
	}
	waitSourceAnalysis(t, f.plugin)
	result := f.plugin.analysis.ready
	if result == nil || result.value.Frames != 3000 || len(result.value.Channels) != 1 ||
		result.value.Channels[0].Peak != .25 || result.value.Channels[0].RMS != .25 || result.value.Channels[0].ClippedFrames != 0 {
		t.Fatalf("expected the original source, not dirty import gain: %+v", result)
	}
	if f.module.PreviewEpoch() != epoch || f.module.PreviewClip() != f.id {
		t.Fatal("analysis interrupted the playing preview")
	}
	if f.doc.Metadata.Code.Rev() != revision || !bytes.Equal(f.doc.Metadata.Code.Bytes(), journal) || !f.doc.Dirty() || !f.doc.CanUndo() {
		t.Fatal("analysis changed the shared working history")
	}
	f.checkFiles(t, f.source, f.meta)
}

func TestSourceAnalysisOGGMatchesSequentialPCM(t *testing.T) {
	f, v := sourceAnalysisFixtureWithExt(t, ".ogg")
	decoder, err := codec.Open(bytes.NewReader(f.source), v.Path)
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := codec.DecodeAll(decoder, 0)
	closeErr := decoder.Close()
	if err != nil || closeErr != nil {
		t.Fatal("cannot sequentially decode OGG", err, closeErr)
	}
	if err := f.plugin.startSourceAnalysis(f.ed, v); err != nil {
		t.Fatal(err)
	}
	waitSourceAnalysis(t, f.plugin)
	result := f.plugin.analysis.ready
	if result == nil || len(result.value.Channels) != v.Channels || result.value.Frames != 3000 {
		t.Fatalf("missing OGG selection analysis: %+v", result)
	}
	for channel := range v.Channels {
		peak, sum := 0.0, 0.0
		var clipped int64
		for frame := 1000; frame < 4000; frame++ {
			value := float64(pcm.PCM[frame*v.Channels+channel])
			peak = max(peak, math.Abs(value))
			sum += value * value
			if math.Abs(value) >= 1 {
				clipped++
			}
		}
		got := result.value.Channels[channel]
		if math.Abs(got.Peak-peak) > 1e-6 || math.Abs(got.RMS-math.Sqrt(sum/3000)) > 1e-6 || got.ClippedFrames != clipped {
			t.Fatalf("OGG channel %d mismatch: %+v", channel, got)
		}
	}
	f.checkFiles(t, f.source, f.meta)
}

func TestSourceAnalysisRejectsChangedSelectionAndSource(t *testing.T) {
	for _, change := range []string{"selection", "source", "closed"} {
		t.Run(change, func(t *testing.T) {
			f, v := sourceAnalysisFixture(t)
			entered, releaseCh := make(chan struct{}), make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(releaseCh) }) }
			defer release()
			f.plugin.analysis.beforeVerify = func() { close(entered); <-releaseCh }
			journal, revision := bytes.Clone(f.doc.Metadata.Code.Bytes()), f.doc.Metadata.Code.Rev()
			if err := f.plugin.startSourceAnalysis(f.ed, v); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not reach its verification checkpoint")
			}
			source := bytes.Clone(f.source)
			switch change {
			case "selection":
				f.plugin.clip.selectEnd++
				f.plugin.pollSourceAnalysis()
			case "source":
				source[44] ^= 1
				if err := os.WriteFile(f.file, source, 0o644); err != nil {
					t.Fatal(err)
				}
			case "closed":
				f.plugin.closeSourceAnalysis(f.ed.Docs, f.doc.ID)
			}
			release()
			waitSourceAnalysis(t, f.plugin)
			if f.plugin.analysis.ready != nil {
				t.Fatalf("%s produced a stale result", change)
			}
			if f.doc.Metadata.Code.Rev() != revision || !bytes.Equal(f.doc.Metadata.Code.Bytes(), journal) || !f.doc.Dirty() || !f.doc.CanUndo() {
				t.Fatal("rejecting analysis changed the shared history")
			}
			f.checkFiles(t, source, f.meta)
		})
	}
}

func TestSourceAnalysisCoalescesLatestSelection(t *testing.T) {
	f, v := sourceAnalysisFixture(t)
	entered, releaseCh := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
	defer release()
	f.plugin.analysis.beforeVerify = func() { enteredOnce.Do(func() { close(entered); <-releaseCh }) }
	if err := f.plugin.startSourceAnalysis(f.ed, v); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first analysis did not reach its checkpoint")
	}
	f.plugin.clip.selectAnchor, f.plugin.clip.selectEnd = 5000, 9000
	if err := f.plugin.startSourceAnalysis(f.ed, v); err != nil {
		t.Fatal(err)
	}
	if f.plugin.analysis.waiting == nil {
		t.Fatal("latest analysis was not queued")
	}
	release()
	waitSourceAnalysis(t, f.plugin)
	result := f.plugin.analysis.ready
	if result == nil || result.request.region.Start != 5000 || result.request.region.End != 9000 || result.value.Frames != 4000 {
		t.Fatalf("latest selection was not analyzed: %+v", result)
	}
	f.checkFiles(t, f.source, f.meta)
}

func TestSourceAnalysisCancelDetachesClosedView(t *testing.T) {
	f, v := sourceAnalysisFixture(t)
	entered, releaseCh := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(releaseCh) }) }
	defer release()
	f.plugin.analysis.beforeVerify = func() { close(entered); <-releaseCh }
	if err := f.plugin.startSourceAnalysis(f.ed, v); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not reach its checkpoint")
	}
	running := f.plugin.analysis.running
	f.plugin.closeSourceAnalysis(f.ed.Docs, f.doc.ID)
	if running.view != nil || running.current != nil {
		t.Fatal("canceled analysis retained the closed view or its source buffer")
	}
	release()
	waitSourceAnalysis(t, f.plugin)
}

func TestSourceAnalysisClosingOldDocumentKeepsNewRequest(t *testing.T) {
	first, v1 := sourceAnalysisFixture(t)
	second, v2 := sourceAnalysisFixture(t)
	second.plugin.auditionOwner = first.plugin
	entered, releaseCh := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
	defer release()
	first.plugin.analysis.beforeVerify = func() { enteredOnce.Do(func() { close(entered); <-releaseCh }) }
	if err := first.plugin.startSourceAnalysis(first.ed, v1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("old analysis did not reach its checkpoint")
	}
	if err := second.plugin.startSourceAnalysis(second.ed, v2); err != nil {
		t.Fatal(err)
	}
	sequence := first.plugin.analysis.sequence
	first.plugin.closeSourceAnalysis(first.ed.Docs, first.doc.ID)
	if first.plugin.analysis.waiting == nil || first.plugin.analysis.sequence != sequence {
		t.Fatal("closing an older document canceled the newer analysis")
	}
	release()
	waitSourceAnalysis(t, first.plugin)
	if ready := first.plugin.analysis.ready; ready == nil || ready.request.view != second.plugin {
		t.Fatal("newer document did not receive its analysis")
	}
	first.checkFiles(t, first.source, first.meta)
	second.checkFiles(t, second.source, second.meta)
}

func TestSourceAnalysisRejectsUnboundBuffer(t *testing.T) {
	f, v := sourceAnalysisFixture(t)
	clip, artifact := *v.source, *v.source.Art
	artifact.Data = bytes.Clone(artifact.Data)
	clip.Art = &artifact
	v.source = &clip
	if err := f.plugin.startSourceAnalysis(f.ed, v); err == nil || f.plugin.analysis.running != nil {
		t.Fatal("analysis accepted a buffer different from the displayed waveform")
	}
	f.checkFiles(t, f.source, f.meta)
}

func TestSourceAnalysisButtonRunsOnSelectedSource(t *testing.T) {
	f, v := sourceAnalysisFixture(t)
	state, ui := &input.State{}, f.ed.UI
	area := mathx.Rect{W: 320, H: 240}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: 320, Y: 240}, 1)
		ui.Region(area)
		ui.PushID("clip/" + v.ID.String())
		f.plugin.sourceAnalysisUI(f.ed, v)
		ui.PopID()
		ui.PopCursor()
		ui.End()
	}
	at := mathx.Vec2{X: ui.Theme.Padding + 10, Y: ui.Theme.Padding + ui.Theme.RowHeight/2}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(at, input.MouseLeft)
	draw()
	if f.plugin.analysis.running == nil {
		t.Fatal("analysis button did not start a worker")
	}
	waitSourceAnalysis(t, f.plugin)
	if f.plugin.analysis.ready == nil {
		t.Fatal("analysis button did not produce a result")
	}
	state.BeginFrame()
	draw()
	f.checkFiles(t, f.source, f.meta)
}

func TestSourceAnalysisRefusesUnsupportedOrOversizedSource(t *testing.T) {
	f, v := sourceAnalysisFixture(t)
	original := v
	v.Path = "assets/tone.mp3"
	if err := f.plugin.startSourceAnalysis(f.ed, v); err == nil || f.plugin.analysis.running != nil {
		t.Fatal("analysis accepted an unsupported source")
	}
	clip, artifact := *original.source, *original.source.Art
	artifact.Data = make([]byte, audio.MaxPrivatePreviewPayloadBytes+1)
	clip.Art = &artifact
	original.source = &clip
	f.plugin.clip.bindWaveSource(f.ed.Docs, &clip)
	f.plugin.clip.selectAnchor, f.plugin.clip.selectEnd = 1000, 4000
	if err := f.plugin.startSourceAnalysis(f.ed, original); err == nil || f.plugin.analysis.running != nil {
		t.Fatal("analysis accepted an oversized source")
	}
	if got, err := os.ReadFile(f.file + asset.MetaSuffix); err != nil || !bytes.Equal(got, f.meta) {
		t.Fatal("refusing analysis changed metadata", err)
	}
}
