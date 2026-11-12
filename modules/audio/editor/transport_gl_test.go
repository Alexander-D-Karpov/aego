//go:build aego_gltest

package editor

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/platform/input"
	"aego/render"
	"aego/render/text"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/theme"
	"aego/ui/widgets"
)

// Exercise the real localised controls and font with the normal offscreen GPU
// path. This does not claim physical display or audio-device acceptance.
func TestAudioTransportRenderedLayout(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	bundle := i18n.Default()
	if err := bundle.LoadOnce("audio-transport-gl-test", os.DirFS("../res"), "i18n", ""); err != nil {
		t.Fatal(err)
	}
	oldLanguage := bundle.Language()
	defer bundle.SetLanguage(oldLanguage)
	const width, height = 360, 320
	e, err := engine.NewBuilder(engine.Config{Title: "audio-transport-test", Width: width, Height: height, Hidden: true,
		ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "audio-transport-test"}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Caps().Renderer == "null" {
		t.Fatal("transport layout test requires a graphics context")
	}
	t.Logf("renderer: %s", e.Caps().Renderer)
	r, err := render.New(e.GPU(), e.Caps(), e.VFS(), log.Nop(), render.Options{MaxSprites: 4096, MaxMeshes: 4, MaxSprites3D: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fonts, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2})
	if err != nil {
		t.Fatal("editor fonts are unavailable", err)
	}
	target, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: width, Height: height, Format: gpu.TexSRGBA8, Label: "audio.transport.test"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.GPU().DestroyTexture(target)
	for _, language := range []string{i18n.English, i18n.Russian} {
		bundle.SetLanguage(language)
		if language == i18n.Russian && previewPlayLabel(ClipView{}) != "Воспроизвести" {
			t.Fatal("Russian audio catalog was not loaded")
		}
		if language == i18n.Russian && (previewPlayLabel(ClipView{Preparing: true}) != "Отмена" ||
			i18n.Tr("editor.audio.preview_preparing", "Preparing preview...") != "Подготовка прослушивания...") {
			t.Fatal("Russian working-preview controls were not translated")
		}
		for _, tc := range []struct {
			name  string
			width float32
			scale float32
			light bool
		}{
			{"wide-dark", 320, 1, false}, {"wide-light", 320, 1, true},
			{"narrow", 132, 1, false}, {"hidpi", 180, 1.5, false},
		} {
			for _, state := range []struct {
				name                      string
				playing, pause, preparing bool
			}{{"play", false, false, false}, {"pause", true, false, false}, {"resume", true, true, false}, {"preparing", false, false, true}} {
				func() {
					caseName := fmt.Sprintf("%s/%s/%s", language, tc.name, state.name)
					ui := core.New(nil)
					if tc.light {
						ui.Theme = theme.Light()
					} else {
						ui.Theme = theme.Editor()
					}
					ui.SetFonts(fonts)
					inputState := &input.State{}
					inputState.BeginFrame()
					ui.Begin(inputState, mathx.Vec2{X: width / tc.scale, Y: height / tc.scale}, tc.scale)
					if language == i18n.Russian && (ui.Font() == nil || !ui.Font().Has('В') || !ui.Font().Has('ы')) {
						t.Fatal("selected editor font is missing Cyrillic glyphs")
					}
					ui.Region(mathx.Rect{W: tc.width, H: height / tc.scale})
					ui.PushClip(mathx.Rect{W: tc.width, H: height / tc.scale})
					v := ClipView{Playing: state.playing, Paused: state.pause, Preparing: state.preparing, Position: 100, Seconds: 110.7}
					p := &plugin{}
					if state.preparing {
						p.workingAudio.sequence = 1
						p.workingAudio.running = &workingAudioRequest{sequence: 1, id: v.ID, current: func() bool { return true }, cancel: func() {}}
						p.workingAudio.result = make(chan workingAudioResult)
					}
					p.transport(&context.Editor{UI: ui}, nil, v)
					ui.PopClip()
					ui.PopCursor()
					list := ui.End()
					labels := []string{previewPlayLabel(v), i18n.Tr("i18n.audio.stop", "Stop"), i18n.Tr("i18n.audio.loop", "Loop")}
					bounds := make([]mathx.Rect, 0, len(labels))
					for _, label := range labels {
						items := list.TestDrawnItems(mathx.Rect{W: width / tc.scale, H: height / tc.scale}, ui.Font(), label, 0, 0)
						var item draw.TestDrawnItem
						found := 0
						for _, candidate := range items {
							if candidate.Text == label {
								item, found = candidate, found+1
							}
						}
						if found != 1 || item.Bounds.X < item.Clip.X || item.Bounds.X+item.Bounds.W > item.Clip.X+item.Clip.W+.01 ||
							item.Bounds.Y < item.Clip.Y || item.Bounds.Y+item.Bounds.H > item.Clip.Y+item.Clip.H+.01 {
							t.Fatalf("%s: transport label %q missing or clipped: found=%d %+v", caseName, label, found, item)
						}
						for _, prev := range bounds {
							if item.Bounds.X < prev.X+prev.W && item.Bounds.X+item.Bounds.W > prev.X &&
								item.Bounds.Y < prev.Y+prev.H && item.Bounds.Y+item.Bounds.H > prev.Y {
								t.Fatalf("%s: transport labels overlap: %+v and %+v", caseName, prev, item.Bounds)
							}
						}
						bounds = append(bounds, item.Bounds)
					}
					position := i18n.Tr("i18n.audio.position_s_of_seconds_s", "{position} s of {seconds} s",
						i18n.Str("position", seconds(v.Position)), i18n.Str("seconds", seconds(v.Seconds)))
					if state.preparing {
						position = i18n.Tr("editor.audio.preview_preparing", "Preparing preview...")
					}
					for _, line := range widgets.WrapLines(ui, position, tc.width-2*ui.Theme.Padding) {
						items := list.TestDrawnItems(mathx.Rect{W: width / tc.scale, H: height / tc.scale}, ui.Font(), line, 0, 0)
						found := false
						for _, item := range items {
							if item.Text == line {
								found = true
								if item.Bounds.X < item.Clip.X || item.Bounds.X+item.Bounds.W > item.Clip.X+item.Clip.W+.01 ||
									item.Bounds.Y < item.Clip.Y || item.Bounds.Y+item.Bounds.H > item.Clip.Y+item.Clip.H+.01 {
									t.Fatalf("position label clipped: %+v", item)
								}
							}
						}
						if !found {
							t.Fatalf("position line %q was not drawn", line)
						}
					}
					if err := r.BeginFrame(e.GPU().BeginFrame(width, height), width, height, tc.scale); err != nil {
						t.Fatal(err)
					}
					screen := view.ScreenView(width, height, tc.scale)
					screen.Target, screen.Clear, screen.DoClear = target, ui.Theme.Background, true
					frame := r.View(screen)
					list.Flush(frame, ui.Font(), r.White(), 0)
					frame.End()
					r.EndFrame()
					e.GPU().EndFrame()
					if r.Stats().Dropped != 0 {
						t.Fatal("renderer dropped transport sprites")
					}
					pixels := make([]byte, width*height*4)
					if err := e.GPU().ReadPixels(target, 0, 0, width, height, pixels); err != nil {
						t.Fatal(err)
					}
					different := 0
					for i := 4; i < len(pixels); i += 4 {
						if pixels[i] != pixels[0] || pixels[i+1] != pixels[1] || pixels[i+2] != pixels[2] {
							different++
						}
					}
					if different < 10 {
						t.Fatalf("%s: transport rendered no visible controls", caseName)
					}
					if out := os.Getenv("AEGO_AUDIO_TRANSPORT_SHOTS"); out != "" {
						name := fmt.Sprintf("%s-%s-%s.png", language, tc.name, state.name)
						f, err := os.OpenFile(filepath.Join(out, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
						if err != nil {
							t.Fatal(err)
						}
						img := &image.NRGBA{Pix: pixels, Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
						if err := png.Encode(f, img); err != nil {
							f.Close()
							t.Fatal(err)
						}
						if err := f.Close(); err != nil {
							t.Fatal(err)
						}
					}
				}()
			}
		}
	}
}
