//go:build aego_editor

package docsview

import (
	"fmt"
	"image"
	"math"
)

// Viewport is the last Game View rectangle actually laid out by the Go UI.
// It does not include the Inspector or the document toolbar.
type Viewport struct {
	X            float32 `json:"x"`
	Y            float32 `json:"y"`
	Width        float32 `json:"width"`
	Height       float32 `json:"height"`
	CanvasWidth  int     `json:"canvasWidth"`
	CanvasHeight int     `json:"canvasHeight"`
}

func (v Viewport) pixelRect(width, height int) (image.Rectangle, error) {
	if v.CanvasWidth <= 0 || v.CanvasHeight <= 0 || v.Width <= 0 || v.Height <= 0 || width <= 0 || height <= 0 {
		return image.Rectangle{}, fmt.Errorf("game view has not been presented")
	}
	sx, sy := float64(width)/float64(v.CanvasWidth), float64(height)/float64(v.CanvasHeight)
	r := image.Rect(int(math.Ceil(float64(v.X)*sx)), int(math.Ceil(float64(v.Y)*sy)), int(math.Floor(float64(v.X+v.Width)*sx)), int(math.Floor(float64(v.Y+v.Height)*sy)))
	if r.Empty() || !r.In(image.Rect(0, 0, width, height)) {
		return image.Rectangle{}, fmt.Errorf("game view rectangle is outside the captured canvas")
	}
	return r, nil
}
