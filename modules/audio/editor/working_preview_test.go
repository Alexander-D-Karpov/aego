package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"aego/asset"
	audioimport "aego/asset/importers/audio"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/audio/codec/wav"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/i18n"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/gpu/null"
	"aego/modules/audio"
	"aego/platform/input"
	"aego/project"
	"aego/render/text"
	"aego/render/texture"
	"aego/script/bind"
	"aego/ui/core"
)

type workingAudioFixture struct {
	ed             *context.Editor
	plugin         *plugin
	module         *audio.Module
	doc            *document.AssetDocument
	id             ids.AssetID
	file           string
	source, meta   []byte
	oldArtifact    asset.Artifact
	oldArtifactRaw []byte
}

func newWorkingAudioFixture(t *testing.T, ext string, streamed bool) *workingAudioFixture {
	t.Helper()
	var source []byte
	if ext == ".wav" {
		pcm := make([]float32, 48000)
		for i := range pcm {
			pcm[i] = .25
		}
		source = wav.Encode(pcm, 48000, 1, wav.EncodeOptions{Float: true})
	} else {
		var err error
		source, err = os.ReadFile(filepath.Join("..", "..", "..", "audio", "codec", "testdata", "tone.ogg"))
		if err != nil {
			t.Fatal(err)
		}
	}
	return newWorkingAudioFixtureWithSource(t, ext, streamed, source)
}

