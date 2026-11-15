package editor

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aego/asset/audiosidecar"
	"aego/audio/edit"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/platform/input"
)

func volatileWorkingCandidate(t *testing.T, f *workingAudioFixture) audioedit.Document {
	t.Helper()
	clip, err := f.module.Clip(f.ed.Loader, f.id)
	if err != nil || clip == nil || clip.Art == nil {
		t.Fatal("cannot load preview source", err)
	}
	base, err := audioedit.New(audioedit.Source{AssetID: f.id.String(), Path: f.doc.Path,
		SHA256: fmt.Sprintf("%x", sha256.Sum256(f.source)), Codec: strings.TrimPrefix(filepath.Ext(f.doc.Path), "."),
		SampleRate: clip.Art.Rate, Channels: clip.Art.Channels, Frames: clip.Art.Frames})
	if err != nil {
		t.Fatal(err)
	}
	next, changed, err := audioedit.PrepareMutation(base, base.Revision, func(d *audioedit.Document) error {
		audioedit.AddOperation(d, audioedit.Operation{Kind: audioedit.Gain, Region: d.Trim, Value: .25})
		return nil
	})
	if err != nil || !changed {
		t.Fatal("cannot prepare audio candidate", err)
	}
	return next
}

func TestVolatileWorkingAudioOGGMatchesEditedGain(t *testing.T) {
	peakFor := func(t *testing.T, edited, streamed bool) float64 {
		t.Helper()
		f := newWorkingAudioFixture(t, ".ogg", streamed)
		if edited {
			candidate := volatileWorkingCandidate(t, f)
			raw, err := candidate.Encode()
			if err != nil {
				t.Fatal(err)
			}
			storeVolatileAudio(t, f, raw)
		}
		if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, false); err != nil {
			t.Fatal(err)
		}
		waitWorkingAudio(t, f)
		if f.module.PreviewClip() != f.id {
			notice, _ := f.ed.LastNotice()
			t.Fatal("OGG working preview did not start", notice)
		}
		var peak float64
		for i := 0; i < 80; i++ {
			out := make([]float32, 2048)
			f.module.Mixer().Mix(out)
			for _, sample := range out {
				peak = max(peak, math.Abs(float64(sample)))
			}
			if i >= 10 && peak > .0001 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		f.checkFiles(t, f.source, f.meta)
		return peak
	}
	for _, streamed := range []bool{false, true} {
		name := "memory"
		if streamed {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			baseline := peakFor(t, false, streamed)
			edited := peakFor(t, true, streamed)
			if baseline < .0001 || edited < baseline*.1 || edited > baseline*.65 {
				t.Fatalf("OGG edited preview does not reflect gain: original=%v edited=%v", baseline, edited)
			}
		})
	}
}

func storeVolatileAudio(t *testing.T, f *workingAudioFixture, raw []byte) {
	t.Helper()
	journal := f.doc.Metadata.Code
	if err := journal.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if changed, err := journal.ReplaceVolatile(journal.Rev(), raw); err != nil || !changed {
		t.Fatal("cannot record volatile audio candidate", err)
	}
}

func TestVolatileWorkingAudioUsesEditedSignal(t *testing.T) {
	for _, streamed := range []bool{false, true} {
		name := "memory"
		if streamed {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			f := newWorkingAudioFixture(t, ".wav", streamed)
			original, err := f.module.Clip(f.ed.Loader, f.id)
			if err != nil {
				t.Fatal(err)
			}
			candidate := volatileWorkingCandidate(t, f)
			raw, err := candidate.Encode()
			if err != nil {
				t.Fatal(err)
			}
			storeVolatileAudio(t, f, raw)
			if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, false); err != nil {
				t.Fatal(err)
			}
			request := f.plugin.workingAudio.running
			waitWorkingAudio(t, f)
			if request == nil || request.edit != nil || request.metadata != nil || request.source != nil {
				t.Fatal("completed preview retained its private source snapshots")
			}
			if f.module.PreviewClip() != f.id {
				notice, _ := f.ed.LastNotice()
				t.Fatal("edited working preview did not start", notice)
			}
			var peak float64
			for range 100 {
				out := make([]float32, 2048)
				f.module.Mixer().Mix(out)
				for _, sample := range out {
					peak = max(peak, math.Abs(float64(sample)))
				}
				if peak > .035 {
					break
				}
				time.Sleep(time.Millisecond)
			}
			// Synthetic source is .25, import gain is 2, edit gain is .25.
			// The wide range allows the mixer's pan/fade while rejecting the
			// original unedited .5 signal.
			if peak < .035 || peak > .19 {
				t.Fatalf("working preview played the wrong PCM: peak=%v", peak)
			}
			if cached, err := f.module.Clip(f.ed.Loader, f.id); err != nil || cached != original || cached.Gain != 1 {
				t.Fatal("edited preview changed the saved runtime cache", err)
			}
			if !f.doc.Metadata.Code.VolatileDirty() || !f.doc.CanUndo() {
				t.Fatal("edited preview lost the working history")
			}
			f.checkFiles(t, f.source, f.meta)
		})
	}
}

