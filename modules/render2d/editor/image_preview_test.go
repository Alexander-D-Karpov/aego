package editor

import (
	"errors"
	"testing"

	"aego/asset"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/res"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render"
	"aego/render/texture"
	"aego/vfs"
)

type failingImageDevice struct {
	*null.Device
	fail bool
}

func (d *failingImageDevice) CreateTexture(desc gpu.TextureDesc) (gpu.TextureHandle, error) {
	if d.fail && desc.Label == "image.preview.normal" {
		return gpu.TextureHandle{}, errors.New("injected normal texture allocation failure")
	}
	return d.Device.CreateTexture(desc)
}

func imageTestRenderer(t *testing.T, dev gpu.Device) *render.Renderer {
	t.Helper()
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	r, err := render.New(dev, dev.Caps(), files, log.Nop(), render.Options{MaxSprites: 8, MaxMeshes: 4, MaxSprites3D: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func normalPreviewPixels(r, g, b byte) *imagePixels {
	d := texture.New(2, 2, gpu.TexRGBA8)
	d.NonColor = true
	for i := 0; i < len(d.Pixels); i += 4 {
		d.Pixels[i], d.Pixels[i+1], d.Pixels[i+2], d.Pixels[i+3] = r, g, b, 255
	}
	return &imagePixels{data: *d, semantic: asset.TextureNormal}
}

func TestImagePreviewFailedUploadKeepsWorkingNormalAndResources(t *testing.T) {
	dev := &failingImageDevice{Device: null.New()}
	defer dev.Close()
	ed := &context.Editor{Renderer: imageTestRenderer(t, dev)}
	var p imagePreview
	first := normalPreviewPixels(128, 128, 255)
	region, err := p.candidateRegion(ed, first, [4]int{0, 0, 2, 2})
	if err != nil || region.IsZero() {
		t.Fatalf("initial upload: %v", err)
	}
	baseline := dev.ResourceStats()
	dev.fail = true
	second := normalPreviewPixels(255, 128, 128)
	kept, err := p.candidateRegion(ed, second, [4]int{0, 0, 2, 2})
	if err == nil || kept != region || p.candidate != first || p.texture != region.Texture {
		t.Fatalf("failed candidate replaced last-good normal: %v %v", kept, err)
	}
	if dev.ResourceStats() != baseline {
		t.Fatal("failed upload changed GPU resource counts")
	}
	dev.fail = false
	replacement, err := p.candidateRegion(ed, second, [4]int{0, 0, 2, 2})
	if err != nil || replacement.Texture == region.Texture || p.candidate != second {
		t.Fatal("repaired candidate did not commit")
	}
	if _, valid := dev.TextureInfo(region.Texture); valid {
		t.Fatal("previous candidate texture leaked")
	}
	p.close(ed)
	if _, valid := dev.TextureInfo(replacement.Texture); valid {
		t.Fatal("closing preview retained its private normal texture")
	}
}

func TestImagePreviewControlsAreLocalAndNoGPUIsExplicit(t *testing.T) {
	a, b := &imagePreview{}, &imagePreview{}
	a.init()
	b.init()
	a.controls.Height = 3
	a.controls.FlipX = true
	a.controls.Position = mathx.Vec2{X: 4}
	if a.viewID == b.viewID || b.controls.Height != 1.5 || b.controls.FlipX {
		t.Fatal("preview controls alias another session")
	}
	for _, l := range a.lighting().Lights {
		if err := l.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	dev := null.New()
	defer dev.Close()
	ed := &context.Editor{Renderer: imageTestRenderer(t, dev)}
	before := dev.ResourceStats()
	if err := a.ensure(ed, 64, 64); err == nil {
		t.Fatal("headless Lit preview pretended to succeed")
	}
	if a.mat != nil || a.target != nil || dev.ResourceStats() != before {
		t.Fatal("no-GPU fallback allocated fake preview resources")
	}
}

func TestImageCropPreservesFrameIdentityAndLogicalTrim(t *testing.T) {
	d := texture.New(4, 4, gpu.TexRGBA8)
	d.NonColor = true
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			i := (y*4 + x) * 4
			d.Pixels[i] = byte(x)
			d.Pixels[i+1] = byte(y)
			d.Pixels[i+3] = 255
		}
	}
	p := &imagePixels{data: *d, semantic: asset.TextureNormal}
	out, err := croppedPixels(p, [4]int{1, 2, 2, 1}, [4]int{4, 3, 1, 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.data.Width != 4 || out.data.Height != 3 || !out.data.NonColor {
		t.Fatal("logical source canvas or data semantic was lost")
	}
	got, _ := out.pixel(1, 1)
	if got != [4]byte{1, 2, 0, 255} {
		t.Fatalf("atlas crop used the wrong page coordinates: %v", got)
	}
	pad, _ := out.pixel(0, 0)
	if pad != [4]byte{} {
		t.Fatal("trim padding contains neighboring sprite data")
	}
}
