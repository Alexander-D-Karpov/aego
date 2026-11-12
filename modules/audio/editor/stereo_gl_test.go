//go:build aego_gltest

package editor

import (
	"bytes"
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
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/theme"
	"aego/ui/widgets"
)

// Synthetic UI rendered through the normal GPU path. This is not a physical
// display or audio-device test and does not update any golden image.
func TestStereoWaveRenderedPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const width, height = 360, 320
	e, err := engine.NewBuilder(engine.Config{Title: "audio-wave-test", Width: width, Height: height, Hidden: true,
		ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "audio-wave-test"}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Caps().Renderer == "null" {
		t.Fatal("stereo waveform test requires a graphics context")
	}
	t.Logf("renderer: %s", e.Caps().Renderer)
	r, err := render.New(e.GPU(), e.Caps(), e.VFS(), log.Nop(), render.Options{MaxSprites: 4096, MaxMeshes: 4, MaxSprites3D: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	target, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: width, Height: height, Format: gpu.TexSRGBA8, Label: "audio.wave.test"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.GPU().DestroyTexture(target)
	for _, tc := range []struct {
		name  string
		width float32
		light bool
		scale float32
	}{{"dark", 320, false, 1}, {"light", 320, true, 1}, {"narrow", 132, false, 1}, {"hidpi", 180, false, 1.5}} {
		func() {
			ui := core.New(nil)
			if tc.light {
				ui.Theme = theme.Light()
			} else {
				ui.Theme = theme.Editor()
			}
			ui.SetFont(r.UIFont())
			state := &input.State{}
			p := &plugin{clip: clipEdit{drag: -1}}
			ed := &context.Editor{UI: ui}
			var waveRect mathx.Rect
			draw := func(left, right float32, mono, zoom, selected bool) []byte {
				p.clip.selectAnchor, p.clip.selectEnd = 0, 0
				if selected {
					p.clip.selectAnchor, p.clip.selectEnd = 100*48000+24000, 101*48000+24000
					if mono && !zoom {
						p.clip.selectAnchor, p.clip.selectEnd = 12000, 36000
					}
				}
				state.BeginFrame()
				ui.Begin(state, mathx.Vec2{X: width / tc.scale, Y: height / tc.scale}, tc.scale)
				ui.Region(mathx.Rect{W: tc.width, H: height / tc.scale})
				ui.PushClip(mathx.Rect{W: tc.width, H: height / tc.scale})
				v := ClipView{Frames: 48000, PeaksDone: true}
				if mono {
					v.Peaks = make([]float32, peakCols)
					for i := range v.Peaks {
						v.Peaks[i] = max(left, right)
					}
					if selected && !zoom {
						v.Rate, v.WaveSelect = 48000, true
					}
				} else {
					v.StereoPeaks = make([]float32, peakCols*2)
					for i := range peakCols {
						v.StereoPeaks[i*2], v.StereoPeaks[i*2+1] = left, right
					}
				}
				if zoom {
					v.Frames, v.Rate = 5313600, 48000
					v.WaveStart, v.WaveEnd, v.WaveNav = 99*48000, 103*48000, true
					v.Playing, v.Position = true, 101
					v.Settings = pipeline.Settings{"loopStart": aetext.Int(100 * 48000), "loopEnd": aetext.Int(102 * 48000)}
					p.waveNavigation(ed, v)
					drawAudioRuler(ui, v)
				}
				cur := ui.Cursor()
				waveRect = mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: waveH}
				p.wave(ed, v)
				if selected {
					p.waveSelectionLabel(ui, v)
				}
				ui.PopClip()
				ui.PopCursor()
				list := ui.End()
				if selected && mono && !zoom {
					start, end, ok := p.clip.waveSelection(v.Frames)
					if !ok {
						t.Fatalf("%s: missing mono selection", tc.name)
					}
					for _, line := range widgets.WrapLines(ui, formatWaveSelection(v, start, end), tc.width-2*ui.Theme.Padding) {
						items := list.TestDrawnItems(mathx.Rect{W: width / tc.scale, H: height / tc.scale}, ui.Font(), line, 0, 0)
						found := false
						for _, item := range items {
							if item.Text != line {
								continue
							}
							found = true
							if item.Bounds.X < item.Clip.X || item.Bounds.X+item.Bounds.W > item.Clip.X+item.Clip.W+.01 ||
								item.Bounds.Y < item.Clip.Y || item.Bounds.Y+item.Bounds.H > item.Clip.Y+item.Clip.H+.01 {
								t.Fatalf("%s: mono selection label is clipped: %+v", tc.name, item)
							}
						}
						if !found {
							t.Fatalf("%s: mono selection line %q was not drawn", tc.name, line)
						}
					}
				}
				if zoom {
					label := i18n.Tr("i18n.audio.wave_visible", "Visible: {start} - {end} s",
						i18n.Str("start", seconds(float32(v.WaveStart)/float32(v.Rate))),
						i18n.Str("end", seconds(float32(v.WaveEnd)/float32(v.Rate))))
					lines := widgets.WrapLines(ui, label, tc.width-2*ui.Theme.Padding)
					for _, line := range lines {
						items := list.TestDrawnItems(mathx.Rect{W: width / tc.scale, H: height / tc.scale}, ui.Font(), line, 0, 0)
						found := false
						for _, item := range items {
							if item.Text != line {
								continue
							}
							found = true
							if item.Bounds.X < item.Clip.X || item.Bounds.X+item.Bounds.W > item.Clip.X+item.Clip.W+.01 ||
								item.Bounds.Y < item.Clip.Y || item.Bounds.Y+item.Bounds.H > item.Clip.Y+item.Clip.H+.01 {
								t.Fatalf("%s: waveform range label is clipped: %+v", tc.name, item)
							}
						}
						if !found {
							t.Fatalf("%s: waveform range line %q was not drawn", tc.name, line)
						}
					}
					if selected {
						start, end, ok := p.clip.waveSelection(v.Frames)
						if !ok {
							t.Fatalf("%s: missing selection", tc.name)
						}
						for _, line := range widgets.WrapLines(ui, formatWaveSelection(v, start, end), tc.width-2*ui.Theme.Padding) {
							items := list.TestDrawnItems(mathx.Rect{W: width / tc.scale, H: height / tc.scale}, ui.Font(), line, 0, 0)
							found := false
							for _, item := range items {
								if item.Text != line {
									continue
								}
								found = true
								if item.Bounds.X < item.Clip.X || item.Bounds.X+item.Bounds.W > item.Clip.X+item.Clip.W+.01 ||
									item.Bounds.Y < item.Clip.Y || item.Bounds.Y+item.Bounds.H > item.Clip.Y+item.Clip.H+.01 {
									t.Fatalf("%s: selection label is clipped: %+v", tc.name, item)
								}
							}
							if !found {
								t.Fatalf("%s: selection line %q was not drawn", tc.name, line)
							}
						}
					}
					ticks, tickCount, step := audioRulerTicks(v.WaveStart, v.WaveEnd, v.Rate, tc.width-2*ui.Theme.Padding)
					if tickCount == 0 {
						t.Fatalf("%s: no ruler ticks", tc.name)
					}
					lastRight := float32(-1)
					for _, tick := range ticks[:tickCount] {
						label := audioRulerLabel(tick.Seconds, step)
						items := list.TestDrawnItems(mathx.Rect{W: width / tc.scale, H: height / tc.scale}, ui.Font(), label, 0, 0)
						found := false
						for _, item := range items {
							if item.Text != label {
								continue
							}
							found = true
							if item.Bounds.X < item.Clip.X || item.Bounds.X+item.Bounds.W > item.Clip.X+item.Clip.W+.01 ||
								item.Bounds.Y < item.Clip.Y || item.Bounds.Y+item.Bounds.H > item.Clip.Y+item.Clip.H+.01 ||
								item.Bounds.X < lastRight {
								t.Fatalf("%s: ruler label is clipped or overlaps: %+v", tc.name, item)
							}
							lastRight = item.Bounds.X + item.Bounds.W
						}
						if !found {
							t.Fatalf("%s: ruler label %q was not drawn", tc.name, label)
						}
					}
				}
				if err := r.BeginFrame(e.GPU().BeginFrame(width, height), width, height, tc.scale); err != nil {
					t.Fatal(err)
				}
				vscreen := view.ScreenView(width, height, tc.scale)
				vscreen.Target, vscreen.Clear, vscreen.DoClear = target, ui.Theme.Background, true
				frame := r.View(vscreen)
				list.Flush(frame, ui.Font(), r.White(), 0)
				frame.End()
				r.EndFrame()
				e.GPU().EndFrame()
				if r.Stats().Dropped != 0 {
					t.Fatal("renderer dropped waveform sprites")
				}
				pixels := make([]byte, width*height*4)
				if err := e.GPU().ReadPixels(target, 0, 0, width, height, pixels); err != nil {
					t.Fatal(err)
				}
				return pixels
			}
			stereo := draw(.9, .1, false, false, false)
			swapped := draw(.1, .9, false, false, false)
			before := draw(.9, .1, true, false, false)
			monoSelected := draw(.9, .1, true, false, true)
			monoRect := waveRect
			zoomed := draw(.9, .1, false, true, false)
			selected := draw(.9, .1, false, true, true)
			monoZoomed := draw(.9, .1, true, true, false)
			monoZoomedSelected := draw(.9, .1, true, true, true)
			if bytes.Equal(stereo, swapped) || bytes.Equal(stereo, before) || bytes.Equal(stereo, zoomed) {
				t.Fatalf("%s: channel swap, mono, or zoom view produced identical pixels", tc.name)
			}
			changed := 0
			for y := max(0, int(waveRect.Y*tc.scale)); y < min(height, int((waveRect.Y+waveRect.H)*tc.scale)); y++ {
				for x := max(0, int(waveRect.X*tc.scale)); x < min(width, int((waveRect.X+waveRect.W)*tc.scale)); x++ {
					i := (y*width + x) * 4
					if !bytes.Equal(zoomed[i:i+4], selected[i:i+4]) {
						changed++
					}
				}
			}
			if changed < 10 {
				t.Fatalf("%s: selection did not change the waveform pixels (%d)", tc.name, changed)
			}
			changed = 0
			for y := max(0, int(monoRect.Y*tc.scale)); y < min(height, int((monoRect.Y+monoRect.H)*tc.scale)); y++ {
				for x := max(0, int(monoRect.X*tc.scale)); x < min(width, int((monoRect.X+monoRect.W)*tc.scale)); x++ {
					i := (y*width + x) * 4
					if !bytes.Equal(before[i:i+4], monoSelected[i:i+4]) {
						changed++
					}
				}
			}
			if changed < 10 {
				t.Fatalf("%s: mono selection did not change the waveform pixels (%d)", tc.name, changed)
			}
			changed = 0
			for y := max(0, int(waveRect.Y*tc.scale)); y < min(height, int((waveRect.Y+waveRect.H)*tc.scale)); y++ {
				for x := max(0, int(waveRect.X*tc.scale)); x < min(width, int((waveRect.X+waveRect.W)*tc.scale)); x++ {
					i := (y*width + x) * 4
					if !bytes.Equal(monoZoomed[i:i+4], monoZoomedSelected[i:i+4]) {
						changed++
					}
				}
			}
			if changed < 10 {
				t.Fatalf("%s: zoomed mono selection did not change waveform pixels (%d)", tc.name, changed)
			}
			if out := os.Getenv("AEGO_AUDIO_WAVE_SHOTS"); out != "" {
				for name, pixels := range map[string][]byte{tc.name + "-stereo.png": stereo, tc.name + "-mono.png": before, tc.name + "-mono-selection.png": monoSelected, tc.name + "-zoom.png": zoomed, tc.name + "-selection.png": selected, tc.name + "-mono-zoom.png": monoZoomed, tc.name + "-mono-zoom-selection.png": monoZoomedSelected} {
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
			}
		}()
	}
}
