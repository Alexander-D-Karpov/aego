package editor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/gpu"
	"aego/render/texture"
)

func TestImageViewportCursorZoomPanAndPhysicalOneToOne(t *testing.T) {
	area := mathx.Rect{X: 50, Y: 20, W: 800, H: 500}
	var v imageViewport
	v.fit(area, 256, 128)
	if !v.Ready || v.Zoom <= 0 {
		t.Fatal("fit did not initialize viewport")
	}
	mouse := mathx.Vec2{X: 314, Y: 178}
	before := v.imagePoint(area, mouse)
	v.zoomAt(area, mouse, 2.5)
	after := v.imagePoint(area, mouse)
	if before.Sub(after).Len() > 1e-4 {
		t.Fatal("zoom drifted under cursor", before, after)
	}
	point := v.screenPoint(area, before)
	v.pan(mathx.Vec2{X: 17, Y: -9})
	moved := v.screenPoint(area, before)
	if moved.Sub(point).Sub(mathx.Vec2{X: 17, Y: -9}).Len() > 1e-4 {
		t.Fatal("pan did not move image in logical pixels")
	}
	v.oneToOne(area, 256, 128, 2)
	if rect := v.rect(area, 256, 128); rect.W*2 != 256 || rect.H*2 != 128 {
		t.Fatal("1:1 ignored physical display scale")
	}
	v.zoomAt(area, mouse, 1e5)
	if v.Zoom != 256 {
		t.Fatal("zoom was not bounded")
	}
}

func TestImageChannelsPreserveRawPixelsAndApplyAlphaOnce(t *testing.T) {
	raw := []byte{188, 99, 42, 128, 201, 13, 250, 0}
	p := &imagePixels{data: texture.Data{Width: 2, Height: 1, Format: gpu.TexSRGBA8, Pixels: bytes.Clone(raw)}, semantic: asset.TextureColor}
	for _, channel := range []imageChannel{imageRGBA, imageRGB, imageR, imageG, imageB, imageA} {
		d, err := p.projection(channel)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(p.data.Pixels, raw) {
			t.Fatal("channel view modified source bytes")
		}
		if channel >= imageR {
			v := raw[int(channel-imageR)]
			if d.Pixels[0] != v || d.Pixels[1] != v || d.Pixels[2] != v || d.Pixels[3] != 255 {
				t.Fatal("raw channel projection wrong")
			}
		}
		if channel == imageRGB && d.Pixels[4] != 201 {
			t.Fatal("RGB view discarded hidden transparent color")
		}
		if channel == imageRGBA {
			if d.Pixels[0] >= 188 || d.Pixels[3] != 128 {
				t.Fatal("source RGBA was not premultiplied once")
			}
			imported := &imagePixels{data: *d, semantic: asset.TextureColor}
			again, _ := imported.projection(channel)
			if !bytes.Equal(d.Pixels, again.Pixels) {
				t.Fatal("imported PMA was multiplied a second time")
			}
		}
	}
	if got, ok := p.pixel(1, 0); !ok || got != ([4]byte{201, 13, 250, 0}) {
		t.Fatal("pixel readout lost raw RGBA")
	}
	if _, ok := p.pixel(-1, 0); ok {
		t.Fatal("negative coordinate accepted")
	}
}

func TestImageSourceAndArtifactRemainDistinct(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 210, G: 120, B: 70, A: 128})
	src.SetNRGBA(1, 0, color.NRGBA{R: 90, G: 80, B: 70, A: 0})
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	got, err := decodeImageSource(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if px, _ := got.pixel(0, 0); px != ([4]byte{210, 120, 70, 128}) {
		t.Fatal("source decoder changed raw PNG")
	}
	if px, _ := got.pixel(1, 0); px[0] != 90 {
		t.Fatal("source decoder discarded transparent RGB")
	}
	normal, err := decodeImageArtifact(asset.EncodeTexture(&asset.TextureArtifact{Width: 1, Height: 1, Format: gpu.TexRGBA8, Semantic: asset.TextureNormal, Pixels: []byte{128, 170, 250, 73}}))
	if err != nil {
		t.Fatal(err)
	}
	if normal.data.Premultiplied || !normal.data.NonColor {
		t.Fatal("normal was treated as color")
	}
	view, err := normal.projection(imageRGB)
	if err != nil || view.Pixels[1] != 170 || view.Pixels[3] != 255 {
		t.Fatal("normal raw RGB not visible", err)
	}
	if px, _ := normal.pixel(0, 0); px[3] != 73 {
		t.Fatal("normal source alpha modified")
	}
}

func TestImageSourceReadRejectsOversizeBeforeDecoding(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "large.png")
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	err = f.Truncate(maxViewerSource + 1)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	docs := document.NewManager(document.Options{Root: root})
	if data, err := readImageSource(docs, "large.png"); err == nil || len(data) != 0 {
		t.Fatal("oversize encoded source was read into the viewer")
	}
}
