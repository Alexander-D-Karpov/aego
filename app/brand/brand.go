package brand

import (
	"bytes"
	"embed"
	"image"
	"image/png"
	"strconv"
)

//go:embed png/aego-*.png
var files embed.FS

var windowSizes = []int{16, 24, 32, 48, 64, 128, 256}

func PNG(n int) ([]byte, bool) {
	data, err := files.ReadFile("png/aego-" + strconv.Itoa(n) + ".png")
	return data, err == nil
}

func Window() []image.Image {
	out := make([]image.Image, 0, len(windowSizes))
	for _, n := range windowSizes {
		data, ok := PNG(n)
		if !ok {
			continue
		}
		if img, err := png.Decode(bytes.NewReader(data)); err == nil {
			out = append(out, img)
		}
	}
	return out
}