func newWorkingAudioFixtureWithSource(t *testing.T, ext string, streamed bool, source []byte) *workingAudioFixture {
	t.Helper()
	pj, err := project.Create(t.TempDir(), "WorkingAudioPreview", false)
	if err != nil {
		t.Fatal(err)
	}
	path := "assets/tone" + ext
	file := filepath.Join(pj.Root, filepath.FromSlash(path))
	if err := os.WriteFile(file, source, 0o644); err != nil {
		t.Fatal(err)
	}
	imp := audioimport.New()
	meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
	meta.Settings["stream"] = aetext.Bool(streamed)
	before := asset.MarshalSourceMeta(meta)
	if err := os.WriteFile(file+asset.MetaSuffix, before, 0o644); err != nil {
		t.Fatal(err)
	}
	reg := pipeline.NewImporterRegistry()
	if err := reg.Register(imp); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(pj, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.Import(meta.ID)
	if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v %v", batch, err)
	}
	old, ok := pl.PrimaryArtifact(meta.ID)
	if !ok {
		t.Fatal("missing imported artifact")
	}
	oldRaw, err := pl.Artifacts().ReadFile(old.Path)
	if err != nil {
		t.Fatal(err)
	}
	dev := null.New()
	load := loader.New(dev, texture.NewUploader(dev), log.Nop())
	if err := load.Mount(pl.Index(), pl.Artifacts()); err != nil {
		t.Fatal(err)
	}
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := module.Load(types, bind.New(), audio.New())
	if err != nil {
		t.Fatal(err)
	}
	settings, err := man.NewSettings()
	if err != nil {
		t.Fatal(err)
	}
	eng, err := engine.NewBuilder(engine.Config{Headless: true, HeadlessFrameTime: 1.0 / 60,
		UserRoot: t.TempDir(), AppID: "working-audio-preview"}).WithLogger(log.Nop()).Use(man.Engine()).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := man.Init(&module.InitContext{Engine: eng, Settings: settings, Log: log.Nop()}); err != nil {
		t.Fatal(err)
	}
	settings.Start()
	mod := audio.ModuleOf(eng)
	if mod == nil || mod.Mixer() == nil {
		t.Fatal("headless audio output is missing")
	}
	docs := document.NewManager(document.Options{Root: pj.Root})
	if err := docs.Handlers().Register(document.Handler{ID: "audio.clip", Kinds: []asset.ArtifactKind{asset.ArtAudio}, Capabilities: document.View | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	ed := &context.Editor{Project: pj, Pipeline: pl, Loader: load, Engine: eng, Docs: docs, UI: core.New(nil)}
	font := text.NewFont(14, 1, 16, 12)
	for ch := rune(32); ch < 127; ch++ {
		font.SetGlyph(ch, text.FontGlyph{Advance: 8})
	}
	ed.UI.SetFont(font)
	doc, err := ed.DocumentForAsset(meta.ID)
	if err != nil || doc.Metadata == nil {
		t.Fatal("cannot open shared audio document", err)
	}
	owner := &plugin{ed: ed, clip: clipEdit{id: meta.ID, drag: -1, loop: true}}
	next := meta.Settings.Clone()
	next["gain"] = aetext.Float64(2)
	owner.write(ed, meta.Settings, next)
	if !doc.Dirty() || !doc.CanUndo() {
		t.Fatal("fixture did not create a shared working edit")
	}
	t.Cleanup(func() {
		owner.cancelWorkingAudio()
		if owner.workingAudio.result != nil {
			select {
			case <-owner.workingAudio.result:
			case <-time.After(5 * time.Second):
				t.Error("private preview worker did not stop")
			}
		}
		_ = mod.StopPreview()
		man.Shutdown()
		eng.Close()
		load.Close()
		dev.Close()
		pl.Close()
	})
	return &workingAudioFixture{ed: ed, plugin: owner, module: mod, doc: doc, id: meta.ID,
		file: file, source: source, meta: before, oldArtifact: old, oldArtifactRaw: oldRaw}
}

func (f *workingAudioFixture) checkFiles(t *testing.T, source, meta []byte) {
	t.Helper()
	if got, err := os.ReadFile(f.file); err != nil || !bytes.Equal(got, source) {
		t.Fatal("working preview changed source", err)
	}
	if got, err := os.ReadFile(f.file + asset.MetaSuffix); err != nil || !bytes.Equal(got, meta) {
		t.Fatal("working preview changed metadata on disk", err)
	}
	if got, err := f.ed.Pipeline.Artifacts().ReadFile(f.oldArtifact.Path); err != nil || !bytes.Equal(got, f.oldArtifactRaw) {
		t.Fatal("working preview changed the saved artifact", err)
	}
}

func waitWorkingAudio(t *testing.T, f *workingAudioFixture) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for f.plugin.workingAudio.result != nil && time.Now().Before(deadline) {
		f.plugin.pollWorkingAudio(f.module)
		runtime.Gosched()
	}
	if f.plugin.workingAudio.result != nil {
		t.Fatal("working preview did not complete")
	}
}

func TestWorkingAudioPlayButtonUsesUnsavedSettingsWithoutPublishing(t *testing.T) {
	for _, ext := range []string{".wav", ".ogg"} {
		for _, streamed := range []bool{false, true} {
			name := ext + "/memory"
			if streamed {
				name = ext + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				f := newWorkingAudioFixture(t, ext, streamed)
				original, err := f.module.Clip(f.ed.Loader, f.id)
				if err != nil || original.Gain != 1 {
					t.Fatal("could not cache the saved clip", err)
				}
				state := &input.State{}
				draw := func() {
					ui := f.ed.UI
					ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
					ui.Region(mathx.Rect{W: 320, H: 180})
					v := ClipView{ID: f.id, Seconds: 1}
					if f.module.PreviewClip() == f.id {
						v.Position, v.Playing = f.module.PreviewPosition()
						v.Paused = v.Playing && f.module.PreviewPaused()
					}
					f.plugin.transport(f.ed, f.module, v)
					ui.PopCursor()
					ui.End()
				}
				at := mathx.Vec2{X: 16, Y: f.ed.UI.Theme.Padding + f.ed.UI.Theme.RowHeight/2}
				state.BeginFrame()
				state.ApplyMouseDown(at, input.MouseLeft)
				draw()
				state.BeginFrame()
				state.ApplyMouseUp(at, input.MouseLeft)
				draw()
				if !f.plugin.workingAudioPending(f.id) {
					t.Fatal("Play did not request a working preview")
				}
				waitWorkingAudio(t, f)
				if f.module.PreviewClip() != f.id {
					notice, _ := f.ed.LastNotice()
					t.Fatal("working preview did not start", notice)
				}
				if cached, err := f.module.Clip(f.ed.Loader, f.id); err != nil || cached != original || cached.Gain != 1 {
					t.Fatal("working preview changed the saved runtime cache", err)
				}
				if ext == ".wav" {
					var peak float32
					for range 100 {
						out := make([]float32, 512)
						f.module.Mixer().Mix(out)
						for _, sample := range out {
							peak = max(peak, sample)
						}
						if peak > .25 {
							break
						}
						time.Sleep(time.Millisecond)
					}
					if peak <= .25 {
						t.Fatalf("preview did not use unsaved gain 2: peak=%v", peak)
					}
				}
				if !f.doc.Dirty() || !f.doc.CanUndo() {
					t.Fatal("preview lost shared dirty state or Undo")
				}
				f.checkFiles(t, f.source, f.meta)
			})
		}
	}
}

