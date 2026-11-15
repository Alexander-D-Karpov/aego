package editor

import (
	"testing"

	"aego/asset"
	"aego/audio/mix"
	"aego/core/log"
	"aego/core/mathx"
	"aego/demos"
	"aego/editor/core/context"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/i18n"
	"aego/engine/module"
	"aego/modules/audio"
	"aego/platform/input"
	"aego/render/text"
	"aego/script/bind"
	"aego/ui/core"
)

func TestAudioTransportButtonsControlOnlyPreview(t *testing.T) {
	bundle := i18n.Default()
	previous := bundle.Language()
	bundle.SetLanguage(i18n.English)
	defer bundle.SetLanguage(previous)
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
	e, err := engine.NewBuilder(engine.Config{Headless: true, HeadlessFrameTime: 1.0 / 60,
		UserRoot: t.TempDir(), AppID: "audio-transport-test"}).WithLogger(log.Nop()).Use(man.Engine()).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := man.Init(&module.InitContext{Engine: e, Settings: settings, Log: log.Nop()}); err != nil {
		t.Fatal(err)
	}
	defer man.Shutdown()
	settings.Start()
	mod := audio.ModuleOf(e)
	if mod == nil || mod.Mixer() == nil {
		t.Fatal("no audio module")
	}
	ui := core.New(nil)
	font := text.NewFont(14, 1, 16, 12)
	for ch := rune(32); ch < 127; ch++ {
		font.SetGlyph(ch, text.FontGlyph{Advance: 8})
	}
	ui.SetFont(font)
	ed := &context.Editor{UI: ui}
	p := &plugin{ed: ed, clip: clipEdit{loop: true}}
	state := &input.State{}
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
		ui.Region(mathx.Rect{W: 320, H: 180})
		v := ClipView{ID: asset.BuiltinMelody, Seconds: 30}
		if mod.PreviewClip() == v.ID {
			v.Position, v.Playing = mod.PreviewPosition()
			v.Paused = v.Playing && mod.PreviewPaused()
		}
		p.transport(ed, mod, v)
		ui.PopCursor()
		ui.End()
	}
	click := func(x float32) {
		at := mathx.Vec2{X: x, Y: ui.Theme.Padding + ui.Theme.RowHeight/2}
		state.BeginFrame()
		state.ApplyMouseDown(at, input.MouseLeft)
		draw()
		state.BeginFrame()
		state.ApplyMouseUp(at, input.MouseLeft)
		draw()
	}
	step := func(n int) {
		for range n {
			if !e.Step(demos.Base{}) {
				t.Fatal("headless engine stopped", e.Err())
			}
		}
	}
	game := mod.Play(&mix.Memory{PCM: make([]float32, 48000), Rate: 48000, Channels: 1}, "sfx", audio.PlayOptions{Loop: true})
	if !game.Valid() {
		t.Fatal("could not create independent game voice")
	}
	click(16) // Play
	if mod.PreviewClip() != asset.BuiltinMelody || mod.PreviewPaused() {
		t.Fatal("Play did not start the preview")
	}
	step(4)
	click(16) // Pause
	if !mod.PreviewPaused() {
		t.Fatal("Pause button did not queue pause")
	}
	step(4)
	paused, _ := mod.PreviewPosition()
	gameBefore := mod.Mixer().Position(game)
	step(8)
	again, _ := mod.PreviewPosition()
	if paused != again || mod.Mixer().Position(game) <= gameBefore {
		t.Fatal("Pause stopped the wrong voice or did not hold preview position")
	}
	click(16) // Resume
	if mod.PreviewPaused() {
		t.Fatal("Resume button did not queue resume")
	}
	step(4)
	resumed, _ := mod.PreviewPosition()
	if resumed <= paused {
		t.Fatal("Resume button did not restart preview")
	}
	queue := mod.Mixer().Commands()
	count := 0
	for count < 4096 && queue.Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
		count++
	}
	if count == 4096 {
		t.Fatal("fixture could not fill command queue")
	}
	click(80) // Stop, rejected by the full queue.
	if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError || mod.PreviewClip() != asset.BuiltinMelody {
		t.Fatal("Stop failure was not reported, or lost control of the preview")
	}
	step(3)
	click(80) // Retry Stop.
	step(3)
	if !mod.PreviewClip().IsZero() || !mod.Mixer().Live(game) {
		t.Fatal("Stop did not release only the preview voice")
	}
	if ed.Docs != nil {
		t.Fatal("transport changed document state")
	}
}
