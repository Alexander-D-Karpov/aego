package editor

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/mix"
	"aego/core/mathx"
	"aego/demos"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/modules/audio"
	"aego/platform/input"
)

func TestAudioSeekSourceErrorLocalized(t *testing.T) {
	bundle := i18n.Default()
	if err := bundle.LoadOnce("audio-seek-source-error-test", os.DirFS("../res"), "i18n", ""); err != nil {
		t.Fatal(err)
	}
	previous := bundle.Language()
	defer bundle.SetLanguage(previous)
	for _, tc := range []struct{ language, want string }{
		{i18n.English, "The preview source is different or cannot be verified."},
		{i18n.Russian, "Источник прослушивания отличается или не может быть проверен."},
	} {
		bundle.SetLanguage(tc.language)
		if got := localizeAudioSeekError(audio.ErrPreviewSeekSourceChanged); got == nil || got.Error() != tc.want {
			t.Errorf("%s: got %v, want %q", tc.language, got, tc.want)
		}
	}
	other := errors.New("independent transport failure")
	if localizeAudioSeekError(nil) != nil || localizeAudioSeekError(other) != other {
		t.Fatal("localization changed an unrelated error")
	}
}

func TestAudioSeekSourceErrorLocalizedThroughEditor(t *testing.T) {
	bundle := i18n.Default()
	if err := bundle.LoadOnce("audio-seek-source-error-editor-test", os.DirFS("../res"), "i18n", ""); err != nil {
		t.Fatal(err)
	}
	previous := bundle.Language()
	defer bundle.SetLanguage(previous)
	for _, tc := range []struct{ language, want string }{
		{i18n.English, "The preview source is different or cannot be verified."},
		{i18n.Russian, "Источник прослушивания отличается или не может быть проверен."},
	} {
		for _, stream := range []bool{false, true} {
			name := tc.language + "/memory"
			if stream {
				name = tc.language + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				bundle.SetLanguage(tc.language)
				f := newWorkingAudioFixture(t, ".wav", stream)
				clip, err := f.module.Clip(f.ed.Loader, f.id)
				if err != nil || f.module.Preview(f.ed.Loader, f.id, false) != nil {
					t.Fatal("could not start preview", err)
				}
				if err := f.module.SetPreviewPaused(true); err != nil {
					t.Fatal(err)
				}
				f.module.Mixer().Mix(nil)
				before, live := f.module.PreviewPosition()
				if !live {
					t.Fatal("preview is not live")
				}
				displayed, art := *clip, *clip.Art
				art.Data = bytes.Clone(art.Data)
				displayed.Art = &art
				v := ClipView{ID: f.id, source: &displayed, Rate: clip.Art.Rate, Frames: clip.Art.Frames, Playing: true}
				// A mismatching header is rejected before any verification step.
				art.Rate++
				if got := f.plugin.seekAudioPreview(f.module, v, 100); got == nil || got.Error() != tc.want {
					t.Fatalf("synchronous source error: got %v, want %q", got, tc.want)
				}
				art.Rate--
				// A mismatch after the first chunk is detected on a later engine
				// frame and surfaced by the actual document/Inspector view.
				art.Data[len(art.Data)-1] ^= 1
				if err := f.plugin.seekAudioPreview(f.module, v, 100); err != nil || !f.module.PreviewStreamSeekPending() {
					t.Fatalf("asynchronous source verification did not start: %v", err)
				}
				deadline := time.Now().Add(10 * time.Second)
				for f.module.PreviewStreamSeekPending() && time.Now().Before(deadline) {
					if !f.ed.Engine.Step(demos.Base{}) {
						t.Fatal("headless engine stopped", f.ed.Engine.Err())
					}
					runtime.Gosched()
				}
				if f.module.PreviewStreamSeekPending() {
					t.Fatal("asynchronous source verification did not finish")
				}
				state := &input.State{}
				state.BeginFrame()
				ui := f.ed.UI
				ui.Begin(state, mathx.Vec2{X: 320, Y: 600}, 1)
				ui.Region(mathx.Rect{W: 320, H: 600})
				f.plugin.clipInspectorView(f.ed, f.id, false, mathx.Rect{})
				ui.PopCursor()
				ui.End()
				if notice, ok := f.ed.LastNotice(); !ok || notice.Level != context.LevelError || notice.Text != tc.want {
					t.Fatalf("asynchronous source notice: got %+v/%v, want %q", notice, ok, tc.want)
				}
				if after, active := f.module.PreviewPosition(); !active || before != after || !f.module.PreviewPaused() {
					t.Fatal("rejected seek moved the paused preview")
				}
				f.checkFiles(t, f.source, f.meta)
			})
		}
	}
}

