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

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine"
	"aego/gpu"
	"aego/platform/input"
	"aego/render"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/theme"
)

// Synthetic UI rendered through the normal GPU path. This is not a physical
// display or audio-device test and does not update any golden image.
func TestStereoWaveRenderedPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const width, height = 360, 120
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
			draw := func(left, right float32, mono bool) []byte {
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
				} else {
					v.StereoPeaks = make([]float32, peakCols*2)
					for i := range peakCols {
						v.StereoPeaks[i*2], v.StereoPeaks[i*2+1] = left, right
					}
				}
				p.wave(ed, v)
				ui.PopClip()
				ui.PopCursor()
				list := ui.End()
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
			stereo := draw(.9, .1, false)
			swapped := draw(.1, .9, false)
			before := draw(.9, .1, true)
			if bytes.Equal(stereo, swapped) || bytes.Equal(stereo, before) {
				t.Fatalf("%s: channel swap or old mono view produced identical pixels", tc.name)
			}
			if out := os.Getenv("AEGO_AUDIO_WAVE_SHOTS"); out != "" {
				for name, pixels := range map[string][]byte{tc.name + "-stereo.png": stereo, tc.name + "-mono.png": before} {
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
