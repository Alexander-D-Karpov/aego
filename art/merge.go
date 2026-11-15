package art

import (
	"context"
	"fmt"
	"image"
)

// MergeDown bakes two adjacent normal-blend raster (or reference) siblings for
// every frame and surface channel. Their parent, frame IDs, lower layer ID and
// existing lower cel IDs survive. A repeated pair of linked images remains
// linked, while images shared with other layers are never overwritten.
//
// Backdrop-dependent blend modes are rejected instead of silently changing
// their appearance over unrelated lower layers. Indexed art is accepted only
// when the exact merged colors are representable in its current palette.
func (e *Edit) MergeDown(ctx context.Context, upper ID) (ID, error) {
	if err := e.check(); err != nil {
		return "", err
	}
	ui, ok := e.work.layer(upper)
	if !ok {
		return "", fmt.Errorf("art: missing layer")
	}
	top := e.work.Layers[ui]
	li := -1
	for i := ui - 1; i >= 0; i-- {
		if e.work.Layers[i].Parent == top.Parent {
			li = i
			break
		}
	}
	if li < 0 {
		return "", fmt.Errorf("art: no layer below in the same group")
	}
	lower := e.work.Layers[li]
	if top.Kind == Group || lower.Kind != top.Kind {
		return "", fmt.Errorf("art: merge requires two raster or two reference layers")
	}
	if e.work.isLocked(upper) || e.work.isLocked(lower.ID) {
		return "", ErrLocked
	}
	if !top.Visible || !lower.Visible {
		return "", fmt.Errorf("art: show both layers before merging")
	}
	if top.Blend != BlendNormal || lower.Blend != BlendNormal {
		return "", fmt.Errorf("art: merge requires Normal blend to preserve the existing backdrop")
	}
	before := e.work.clone()
	pair := before.clone()
	lower.Parent, top.Parent = "", ""
	pair.Layers = []Layer{lower, top}
	type source struct {
		Image   ID
		X, Y    int
		Opacity uint8
	}
	type identity struct {
		Lower, Upper source
		Channel      Channel
	}
	cache := make(map[identity]ID)
	colorIndices := make(map[Color]uint32)
	for i, swatch := range before.Palette {
		if _, exists := colorIndices[swatch.Color]; !exists {
			colorIndices[swatch.Color] = uint32(i)
		}
	}
	for _, frame := range before.Frames {
		for _, channel := range []Channel{Albedo, Normal, Height, Emission} {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			key := identity{Channel: channel}
			present := false
			for _, cel := range before.Cels {
				if cel.Frame != frame.ID || cel.Channel != channel {
					continue
				}
				value := source{cel.Image, cel.X, cel.Y, cel.Opacity}
				if cel.Layer == lower.ID {
					key.Lower, present = value, true
				} else if cel.Layer == upper {
					key.Upper, present = value, true
				}
			}
			if !present {
				continue
			}
			merged := cache[key]
			if merged == "" {
				im := image.NewNRGBA(image.Rect(0, 0, before.Canvas.Width, before.Canvas.Height))
				if err := renderRegion(pair, frame.ID, im, im.Bounds(), RenderOptions{Channel: channel, References: true}); err != nil {
					return "", err
				}
				merged = e.doc.newID()
				e.work.Images[merged] = newPixels(im.Bounds().Dx(), im.Bounds().Dy())
				for y := 0; y < im.Bounds().Dy(); y++ {
					if err := ctx.Err(); err != nil {
						return "", err
					}
					for x := 0; x < im.Bounds().Dx(); x++ {
						p := im.NRGBAAt(x, y)
						color := Color{p.R, p.G, p.B, p.A}
						var pixel uint32
						if channel == Albedo && before.Canvas.Mode == Indexed {
							if color.A != 0 {
								var exact bool
								pixel, exact = colorIndices[color]
								if !exact {
									return "", fmt.Errorf("art: merged color is outside the indexed palette; convert to RGBA explicitly")
								}
							}
						} else {
							pixel = encode(before.forChannel(channel), color)
						}
						if err := e.raw(merged, x, y, pixel); err != nil {
							return "", err
						}
					}
				}
				cache[key] = merged
			}
			if _, err := e.EnsureCel(lower.ID, frame.ID, channel); err != nil {
				return "", err
			}
			ci, _ := e.work.findCel(lower.ID, frame.ID, channel)
			e.work.Cels[ci].Image = merged
			e.work.Cels[ci].X, e.work.Cels[ci].Y, e.work.Cels[ci].Opacity = 0, 0, 255
		}
	}
	e.work.Layers[li].Opacity = 255
	e.touch(image.Rect(0, 0, before.Canvas.Width, before.Canvas.Height))
	if err := e.DeleteLayer(upper); err != nil {
		return "", err
	}
	return lower.ID, nil
}
