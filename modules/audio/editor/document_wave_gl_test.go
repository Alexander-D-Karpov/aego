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

	"aego/asset/pipeline"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine"
	"aego/engine/i18n"
	"aego/format/aetext"
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

// Full document layout on a synthetic offscreen GPU. No physical audio device
// or user asset is involved, and this test does not update golden images.
func TestAudioDocumentWaveLayout(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	bundle := i18n.Default()
	if err := bundle.LoadOnce("audio-document-wave-gl-test", os.DirFS("../res"), "i18n", ""); err != nil {
		t.Fatal(err)
	}
	oldLanguage := bundle.Language()
	defer bundle.SetLanguage(oldLanguage)
	const width, height = 1000, 700
	e, err := engine.NewBuilder(engine.Config{Title: "audio-document-wave", Width: width, Height: height, Hidden: true,
		ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "audio-document-wave"}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Caps().Renderer == "null" {
		t.Fatal("document layout test requires a graphics context")
	}
	t.Logf("renderer: %s", e.Caps().Renderer)
	r, err := render.New(e.GPU(), e.Caps(), e.VFS(), log.Nop(), render.Options{MaxSprites: 4096, MaxMeshes: 4, MaxSprites3D: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fonts, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	target, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: width, Height: height, Format: gpu.TexSRGBA8, Label: "audio.document.wave.test"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.GPU().DestroyTexture(target)
	for _, language := range []string{i18n.English, i18n.Russian} {
		bundle.SetLanguage(language)
		for _, tc := range []struct {
			name          string
			width, height float32
			scale, minGap float32
			light         bool
		}{
			{"wide-dark", 800, 600, 1, 100, false},
			{"wide-light", 800, 600, 1, 100, true},
			{"narrow", 320, 540, 1, 70, false},
			{"small", 180, 320, 1, 30, false},
			{"hidpi", 480, 420, 1.5, 30, false},
		} {
			func() {
				caseName := fmt.Sprintf("%s/%s", language, tc.name)
				ui := core.New(nil)
				if tc.light {
					ui.Theme = theme.Light()
				} else {
					ui.Theme = theme.Editor()
				}
				ui.SetFonts(fonts)
				state := &input.State{}
				area := mathx.Rect{W: tc.width, H: tc.height}
				v := ClipView{Rate: 48000, Channels: 2, Frames: 5313600, Seconds: 110.7, PeaksDone: true,
					WaveStart: 99 * 48000, WaveEnd: 103 * 48000, WaveNav: true, Playing: true, Position: 101,
					Settings: pipeline.Settings{"loopStart": aetext.Int(100 * 48000), "loopEnd": aetext.Int(102 * 48000)}}
				v.StereoPeaks = make([]float32, peakCols*2)
				for i := range peakCols {
					v.StereoPeaks[2*i], v.StereoPeaks[2*i+1] = .8, .3
				}
				p := &plugin{clip: clipEdit{drag: -1}}
				drawDocument := func() *draw.List {
					ui.Begin(state, mathx.Vec2{X: width / tc.scale, Y: height / tc.scale}, tc.scale)
					ui.Region(area)
					ui.PushClip(area)
					p.drawClipDocument(&context.Editor{UI: ui}, nil, v, area)
					ui.PopClip()
					ui.PopCursor()
					return ui.End()
				}
				state.BeginFrame()
				drawDocument() // Measure content before deciding whether a scrollbar is needed.
				state.BeginFrame()
				list := drawDocument()
				find := func(list *draw.List, label string) mathx.Rect {
					t.Helper()
					var found mathx.Rect
					count := 0
					for _, item := range list.TestDrawnItems(area, ui.Font(), label, 0, 0) {
						if item.Text != label {
							continue
						}
						count++
						found = item.Bounds
						if item.Bounds.X < item.Clip.X || item.Bounds.X+item.Bounds.W > item.Clip.X+item.Clip.W+.01 ||
							item.Bounds.Y < item.Clip.Y || item.Bounds.Y+item.Bounds.H > item.Clip.Y+item.Clip.H+.01 {
							t.Fatalf("%s: %q is clipped: %+v", caseName, label, item)
						}
					}
					if count != 1 {
						t.Fatalf("%s: expected one visible %q label, found %d", caseName, label, count)
					}
					return found
				}
				left, right := find(list, "L"), find(list, "R")
				if right.Y-left.Y < tc.minGap {
					t.Fatalf("%s: document waveform is too short: channel gap %.1f < %.1f", caseName, right.Y-left.Y, tc.minGap)
				}
				waveWidth := tc.width - 2*ui.Theme.Padding
				if p.clip.docContent > tc.height-ui.Theme.Padding {
					waveWidth -= ui.Theme.ScrollbarW
				}
				summary := fmt.Sprintf("%s, %d Hz, %s, %s s", formatName(v.Format), v.Rate, channels(v.Channels), seconds(v.Seconds))
				for _, line := range widgets.WrapLines(ui, summary, min(waveWidth, 900)) {
					find(list, line)
				}
				for _, label := range []string{previewPlayLabel(v), i18n.Tr("i18n.audio.stop", "Stop"), i18n.Tr("i18n.audio.loop", "Loop")} {
					find(list, label)
				}
				if tc.name == "small" && p.clip.docContent <= tc.height-ui.Theme.Padding {
					t.Fatalf("%s: fixture did not exercise the scrolling path", caseName)
				}
				if p.clip.docContent > tc.height-ui.Theme.Padding {
					state.BeginFrame()
					state.ApplyWheel(mathx.Vec2{X: tc.width / 2, Y: tc.height / 2}, mathx.Vec2{Y: -100})
					list = drawDocument()
					if p.clip.docScroll <= 0 {
						t.Fatalf("%s: overflowing document did not scroll", caseName)
					}
					for _, label := range []string{i18n.Tr("i18n.audio.gain", "Gain"), i18n.Tr("i18n.audio.bus", "Bus"), "sfx"} {
						find(list, label)
					}
					state.BeginFrame()
					state.ApplyWheel(mathx.Vec2{X: tc.width / 2, Y: tc.height / 2}, mathx.Vec2{Y: 100})
					list = drawDocument()
					if p.clip.docScroll != 0 {
						t.Fatalf("%s: document could not return to the top", caseName)
					}
					find(list, previewPlayLabel(v))
				} else {
					for _, label := range []string{i18n.Tr("i18n.audio.import", "Import"), i18n.Tr("i18n.audio.gain", "Gain"), i18n.Tr("i18n.audio.bus", "Bus"), "sfx"} {
						find(list, label)
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
					t.Fatal("renderer dropped audio document sprites")
				}
				if out := os.Getenv("AEGO_AUDIO_DOCUMENT_SHOTS"); out != "" {
					pixels := make([]byte, width*height*4)
					if err := e.GPU().ReadPixels(target, 0, 0, width, height, pixels); err != nil {
						t.Fatal(err)
					}
					name := filepath.Join(out, fmt.Sprintf("%s-%s.png", language, tc.name))
					f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
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
