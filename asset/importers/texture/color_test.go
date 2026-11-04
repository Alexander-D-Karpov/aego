package texture_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"

	"aego/asset"
	"aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/format/aetext"
	"aego/gpu"
)

func TestImporterSeparatesColorAndData(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 128, B: 32, A: 128})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []bool{false, true} {
		imp := texture.New()
		settings := imp.Defaults()
		settings["data"] = aetext.Bool(raw)
		res, err := imp.Import(&pipeline.ImportContext{Path: "image.png", Source: buf.Bytes(), Meta: &pipeline.Meta{Settings: settings}})
		if err != nil {
			t.Fatal(err)
		}
		art, err := asset.DecodeTexture(res.Artifacts[0].Data)
		if err != nil {
			t.Fatal(err)
		}
		if raw {
			if art.Format != gpu.TexRGBA8 || !slices.Equal(art.Pixels, []byte{255, 128, 32, 128}) {
				t.Fatalf("data altered: %v %v", art.Format, art.Pixels)
			}
		} else {
			if art.Format != gpu.TexSRGBA8 || art.Pixels[0] < 187 || art.Pixels[0] > 189 || art.Pixels[3] != 128 {
				t.Fatalf("sRGB premultiply wrong: %v %v", art.Format, art.Pixels)
			}
		}
	}
}
func TestDataTextureCannotEnterColorAtlas(t *testing.T) {
	imp := texture.New()
	s := imp.Defaults()
	s["data"] = aetext.Bool(true)
	s["atlas"] = aetext.String("sprites")
	if _, err := imp.Import(&pipeline.ImportContext{Path: "noise.png", Meta: &pipeline.Meta{Settings: s}}); err == nil {
		t.Fatal("data texture admitted into colour atlas")
	}
}