func TestWorkingAudioRejectsChangesWhilePreparing(t *testing.T) {
	for _, reason := range []string{"source", "metadata", "edit", "undo-redo", "asset-generation", "project", "output-rate", "other-preview", "closed", "cancel", "late"} {
		t.Run(reason, func(t *testing.T) {
			f := newWorkingAudioFixture(t, ".wav", false)
			entered, releaseCh := make(chan struct{}), make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(releaseCh) }) }
			defer release()
			checkpoint := func() { close(entered); <-releaseCh }
			if reason == "source" || reason == "metadata" {
				f.plugin.workingAudio.beforeVerify = checkpoint
			} else {
				f.plugin.workingAudio.afterVerify = checkpoint
			}
			if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, true); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not reach its checkpoint")
			}
			wantSource, wantMeta := bytes.Clone(f.source), bytes.Clone(f.meta)
			var oldEpoch uint64
			switch reason {
			case "source":
				wantSource[len(wantSource)-1] ^= 1
				if err := os.WriteFile(f.file, wantSource, 0o644); err != nil {
					t.Fatal(err)
				}
			case "metadata":
				wantMeta = append(wantMeta, '\n')
				if err := os.WriteFile(f.file+asset.MetaSuffix, wantMeta, 0o644); err != nil {
					t.Fatal(err)
				}
			case "edit":
				meta, err := asset.ParseSourceMeta(f.doc.Metadata.Code.Bytes(), f.doc.Metadata.Path)
				if err != nil {
					t.Fatal(err)
				}
				next := meta.Settings.Clone()
				next["gain"] = aetext.Float64(3)
				f.plugin.write(f.ed, meta.Settings, next)
			case "undo-redo":
				before := bytes.Clone(f.doc.Metadata.Code.Bytes())
				if !f.doc.Metadata.Code.Undo() || !f.doc.Metadata.Code.Redo() || !bytes.Equal(before, f.doc.Metadata.Code.Bytes()) {
					t.Fatal("could not prepare a metadata ABA")
				}
			case "asset-generation":
				f.ed.AssetRev++
			case "project":
				f.ed.Project = &project.Project{}
			case "output-rate":
				f.module.Mixer().SetSampleRate(44100)
			case "other-preview":
				if err := f.module.Preview(f.ed.Loader, f.id, true); err != nil {
					t.Fatal(err)
				}
				oldEpoch = f.module.PreviewEpoch()
			case "closed":
				docs := f.ed.Docs
				docs.Subscribe(func(ev document.Event) {
					if ev.Kind == document.EvClosed {
						f.plugin.closeWorkingAudio(docs, ev.Doc)
					}
				})
				if err := docs.CloseAsset(f.id, true); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				f.plugin.cancelWorkingAudio()
			case "late":
				time.Sleep(1100 * time.Millisecond)
			}
			if reason != "late" {
				f.plugin.pollWorkingAudio(f.module)
			}
			release()
			waitWorkingAudio(t, f)
			if reason == "other-preview" {
				if f.module.PreviewClip() != f.id || f.module.PreviewEpoch() != oldEpoch {
					t.Fatal("stale work displaced the newer preview")
				}
			} else if !f.module.PreviewClip().IsZero() {
				t.Fatalf("stale %s preview unexpectedly started", reason)
			}
			if reason != "closed" && (!f.doc.Dirty() || !f.doc.CanUndo()) {
				t.Fatal("rejecting stale preview lost working history")
			}
			f.checkFiles(t, wantSource, wantMeta)
		})
	}
}

