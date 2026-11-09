package artsource

import (
	"image"
	"math"
)

// The caller supplies the relevant extent: PNG source-frame dimensions for a
// linked texture, or Art canvas dimensions for a generated Art region. Keep
// the two coordinate spaces distinct, especially for differently sized frames.
func normalizedPivotToPixels(pivot [2]float32, width, height int) image.Point {
	return image.Pt(int(math.Round(float64(pivot[0])*float64(width))), int(math.Round(float64(pivot[1])*float64(height))))
}

func pixelPivotToNormalized(pivot image.Point, width, height int) [2]float32 {
	return [2]float32{float32(pivot.X) / float32(width), float32(pivot.Y) / float32(height)}
}
