package texture

import (
	"fmt"
	"math"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/format/aetext"
	rtex "aego/render/texture"
)

func semanticName(s asset.TextureSemantic) string {
	switch s {
	case asset.TextureData:
		return "data"
	case asset.TextureNormal:
		return "normal"
	case asset.TextureMask:
		return "mask"
	case asset.TextureHeight:
		return "height"
	case asset.TextureEmissionRGB:
		return "emission-rgb"
	case asset.TextureEmissionMask:
		return "emission-mask"
	case asset.TextureCookieMask:
		return "cookie-mask"
	}
	return "color"
}
func semanticOf(name string, s pipeline.Settings) (asset.TextureSemantic, error) {
	role, err := asset.TextureRoleOf(name, s)
	if err != nil {
		return 0, err
	}
	switch role {
	case asset.RoleData:
		return asset.TextureData, nil
	case asset.RoleNormal:
		return asset.TextureNormal, nil
	case asset.RoleMask:
		return asset.TextureMask, nil
	case asset.RoleHeight:
		return asset.TextureHeight, nil
	case asset.RoleEmissionRGB:
		return asset.TextureEmissionRGB, nil
	case asset.RoleEmissionMask:
		return asset.TextureEmissionMask, nil
	case asset.RoleCookieMask:
		return asset.TextureCookieMask, nil
	}
	return asset.TextureColor, nil
}

func prepareSurfaceData(d *rtex.Data, semantic asset.TextureSemantic, s pipeline.Settings, subs []pipeline.SubAsset) error {
	for _, key := range []string{"normal_y", "height_channel"} {
		if v, ok := s[key]; ok && v.Kind != aetext.VString && v.Kind != aetext.VIdent {
			return fmt.Errorf("%s must be a name", key)
		}
	}
	if v, ok := s["generate_normal"]; ok && v.Kind != aetext.VBool {
		return fmt.Errorf("generate_normal must be a bool")
	}
	if v, ok := s["height_strength"]; ok && v.Kind != aetext.VFloat && v.Kind != aetext.VInt {
		return fmt.Errorf("height_strength must be a number")
	}
	if semantic == asset.TextureEmissionMask || semantic == asset.TextureCookieMask {
		channel := s.String("mask_channel", "r")
		index := 0
		switch channel {
		case "r":
		case "g":
			index = 1
		case "b":
			index = 2
		case "a":
			index = 3
		case "luminance":
		default:
			return fmt.Errorf("mask_channel must be r, g, b, a or luminance")
		}
		for i := 0; i < len(d.Pixels); i += 4 {
			v := d.Pixels[i+index]
			if channel == "luminance" {
				v = byte(math.Round(float64(d.Pixels[i])*.2126 + float64(d.Pixels[i+1])*.7152 + float64(d.Pixels[i+2])*.0722))
			}
			d.Pixels[i], d.Pixels[i+1], d.Pixels[i+2], d.Pixels[i+3] = v, v, v, 255
		}
	}
	if semantic == asset.TextureEmissionRGB {
		for i := 3; i < len(d.Pixels); i += 4 {
			d.Pixels[i] = 255
		}
	}
	y := s.String("normal_y", "up")
	if y != "up" && y != "down" {
		return fmt.Errorf("normal_y must be up or down, got %q", y)
	}
	generate := s.Bool("generate_normal", false)
	if generate && semantic != asset.TextureHeight {
		return fmt.Errorf("generate_normal requires semantic=height")
	}
	if generate {
		strength := s.Float("height_strength", 1)
		if math.IsNaN(strength) || math.IsInf(strength, 0) || strength < 0 || strength > 65536 {
			return fmt.Errorf("height_strength must be finite and in [0, 65536]")
		}
		channel := s.String("height_channel", "r")
		if channel != "r" && channel != "g" && channel != "b" && channel != "a" && channel != "luminance" {
			return fmt.Errorf("height_channel must be r, g, b, a or luminance")
		}
		if err := generateNormals(d, strength, channel, y == "down", subs); err != nil {
			return err
		}
	} else if semantic == asset.TextureNormal && y == "down" {
		for i := 1; i < len(d.Pixels); i += 4 {
			d.Pixels[i] = 255 - d.Pixels[i]
		}
	}
	return nil
}

// Height is linear and sampled in texels; clamped central differences prevent frame-edge wrap. Green +Y is image-up; alpha is unpremultiplied coverage/data.
func generateNormals(d *rtex.Data, strength float64, channel string, yDown bool, subs []pipeline.SubAsset) error {
	rects := make([][4]int, 0, max(1, len(subs)))
	for _, sub := range subs {
		r := sub.Rect
		if r[0] < 0 || r[1] < 0 || r[2] <= 0 || r[3] <= 0 || r[0] >= d.Width || r[1] >= d.Height || r[2] > d.Width-r[0] || r[3] > d.Height-r[1] {
			return fmt.Errorf("height frame %s has an invalid source rectangle", sub.Name)
		}
		for _, prev := range rects {
			if r[0] < prev[0]+prev[2] && prev[0] < r[0]+r[2] && r[1] < prev[1]+prev[3] && prev[1] < r[1]+r[3] {
				return fmt.Errorf("height frames must not overlap when generating normals")
			}
		}
		rects = append(rects, r)
	}
	if len(rects) == 0 {
		rects = append(rects, [4]int{0, 0, d.Width, d.Height})
	}
	heights := make([]float64, d.Width*d.Height)
	index := 0
	switch channel {
	case "g":
		index = 1
	case "b":
		index = 2
	case "a":
		index = 3
	}
	for i := range heights {
		heights[i] = float64(d.Pixels[i*4+index]) / 255
		if channel == "luminance" {
			heights[i] = (float64(d.Pixels[i*4])*.2126 + float64(d.Pixels[i*4+1])*.7152 + float64(d.Pixels[i*4+2])*.0722) / 255
		}
	}
	encode := func(v float64) byte { return byte(math.Round(max(0, min(1, v*.5+.5)) * 255)) }
	for i := 0; i < len(d.Pixels); i += 4 {
		d.Pixels[i], d.Pixels[i+1], d.Pixels[i+2] = 128, 128, 255
	}
	for _, r := range rects {
		for y := r[1]; y < r[1]+r[3]; y++ {
			for x := r[0]; x < r[0]+r[2]; x++ {
				dx := (heights[y*d.Width+min(x+1, r[0]+r[2]-1)] - heights[y*d.Width+max(x-1, r[0])]) * .5 * strength
				dy := (heights[min(y+1, r[1]+r[3]-1)*d.Width+x] - heights[max(y-1, r[1])*d.Width+x]) * .5 * strength
				if yDown {
					dy = -dy
				}
				length := math.Sqrt(dx*dx + dy*dy + 1)
				i := (y*d.Width + x) * 4
				d.Pixels[i], d.Pixels[i+1], d.Pixels[i+2] = encode(-dx/length), encode(dy/length), encode(1/length)
			}
		}
	}
	return nil
}