func TestWorkingAudioCoalescesLatestEdit(t *testing.T) {
	f := newWorkingAudioFixture(t, ".wav", false)
	entered, releaseCh := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(releaseCh) }) }
	defer release()
	f.plugin.workingAudio.afterVerify = func() { close(entered); <-releaseCh }
	if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first worker did not reach its checkpoint")
	}
	f.plugin.workingAudio.afterVerify = nil
	meta, err := asset.ParseSourceMeta(f.doc.Metadata.Code.Bytes(), f.doc.Metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	next := meta.Settings.Clone()
	next["gain"] = aetext.Float64(3)
	f.plugin.write(f.ed, meta.Settings, next)
	if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, true); err != nil {
		t.Fatal(err)
	}
	if !f.plugin.workingAudioPending(f.id) || f.plugin.workingAudio.waiting == nil {
		t.Fatal("latest working generation was not queued")
	}
	release()
	waitWorkingAudio(t, f)
	if f.module.PreviewClip() != f.id {
		t.Fatal("latest generation was not played")
	}
	out := make([]float32, 512)
	f.module.Mixer().Mix(out)
	var peak float32
	for _, sample := range out {
		peak = max(peak, sample)
	}
	if peak <= .45 {
		t.Fatalf("old gain was played instead of latest gain 3: peak=%v", peak)
	}
	if !f.doc.Dirty() || !f.doc.CanUndo() {
		t.Fatal("coalesced preview lost shared history")
	}
	f.checkFiles(t, f.source, f.meta)
}

func TestWorkingAudioTransportCancelsPendingPreparation(t *testing.T) {
	bundle := i18n.Default()
	previous := bundle.Language()
	bundle.SetLanguage(i18n.English)
	defer bundle.SetLanguage(previous)
	for _, control := range []string{"cancel", "stop", "loop"} {
		t.Run(control, func(t *testing.T) {
			f := newWorkingAudioFixture(t, ".wav", false)
			entered, releaseCh := make(chan struct{}), make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(releaseCh) }) }
			defer release()
			f.plugin.workingAudio.afterVerify = func() { close(entered); <-releaseCh }
			if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, true); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not reach its checkpoint")
			}
			state := &input.State{}
			draw := func() {
				ui := f.ed.UI
				ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
				ui.Region(mathx.Rect{W: 320, H: 180})
				f.plugin.transport(f.ed, f.module, ClipView{ID: f.id, Seconds: 1})
				ui.PopCursor()
				ui.End()
			}
			x := float32(16)
			if control == "stop" {
				x = 90
			} else if control == "loop" {
				x = 145
			}
			at := mathx.Vec2{X: x, Y: f.ed.UI.Theme.Padding + f.ed.UI.Theme.RowHeight/2}
			state.BeginFrame()
			state.ApplyMouseDown(at, input.MouseLeft)
			draw()
			state.BeginFrame()
			state.ApplyMouseUp(at, input.MouseLeft)
			draw()
			if f.plugin.workingAudioPending(f.id) {
				t.Fatalf("%s did not cancel the pending preview", control)
			}
			release()
			waitWorkingAudio(t, f)
			if !f.module.PreviewClip().IsZero() || !f.doc.Dirty() || !f.doc.CanUndo() {
				t.Fatal("cancelled preview played or lost the shared dirty document")
			}
			f.checkFiles(t, f.source, f.meta)
		})
	}
}

func TestWorkingAudioCancellationReleasesCompletedSnapshot(t *testing.T) {
	f := newWorkingAudioFixture(t, ".wav", false)
	if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, true); err != nil {
		t.Fatal(err)
	}
	var result workingAudioResult
	select {
	case result = <-f.plugin.workingAudio.result:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not complete")
	}
	if result.err != nil || result.prepared == nil {
		t.Fatal("worker did not prepare a valid preview", result.err)
	}
	if f.plugin.workingAudio.running.source != nil || f.plugin.workingAudio.running.metadata != nil {
		t.Fatal("completed worker retained its source and metadata snapshots")
	}
	// Put the completed result back as if the UI has not polled it yet.
	ready := make(chan workingAudioResult, 1)
	ready <- result
	f.plugin.workingAudio.result = ready
	f.plugin.cancelWorkingAudio()
	if f.plugin.workingAudio.running != nil || f.plugin.workingAudio.result != nil {
		t.Fatal("cancelling an already-completed preview retained its result")
	}
	if !f.module.PreviewClip().IsZero() || !f.doc.Dirty() || !f.doc.CanUndo() {
		t.Fatal("cancelling a completed preview played audio or lost work")
	}
	f.checkFiles(t, f.source, f.meta)
}