func TestAudioChangedCachedPreviewLocalizedThroughTransport(t *testing.T) {
	bundle := i18n.Default()
	if err := bundle.LoadOnce("audio-preview-source-error-editor-test", os.DirFS("../res"), "i18n", ""); err != nil {
		t.Fatal(err)
	}
	previous := bundle.Language()
	defer bundle.SetLanguage(previous)
	for _, tc := range []struct{ language, want string }{
		{i18n.English, "The preview source is different or cannot be verified."},
		{i18n.Russian, "Источник прослушивания отличается или не может быть проверен."},
	} {
		t.Run(tc.language, func(t *testing.T) {
			bundle.SetLanguage(tc.language)
			f := newWorkingAudioFixture(t, ".wav", false)
			if err := f.ed.UndoAsset(f.doc); err != nil || f.doc.Dirty() {
				t.Fatal("fixture metadata must be clean", err)
			}
			clip, err := f.module.Clip(f.ed.Loader, f.id)
			if err != nil || clip.Mem == nil {
				t.Fatal("could not cache the memory clip", err)
			}
			if err := f.module.Preview(f.ed.Loader, asset.BuiltinMelody, false); err != nil {
				t.Fatal("could not start an independent preview", err)
			}
			f.module.Mixer().Mix(nil)
			epoch := f.module.PreviewEpoch()
			clip.Art.Data[len(clip.Art.Data)-1] ^= 1
			ui, state := f.ed.UI, &input.State{}
			draw := func() {
				ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
				ui.Region(mathx.Rect{W: 320, H: 180})
				f.plugin.transport(f.ed, f.module, ClipView{ID: f.id})
				ui.PopCursor()
				ui.End()
			}
			at := mathx.Vec2{X: 16, Y: ui.Theme.Padding + ui.Theme.RowHeight/2}
			state.BeginFrame()
			state.ApplyMouseDown(at, input.MouseLeft)
			draw()
			state.BeginFrame()
			state.ApplyMouseUp(at, input.MouseLeft)
			draw()
			if notice, ok := f.ed.LastNotice(); !ok || notice.Level != context.LevelError || notice.Text != tc.want {
				t.Fatalf("changed source notice: got %+v/%v, want %q", notice, ok, tc.want)
			}
			if f.module.PreviewClip() != asset.BuiltinMelody || f.module.PreviewEpoch() != epoch {
				t.Fatal("rejected Play disturbed the previous preview")
			}
			f.checkFiles(t, f.source, f.meta)
		})
	}
}

func TestAudioResampledFrameBoundsAndRounding(t *testing.T) {
	for _, tc := range []struct {
		frame    int64
		from, to int
		want     int64
		ok       bool
	}{
		{0, 44100, 48000, 0, true},
		{44100, 44100, 48000, 48000, true},
		{48000, 48000, 44100, 44100, true},
		{1, 4, 2, 1, true},
		{1, 3, 1, 0, true},
		{1 << 53, 48000, 48000, 1 << 53, true},
		{1 << 53, 44100, 48000, 0, false},
		{-1, 48000, 48000, 0, false},
		{1, 0, 48000, 0, false},
		{1, 48000, 0, 0, false},
	} {
		got, ok := audioResampledFrame(tc.frame, tc.from, tc.to)
		if got != tc.want || ok != tc.ok {
			t.Errorf("frame=%d %d->%d: got %d/%v, want %d/%v", tc.frame, tc.from, tc.to, got, ok, tc.want, tc.ok)
		}
	}
}

