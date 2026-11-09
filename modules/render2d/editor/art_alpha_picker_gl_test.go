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

	"aego/art"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	"aego/render/view"
)

// Verify the actual renderer composites the checker, gradient and two-tone
// marker, without requiring or updating any golden image.
func TestArtAlphaPickerRenderedPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const width, height = 320, 400
	e, err := engine.NewBuilder(engine.Config{Title: "art-alpha-test", Width: width, Height: height, Hidden: true, ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "art-alpha-test"}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Caps().Renderer == "null" {
		t.Fatal("real GL alpha test requested, but no graphics context is available")
	}
	r, err := render.New(e.GPU(), e.Caps(), e.VFS(), log.Nop(), render.Options{MaxSprites: 4096, MaxMeshes: 4, MaxSprites3D: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	target, err := e.GPU().CreateTexture(gpu.TextureDesc{Width: width, Height: height, Format: gpu.TexSRGBA8, Label: "art.alpha.test"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.GPU().DestroyTexture(target)

	for _, tc := range []struct {
		name     string
		light    bool
		width    float32
		color    art.Color
		dominant int
		scale    float32
	}{
		{"dark", false, 240, art.Color{R: 220, G: 64, B: 22, A: 128}, 0, 1},
		{"light", true, 240, art.Color{R: 220, G: 64, B: 22, A: 128}, 0, 1},
		{"narrow-dark", false, 132, art.Color{R: 220, G: 64, B: 22, A: 128}, 0, 1},
		{"narrow-light", true, 132, art.Color{R: 220, G: 64, B: 22, A: 128}, 0, 1},
		{"very-narrow", true, 80, art.Color{R: 220, G: 64, B: 22, A: 128}, 0, 1},
		{"tiny-dark", false, 44, art.Color{R: 220, G: 64, B: 22, A: 128}, 0, 1},
		{"tiny-light", true, 44, art.Color{R: 220, G: 64, B: 22, A: 128}, 0, 1},
		{"narrow-hidpi", false, 132, art.Color{R: 220, G: 64, B: 22, A: 128}, 0, 1.5},
		{"transparent-blue", false, 240, art.Color{R: 22, G: 64, B: 220, A: 0}, 2, 1},
		{"opaque-green", true, 240, art.Color{R: 22, G: 220, B: 64, A: 255}, 1, 1},
		{"transparent-black", true, 240, art.Color{A: 0}, -1, 1},
		{"opaque-white", false, 240, art.Color{R: 255, G: 255, B: 255, A: 255}, -2, 1},
	} {
		// Subtests run on another goroutine, which does not own this GL context.
		func() {
			h := newArtAlphaHarness(t, tc.width, tc.light)
			h.p.view.color = artPickerColor(tc.color)
			h.ui.SetFont(r.UIFont())
			h.ui.Begin(h.state, mathx.Vec2{X: width / tc.scale, Y: height / tc.scale}, tc.scale)
			h.ui.Region(h.area)
			cur := h.ui.Cursor()
			cur.Next(min(float32(240), cur.W-24))
			bar, _ := artAlphaRects(h.ui)
			h.ui.PopCursor()
			h.ui.End()

			h.ui.Begin(h.state, mathx.Vec2{X: width / tc.scale, Y: height / tc.scale}, tc.scale)
			h.ui.Rect(mathx.Rect{W: width, H: height}, h.ui.Theme.Background)
			h.ui.Region(h.area)
			h.ui.PushClip(h.area)
			h.p.colorPicker(h.ui)
			h.ui.PopClip()
			h.ui.PopCursor()
			list := h.ui.End()
			if err := r.BeginFrame(e.GPU().BeginFrame(width, height), width, height, tc.scale); err != nil {
				t.Fatal(err)
			}
			v := view.ScreenView(width, height, tc.scale)
			v.Target, v.Clear, v.DoClear = target, h.ui.Theme.Background, true
			f := r.View(v)
			list.Flush(f, h.ui.Font(), r.White(), 0)
			f.End()
			r.EndFrame()
			e.GPU().EndFrame()
			if dropped := r.Stats().Dropped; dropped != 0 {
				t.Fatalf("%s: renderer dropped %d sprites", tc.name, dropped)
			}
			pixels := make([]byte, width*height*4)
			if err := e.GPU().ReadPixels(target, 0, 0, width, height, pixels); err != nil {
				t.Fatal(err)
			}
			img := &image.NRGBA{Pix: pixels, Stride: width * 4, Rect: image.Rect(0, 0, width, height)}
			pixel := func(x, y float32) [3]uint8 {
				c := img.NRGBAAt(int(x*tc.scale), int(y*tc.scale))
				return [3]uint8{c.R, c.G, c.B}
			}
			left1 := pixel(bar.X+7, bar.Y+4)
			left2 := pixel(bar.X+7, bar.Y+12)
			right := pixel(bar.X+bar.W-8, bar.Y+4)
			t.Logf("%s: bar=%+v checker=%v/%v near-opaque=%v", tc.name, bar, left1, left2, right)
			if int(left1[0])-int(left2[0]) < 20 && int(left2[0])-int(left1[0]) < 20 {
				t.Fatalf("%s: transparent end has no visible checker: %v %v", tc.name, left1, left2)
			}
			if tc.dominant >= 0 {
				for i := range right {
					if i != tc.dominant && int(right[tc.dominant]) <= int(right[i])+40 {
						t.Fatalf("%s: opaque end did not use the selected RGB: %v", tc.name, right)
					}
				}
			} else if (tc.dominant == -1 && right[0] >= left2[0]) || (tc.dominant == -2 && right[0] <= left1[0]) {
				t.Fatalf("%s: opaque end did not approach the selected grayscale: %v", tc.name, right)
			}
			// Transparency should decrease across the ramp regardless of the
			// selected alpha: the checker contrast falls towards the right.
			contrast := func(x float32) int {
				a := pixel(bar.X+bar.W*x, bar.Y+4)
				b := pixel(bar.X+bar.W*x, bar.Y+12)
				d := int(a[0]) - int(b[0])
				if d < 0 {
					d = -d
				}
				return d
			}
			if a, b := contrast(.25), contrast(.75); a <= b {
				t.Fatalf("%s: checker contrast did not decrease along alpha ramp: %d <= %d", tc.name, a, b)
			}
			black, white := false, false
			center := int((bar.X + (bar.W-4)*float32(tc.color.A)/255) * tc.scale)
			for x := max(int(bar.X*tc.scale), center-2); x <= min(int((bar.X+bar.W)*tc.scale)-1, center+int(6*tc.scale)); x++ {
				c := img.NRGBAAt(x, int((bar.Y+6)*tc.scale))
				black = black || (c.R < 25 && c.G < 25 && c.B < 25)
				white = white || (c.R > 230 && c.G > 230 && c.B > 230)
			}
			if !black || !white {
				t.Fatalf("%s: alpha marker lacks contrasting black and white pixels: black=%v white=%v bar=%+v", tc.name, black, white, bar)
			}
			if dir := os.Getenv("AEGO_ART_ALPHA_SHOTS"); dir != "" {
				path := filepath.Join(dir, fmt.Sprintf("alpha-%s.png", tc.name))
				out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(out, img); err != nil {
					out.Close()
					t.Fatal(err)
				}
				if err := out.Close(); err != nil {
					t.Fatal(err)
				}
				t.Logf("screenshot: %s", path)
			}
		}()
	}
	t.Logf("offscreen GL renderer: %s", e.Caps().Renderer)
}
