package art

import (
	"context"
	"fmt"
	"reflect"
)

// AnimationClipboard holds an immutable snapshot, not another live document.
// Editing or closing the source cannot change the copied animation. New IDs are
// allocated in the destination; source and destination never share mutable data.
type AnimationClipboard struct {
	source *state
	frames []Frame
}

func (d *Document) CopyAnimation(frames []ID) (*AnimationClipboard, error) {
	snapshot, err := d.Snapshot()
	if err != nil {
		return nil, err
	}
	s := snapshot.view.current
	if len(frames) == 0 || len(frames) > len(s.Frames) {
		return nil, fmt.Errorf("art: select at least one animation frame")
	}
	want := map[ID]bool{}
	for _, id := range frames {
		if want[id] {
			return nil, fmt.Errorf("art: duplicate animation frame")
		}
		if _, ok := s.frame(id); !ok {
			return nil, fmt.Errorf("art: missing animation frame")
		}
		want[id] = true
	}
	clip := &AnimationClipboard{source: s}
	for _, f := range s.Frames {
		if want[f.ID] {
			clip.frames = append(clip.frames, f)
		}
	}
	return clip, nil
}

// PasteAnimation adds an independent set of layers and frames after the given
// frame. It preserves durations, internal cel links, groups, channels, tags and
// slices. Existing target layers/cels are untouched. Canvas dimensions must agree
// so importing a character never silently crops or resizes its pixels/pivot.
func (e *Edit) PasteAnimation(ctx context.Context, clip *AnimationClipboard, after ID) ([]ID, error) {
	if err := e.check(); err != nil {
		return nil, err
	}
	if clip == nil || clip.source == nil || len(clip.frames) == 0 {
		return nil, fmt.Errorf("art: empty animation clipboard")
	}
	src := clip.source
	if src.Canvas.Width != e.work.Canvas.Width || src.Canvas.Height != e.work.Canvas.Height {
		return nil, fmt.Errorf("art: animation clipboard canvas size differs")
	}
	if len(src.Layers)+len(e.work.Layers) > e.doc.limits.MaxLayers || len(clip.frames)+len(e.work.Frames) > e.doc.limits.MaxFrames {
		return nil, ErrLimit
	}
	if after != "" {
		if _, ok := e.work.frame(after); !ok {
			return nil, fmt.Errorf("art: missing destination frame")
		}
	}
	frameIDs := map[ID]ID{}
	layerIDs := map[ID]ID{}
	imageIDs := map[ID]ID{}
	var result []ID
	for _, f := range clip.frames {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id, err := e.AddFrame(after, f.DurationMS)
		if err != nil {
			return nil, err
		}
		frameIDs[f.ID] = id
		result = append(result, id)
		after = id
	}
	// Allocate all IDs before mapping parent links; a valid file need not list a
	// group before its children in physical storage order.
	for _, l := range src.Layers {
		layerIDs[l.ID] = e.doc.newID()
	}
	for _, l := range src.Layers {
		l.ID, l.Parent = layerIDs[l.ID], layerIDs[l.Parent]
		e.work.Layers = append(e.work.Layers, l)
	}
	for _, c := range src.Cels {
		frame, selected := frameIDs[c.Frame]
		if !selected {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(e.work.Cels) >= e.doc.limits.MaxCels {
			return nil, ErrLimit
		}
		imageID := imageIDs[c.Image]
		if imageID == "" {
			imageID = e.doc.newID()
			imageIDs[c.Image] = imageID
			source := src.Images[c.Image]
			targetState := e.work.forChannel(c.Channel)
			sourceState := src.forChannel(c.Channel)
			sameEncoding := sourceState.Canvas.Mode == targetState.Canvas.Mode && (sourceState.Canvas.Mode != Indexed || reflect.DeepEqual(src.Palette, e.work.Palette))
			if sameEncoding {
				e.work.Images[imageID] = source // immutable tiles, first destination write is COW
			} else {
				e.work.Images[imageID] = newPixels(source.Width, source.Height)
				for key, block := range source.Tiles {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					for i, v := range block {
						x, y := key.X*TileSize+i%TileSize, key.Y*TileSize+i/TileSize
						if x >= source.Width || y >= source.Height {
							continue
						}
						color := decode(sourceState, v)
						encoded := encode(targetState, color)
						// Animation transfer is not an implicit lossy palette conversion.
						if decode(targetState, encoded) != color {
							return nil, fmt.Errorf("art: animation color is not representable in destination mode/palette")
						}
						if err := e.raw(imageID, x, y, encoded); err != nil {
							return nil, err
						}
					}
				}
			}
		}
		c.ID = e.doc.newID()
		c.Layer = layerIDs[c.Layer]
		c.Frame = frame
		c.Image = imageID
		e.work.Cels = append(e.work.Cels, c)
	}
	for _, tag := range src.Tags {
		from, _ := src.frame(tag.From)
		to, _ := src.frame(tag.To)
		var selected []ID
		for _, f := range src.Frames[from : to+1] {
			if id := frameIDs[f.ID]; id != "" {
				selected = append(selected, id)
			}
		}
		if len(selected) == 0 {
			continue
		}
		tag.ID = e.doc.newID()
		tag.From, tag.To = selected[0], selected[len(selected)-1]
		e.work.Tags = append(e.work.Tags, tag)
	}
	for _, slice := range src.Slices {
		if slice.Frame != "" {
			id := frameIDs[slice.Frame]
			if id == "" {
				continue
			}
			slice.Frame = id
		}
		slice.ID = e.doc.newID()
		e.work.Slices = append(e.work.Slices, slice)
	}
	e.structural = true
	return result, nil
}