func TestAudioRulerClickSeeksPreviewWithoutChangingDocument(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, working := range []bool{false, true} {
			name := "memory/saved"
			if stream {
				name = "stream/saved"
			}
			if working {
				name += "/working"
			}
			t.Run(name, func(t *testing.T) {
				f := newWorkingAudioFixture(t, ".wav", stream)
				clip, err := f.module.Clip(f.ed.Loader, f.id)
				if err != nil {
					t.Fatal(err)
				}
				if working {
					if err := f.plugin.prepareWorkingAudio(f.ed, f.module, f.doc, false); err != nil {
						t.Fatal(err)
					}
					waitWorkingAudio(t, f)
				} else if err := f.module.Preview(f.ed.Loader, f.id, false); err != nil {
					t.Fatal(err)
				}
				mx := f.module.Mixer()
				game := f.module.Play(&mix.Memory{PCM: make([]float32, 96000), Rate: 48000, Channels: 1}, "sfx", audio.PlayOptions{Loop: true})
				if !game.Valid() || f.module.SetPreviewPaused(true) != nil {
					t.Fatal("could not prepare preview and game voice")
				}
				mx.Mix(nil)
				journal, revision := bytes.Clone(f.doc.Metadata.Code.Bytes()), f.doc.Metadata.Code.Rev()
				f.plugin.clip.selectAnchor, f.plugin.clip.selectEnd = 100, 200
				v := ClipView{ID: f.id, source: clip, Rate: 48000, Frames: 48000,
					WaveStart: 0, WaveEnd: 48000, WaveNav: true, Playing: true, Paused: true}
				state := &input.State{}
				var ruler mathx.Rect
				draw := func() {
					ui := f.ed.UI
					ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
					ui.Region(mathx.Rect{W: 320, H: 180})
					ruler = drawAudioRuler(ui, v)
					f.plugin.seekRuler(ui, f.module, v, ruler)
					ui.PopCursor()
					ui.End()
				}
				state.BeginFrame()
				draw()
				at := mathx.Vec2{X: ruler.X + ruler.W/2, Y: ruler.Y + ruler.H/2}
				state.BeginFrame()
				state.ApplyMouseDown(at, input.MouseLeft)
				draw()
				state.BeginFrame()
				state.ApplyMouseUp(at, input.MouseLeft)
				draw()
				deadline := time.Now().Add(10 * time.Second)
				for f.module.PreviewStreamSeekPending() && time.Now().Before(deadline) {
					if !f.ed.Engine.Step(demos.Base{}) {
						t.Fatal("headless engine stopped", f.ed.Engine.Err())
					}
					runtime.Gosched()
				}
				if f.module.PreviewStreamSeekPending() || f.module.TakePreviewStreamSeekError() != nil {
					t.Fatal("guarded preview seek did not complete")
				}
				mx.Mix(nil)
				pos, live := f.module.PreviewPosition()
				if !live || !f.module.PreviewPaused() || math.Abs(float64(pos)-.5) > 1e-5 || !mx.Live(game) {
					t.Fatalf("ruler seek = %v/%v, paused=%v, game=%v", pos, live, f.module.PreviewPaused(), mx.Live(game))
				}
				if f.plugin.clip.selectAnchor != 100 || f.plugin.clip.selectEnd != 200 || !f.doc.Dirty() ||
					f.doc.Metadata.Code.Rev() != revision || !bytes.Equal(f.doc.Metadata.Code.Bytes(), journal) {
					t.Fatal("ruler seek changed selection or shared document history")
				}
				f.checkFiles(t, f.source, f.meta)
			})
		}
	}
}

func TestAudioSeekRejectsChangedPreviewSource(t *testing.T) {
	f := newWorkingAudioFixture(t, ".wav", false)
	clip, err := f.module.Clip(f.ed.Loader, f.id)
	if err != nil || f.module.Preview(f.ed.Loader, f.id, false) != nil {
		t.Fatal("could not start preview", err)
	}
	other := *clip
	art := *clip.Art
	art.Data = bytes.Clone(art.Data)
	art.Data[len(art.Data)-1] ^= 1
	other.Art = &art
	v := ClipView{ID: f.id, source: &other, Rate: 48000, Frames: 48000, Playing: true}
	if f.module.SetPreviewPaused(true) != nil {
		t.Fatal("could not pause preview")
	}
	f.module.Mixer().Mix(nil)
	before, live := f.module.PreviewPosition()
	if !live {
		t.Fatal("preview is not live")
	}
	err = f.plugin.seekAudioPreview(f.module, v, 24000)
	deadline := time.Now().Add(10 * time.Second)
	for err == nil && f.module.PreviewStreamSeekPending() && time.Now().Before(deadline) {
		if !f.ed.Engine.Step(demos.Base{}) {
			t.Fatal("headless engine stopped", f.ed.Engine.Err())
		}
		runtime.Gosched()
	}
	if err == nil {
		err = f.module.TakePreviewStreamSeekError()
	}
	f.module.Mixer().Mix(nil)
	if after, active := f.module.PreviewPosition(); err == nil || f.module.PreviewStreamSeekPending() || !active || before != after {
		t.Fatalf("a stale waveform moved the active preview: error=%v, position=%v/%v", err, before, after)
	}
	f.checkFiles(t, f.source, f.meta)
}

