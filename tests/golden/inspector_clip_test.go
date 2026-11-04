//go:build aego_golden

package golden

import (
	"math"
	"path/filepath"
	"testing"

	"aego/app/edit"
	"aego/asset"
	audioimport "aego/asset/importers/audio"
	"aego/asset/pipeline"
	"aego/audio/codec/wav"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/format/aetext"
	"aego/modules/audio"
	audioed "aego/modules/audio/editor"
	"aego/project"
)

func TestEditorInspectorClip(t *testing.T) {
	const path = "assets/sounds/jump.wav"
	const rate = 48000
	p, err := project.Create(t.TempDir(), "ClipGolden", false)
	if err != nil {
		t.Fatal(err)
	}
	peaks := make([]float32, 256)
	pcm := make([]float32, rate)
	for i := range peaks {
		f := float64(i)
		peaks[i] = float32(0.08 + 0.85*math.Abs(math.Sin(f*0.11))*math.Exp(-f/200))
		pcm[i*len(pcm)/len(peaks)] = peaks[i]
	}
	source := filepath.Join(p.Root, filepath.FromSlash(path))
	if err := document.WriteAtomic(source, wav.Encode(pcm, rate, 1, wav.EncodeOptions{Bits: 16})); err != nil {
		t.Fatal(err)
	}
	meta := asset.NewSourceMeta("audio", audioimport.Version, pipeline.Settings{
		"stream": aetext.Ident("auto"), "gain": aetext.Float64(0.8), "bus": aetext.Ident("sfx"),
		"loopStart": aetext.Int(12000), "loopEnd": aetext.Int(36000),
	})
	meta.ID = ids.AssetID{Hi: 0xa1, Lo: 1}
	if err := document.WriteAtomic(source+".meta", asset.MarshalSourceMeta(meta)); err != nil {
		t.Fatal(err)
	}

	// Open through the registered audio handler: the document tab and Inspector
	// must see one metadata journal from their very first presentation.
	session, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	session.Step()
	ctx := session.App.Context()
	doc, err := ctx.OpenAsset(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Handler != "audio.clip" || doc.Source != nil || doc.Metadata == nil || doc.Dirty() || !doc.Capabilities.Has(document.EditMetadata) {
		t.Fatal("first audio open did not create the editable metadata document")
	}
	if inspected, err := ctx.DocumentForAsset(meta.ID); err != nil || inspected != doc || ctx.Docs.Script(path+".meta") != doc.Metadata {
		t.Fatal("clip inspector and document tab obtained different journals", err)
	}
	parsed, err := asset.ParseSourceMeta(doc.Metadata.Code.Bytes(), doc.Metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	mod := audio.ModuleOf(ctx.Engine)
	if mod == nil {
		t.Fatal("project audio module is unavailable")
	}
	clip, err := mod.Clip(ctx.Loader, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	art := clip.Art
	v := audioed.ClipView{
		ID: meta.ID, Path: doc.Path, Format: art.Format, Rate: art.Rate, Channels: art.Channels,
		Frames: art.Frames, Seconds: clip.Seconds, LoopStart: art.LoopStart, LoopEnd: art.LoopEnd,
		Peaks: peaks, PeaksDone: true, Settings: parsed.Settings, Position: 0.4, Playing: true,
	}
	// The document/importer are real. Only the waveform and transport position
	// are fixed here, so checking this panel does not depend on audio playback.
	env := newEditorEnv(t)
	prevUI := ctx.UI
	ctx.UI = env.ui
	defer func() { ctx.UI = prevUI }()
	img, err := env.draw(func(mathx.Rect) { audioed.DrawClip(ctx, v) })
	checkGolden(t, "editor_inspector_clip", img, err)
}
