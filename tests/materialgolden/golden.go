// Package materialgolden provides fixed CPU reference images for the material
// integration scenes. Expected pixels follow premultiplied source-over in a
// linear RGBA8 offscreen target; no driver output is used to update a reference.
package materialgolden

import "fmt"

const Size = 32

// Stage12aBuiltins keeps the previous material regression scenes unchanged.
// Stage12b Lit and LitMesh2D have their own lighting/surface GPU suite.
var Stage12aBuiltins = [...]string{"Unlit", "Additive", "Flash", "Dissolve", "Outline", "PixelSnap"}

func Reference(name string) ([]byte, bool) {
	switch name {
	case "trim-none":
		return trimReference(false, false), true
	case "trim-x":
		return trimReference(true, false), true
	case "trim-y":
		return trimReference(false, true), true
	case "trim-xy", "trim-xy-restored":
		return trimReference(true, true), true
	case "pixelsnap-dpr2-offset":
		out := make([]byte, Size*Size*4)
		for y := 12; y < 20; y++ {
			for x := 8; x < 20; x++ {
				for c := 0; c < 4; c++ {
					out[(y*Size+x)*4+c] = 255
				}
			}
		}
		return out, true
	}
	mode, checker := name, false
	switch name {
	case "builtin-Unlit", "builtin-Flash", "builtin-Dissolve", "builtin-PixelSnap":
		mode = "unlit"
	case "builtin-Additive":
		mode = "additive-one"
	case "builtin-Outline":
		mode = "outline"
	case "checker-unlit":
		mode, checker = "unlit", true
	case "checker-flash-restored":
		mode, checker = "flash", true
	case "opacity", "flash", "additive", "outline", "unlit", "dissolved":
	default:
		return nil, false
	}
	data := make([]byte, Size*Size*4)
	covered := func(x, y int) bool { return x >= 4 && x < 28 && y >= 4 && y < 28 && !(x < 10 && y < 10) }
	for y := range Size {
		for x := range Size {
			var dst [4]float64
			if checker {
				v := float64(64)
				if (x/8+y/8)%2 != 0 {
					v = 191
				}
				dst = [4]float64{v, v, v, 255}
			}
			if covered(x, y) && mode != "dissolved" {
				src := [4]float64{128, 0, 0, 128}
				switch mode {
				case "flash":
					src = [4]float64{128, 128, 128, 128}
				case "opacity":
					src = [4]float64{64, 0, 0, 64}
				case "additive":
					// Stage12b deliberately changes Additive to preserve coverage.
					src = [4]float64{255, 0, 0, 0}
				case "additive-one":
					src = [4]float64{128, 0, 0, 0}
				case "outline":
					for dy := -6; dy <= 6; dy += 6 {
						for dx := -6; dx <= 6; dx += 6 {
							if !covered(x+dx, y+dy) {
								src[0] = 0
							}
						}
					}
				}
				for c := range dst {
					dst[c] = src[c] + dst[c]*(1-src[3]/255)
				}
			}
			for c, v := range dst {
				data[(y*Size+x)*4+c] = byte(v + .5)
			}
		}
	}
	return data, true
}

func Compare(name string, got []byte, tolerance int) error {
	want, ok := Reference(name)
	if !ok {
		return fmt.Errorf("unknown reference image %q", name)
	}
	if len(got) != len(want) {
		return fmt.Errorf("image bytes %d, expected %d", len(got), len(want))
	}
	for i, v := range want {
		d := int(got[i]) - int(v)
		if d < -tolerance || d > tolerance {
			return fmt.Errorf("pixel (%d,%d) channel %d: got %d, reference %d (tolerance %d)", (i/4)%Size, (i/4)/Size, i%4, got[i], v, tolerance)
		}
	}
	return nil
}

func trimReference(flipX, flipY bool) []byte {
	out := make([]byte, Size*Size*4)
	x0, y0 := 12, 12
	if flipX {
		x0 = 14
	}
	if flipY {
		y0 = 16
	}
	for y := y0; y < y0+4; y++ {
		for x := x0; x < x0+6; x++ {
			u, v := (float64(x)+.5-8)/16, (float64(y)+.5-10)/12
			if flipX {
				u = 1 - u
			}
			if flipY {
				v = 1 - v
			}
			i := (y*Size + x) * 4
			out[i], out[i+1], out[i+3] = byte(u*255+.5), byte(v*255+.5), 255
		}
	}
	return out
}