func TestAudioRulerSeeksLongVorbisPreviewAt100Seconds(t *testing.T) {
	// The tracked licensed fixture is only decoded; the headless engine never
	// opens a physical sound device. The project and sidecars are temporary.
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "platformer", "assets", "audio", "glorious-morning.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	f := newWorkingAudioFixtureWithSource(t, ".ogg", true, source)
	clip, err := f.module.Clip(f.ed.Loader, f.id)
	if err != nil || clip == nil || clip.Art == nil || clip.Art.Rate != 44100 || clip.Art.Channels != 2 || clip.Art.Frames <= 101*44100 {
		t.Fatalf("long Vorbis fixture: clip=%v err=%v", clip != nil, err)
	}
	if err := f.module.Preview(f.ed.Loader, f.id, false); err != nil {
		t.Fatal(err)
	}
	if err := f.module.SetPreviewPaused(true); err != nil {
		t.Fatal(err)
	}
	mx := f.module.Mixer()
	mx.Mix(nil)
	journal, revision := bytes.Clone(f.doc.Metadata.Code.Bytes()), f.doc.Metadata.Code.Rev()
	v := ClipView{ID: f.id, source: clip, Rate: clip.Art.Rate, Frames: clip.Art.Frames,
		WaveStart: 99 * 44100, WaveEnd: 101 * 44100, WaveNav: true, Playing: true, Paused: true}
	state := &input.State{}
	var ruler mathx.Rect
	draw := func() {
		ui := f.ed.UI
		ui.Begin(state, mathx.Vec2{X: 320, Y: 180}, 1)
		ui.Region(mathx.Rect{W: 320, H: 180})
		ruler = drawAudioRuler(ui, v)
		f.plugin.seekRuler(ui, f.module, v, ruler)
		ui.PopCursor()
		ui.End()
	}
	state.BeginFrame()
	draw()
	at := mathx.Vec2{X: ruler.X + ruler.W/2, Y: ruler.Y + ruler.H/2}
	state.BeginFrame()
	state.ApplyMouseDown(at, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(at, input.MouseLeft)
	draw()
	if !f.module.PreviewStreamSeekPending() {
		t.Fatal("ruler did not start the stream seek")
	}
	deadline := time.Now().Add(20 * time.Second)
	for f.module.PreviewStreamSeekPending() && time.Now().Before(deadline) {
		if !f.ed.Engine.Step(demos.Base{}) {
			t.Fatal("headless engine stopped", f.ed.Engine.Err())
		}
		time.Sleep(time.Millisecond)
	}
	if f.module.PreviewStreamSeekPending() {
		t.Fatal("long Vorbis seek did not complete")
	}
	if err := f.module.TakePreviewStreamSeekError(); err != nil {
		t.Fatal(err)
	}
	if position, live := f.module.PreviewPosition(); !live || !f.module.PreviewPaused() || math.Abs(float64(position)-100) > 1e-4 {
		t.Fatalf("seek position = %v live=%v paused=%v", position, live, f.module.PreviewPaused())
	}

	// Independently decode the source sequentially; do not use SeekFrame for
	// the reference. Retain the preceding sample needed by interpolation.
	d, err := codec.Open(bytes.NewReader(source), ".ogg")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	const target = 100 * 44100
	buf := make([]float32, 4096*2)
	for pos := 0; pos < target-1; {
		n, err := d.Read(buf[:min(4096, target-1-pos)*2])
		if err != nil || n <= 0 {
			t.Fatalf("sequential decode at %d: %d, %v", pos, n, err)
		}
		pos += n
	}
	pcm := make([]float32, 128*2)
	for pos := 0; pos < 128; {
		n, err := d.Read(pcm[pos*2:])
		if err != nil || n <= 0 {
			t.Fatalf("reference window at %d: %d, %v", pos, n, err)
		}
		pos += n
	}
	ref, err := mix.New(mix.Config{SampleRate: mx.SampleRate(), Channels: 2, Voices: audio.Voices, Buses: audio.BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	wantVoice := ref.Play(mix.PlayParams{Memory: &mix.Memory{PCM: pcm, Rate: clip.Art.Rate, Channels: 2},
		Bus: int(audio.BusUI), Gain: clip.Gain})
	if !wantVoice.Valid() || !ref.SeekMemory(wantVoice, 1) || f.module.SetPreviewPaused(false) != nil {
		t.Fatal("could not resume the preview and prepare a reference")
	}
	got, want := make([]float32, 64), make([]float32, 64)
	mx.Mix(got)
	ref.Mix(want)
	for i := range got {
		if diff := math.Abs(float64(got[i] - want[i])); diff > 1e-3 {
			t.Fatalf("sample %d = %v, sequential reference %v (diff %v)", i, got[i], want[i], diff)
		}
	}
	if !f.doc.Dirty() || f.doc.Metadata.Code.Rev() != revision || !bytes.Equal(f.doc.Metadata.Code.Bytes(), journal) {
		t.Fatal("long Vorbis seek changed shared document history")
	}
	f.checkFiles(t, f.source, f.meta)
}