func TestVolatileWorkingAudioPlayButtonWithCleanText(t *testing.T) {
	f := newWorkingAudioFixture(t, ".wav", false)
	journal := f.doc.Metadata.Code
	if !journal.Undo() || journal.Dirty() {
		t.Fatal("cannot restore the saved metadata before the volatile edit")
	}
	candidate := volatileWorkingCandidate(t, f)
	raw, err := candidate.Encode()
	if err != nil {
		t.Fatal(err)
	}
	storeVolatileAudio(t, f, raw)
	state := &input.State{}
	draw := func() {
		ui := f.ed.UI
		ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
		ui.Region(mathx.Rect{W: 320, H: 180})
		f.plugin.transport(f.ed, f.module, ClipView{ID: f.id, Seconds: 1})
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
		t.Fatal("Play ignored a volatile edit with otherwise clean metadata")
	}
	waitWorkingAudio(t, f)
	if f.module.PreviewClip() != f.id {
		notice, _ := f.ed.LastNotice()
		t.Fatal("Play did not start the volatile preview", notice)
	}
	out := make([]float32, 2048)
	f.module.Mixer().Mix(out)
	var peak float64
	for _, sample := range out {
		peak = max(peak, math.Abs(float64(sample)))
	}
	if peak < .015 || peak > .11 {
		t.Fatalf("Play did not use the in-memory edit: peak=%v", peak)
	}
	f.checkFiles(t, f.source, f.meta)
}

func TestVolatileWorkingAudioRejectsInvalidState(t *testing.T) {
	for _, reason := range []string{"json", "digest", "asset", "path", "layout", "metadata-envelope"} {
		t.Run(reason, func(t *testing.T) {
			f := newWorkingAudioFixture(t, ".wav", false)
			candidate := volatileWorkingCandidate(t, f)
			switch reason {
			case "digest":
				candidate.Source.SHA256 = strings.Repeat("0", 64)
			case "asset":
				candidate.Source.AssetID = ids.NewAssetID().String()
			case "path":
				candidate.Source.Path = "assets/other.wav"
			case "layout":
				candidate.Source.SampleRate = 44100
			}
			raw, err := candidate.Encode()
			if err != nil {
				t.Fatal(err)
			}
			if reason == "json" {
				raw = []byte("{invalid audio state")
			}
			if reason == "metadata-envelope" {
				journal := f.doc.Metadata.Code
				meta, err := audiosidecar.Encode(journal.Bytes(), f.doc.Path, candidate)
				if err != nil {
					t.Fatal(err)
				}
				journal.ReplaceSnapshot(meta, false)
			}
			storeVolatileAudio(t, f, raw)
			if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, false); err != nil {
				t.Fatal(err)
			}
			waitWorkingAudio(t, f)
			if !f.module.PreviewClip().IsZero() {
				t.Fatal("invalid volatile state played instead of being rejected")
			}
			if !f.doc.Metadata.Code.VolatileDirty() || !f.doc.CanUndo() {
				t.Fatal("invalid preview discarded the working history")
			}
			f.checkFiles(t, f.source, f.meta)
		})
	}
}

func TestVolatileWorkingAudioRejectsUndoRedoABA(t *testing.T) {
	f := newWorkingAudioFixture(t, ".wav", false)
	candidate := volatileWorkingCandidate(t, f)
	raw, err := candidate.Encode()
	if err != nil {
		t.Fatal(err)
	}
	storeVolatileAudio(t, f, raw)
	entered, releaseCh := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(releaseCh) }) }
	defer release()
	f.plugin.workingAudio.afterVerify = func() { close(entered); <-releaseCh }
	if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("preview did not reach its verification checkpoint")
	}
	journal := f.doc.Metadata.Code
	if !journal.Undo() || !journal.Redo() {
		t.Fatal("cannot prepare volatile undo/redo ABA")
	}
	if restored, _ := journal.Volatile(); !bytes.Equal(restored, raw) {
		t.Fatal("undo/redo did not restore the candidate bytes")
	}
	f.plugin.pollWorkingAudio(f.module)
	release()
	waitWorkingAudio(t, f)
	if !f.module.PreviewClip().IsZero() || !journal.VolatileDirty() {
		t.Fatal("stale preview played or the working history was lost")
	}
	f.checkFiles(t, f.source, f.meta)
}
