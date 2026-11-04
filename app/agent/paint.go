package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"path"
	"strings"

	"aego/anim"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/format/pixelart"
)

func paintCommand(out, replace Arg) *Command {
	props := map[string]any{"op": map[string]any{"type": "string", "enum": []string{"pixel", "line", "rect", "fill", "copy"}}}
	for _, n := range []string{"x", "y", "x2", "y2", "w", "h", "toX", "toY"} {
		props[n] = map[string]any{"type": "integer", "minimum": 0, "maximum": 4096}
	}
	for _, n := range []string{"flipX", "flipY"} {
		props[n] = map[string]any{"type": "boolean"}
	}
	props["color"] = map[string]any{"type": "string"}
	item := map[string]any{"type": "object", "properties": props, "required": []string{"op"}, "additionalProperties": false}
	return &Command{Name: "asset.paint", Write: true, Run: (*Session).paint,
		Doc: "Edit a source PNG, create a transparent canvas, or paint on a copy of another source PNG. A single reversible batch; returns image pixels. Coordinates are top-left source pixels.",
		Args: []Arg{out, {Name: "ops", Type: "array", Items: item, Required: true, Doc: `Ordered objects: {op:"line",x,y,x2,y2,color}, {op:"rect",x,y,w,h,color}, {op:"fill",x,y,color} flood-fills connected pixels, {op:"pixel",x,y,color}, or {op:"copy",x,y,w,h,toX,toY,flipX,flipY}. Color is #rrggbb[aa], transparent, or a palette key.`},
			{Name: "width", Type: "integer", Doc: "width of a new blank canvas"}, {Name: "height", Type: "integer", Doc: "height of a new blank canvas"},
			{Name: "source", Type: "string", Doc: "optional existing source PNG under assets/ to copy before editing; defaults to the output if it exists"},
			{Name: "palette", Type: "string", Doc: "palette as r=#ff0000 b=#0000ff, or use the project palette"},
			replace}}
}

func (s *Session) paint(a Args) (*Output, error) {
	rel, err := pngPath(a.Str("path"))
	if err != nil {
		return nil, err
	}
	src := rel
	if a.Has("source") {
		src, err = pngPath(a.Str("source"))
		if err != nil {
			return nil, err
		}
	}
	var img image.Image
	srcBytes, err := s.Pipeline.SourceBytes(src)
	if err != nil {
		return nil, err
	}
	if srcBytes != nil {
		cfg, err := png.DecodeConfig(bytes.NewReader(srcBytes))
		if err != nil {
			return nil, err
		}
		if cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > pixelart.MaxPixels {
			return nil, errors.New("source PNG exceeds the canvas budget")
		}
		img, err = png.Decode(bytes.NewReader(srcBytes))
		if err != nil {
			return nil, err
		}
	} else if a.Has("source") {
		return nil, fmt.Errorf("source PNG does not exist: %s", src)
	}
	pal, err := pixelart.ParsePalette(a.Str("palette"))
	if err != nil {
		return nil, err
	}
	if !a.Has("palette") {
		style, _ := s.art()
		for i, c := range style.Palette {
			if i >= len(paletteKeys) {
				break
			}
			pal[rune(paletteKeys[i])] = c
		}
	}
	w, err := a.Int("width", 0)
	if err != nil {
		return nil, err
	}
	h, err := a.Int("height", 0)
	if err != nil {
		return nil, err
	}
	raw := a.Str("ops")
	if len(raw) > 1<<20 {
		return nil, errors.New("drawing operations exceed 1 MiB")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var ops []pixelart.Op
	if err = dec.Decode(&ops); err != nil {
		return nil, err
	}
	if ops == nil || dec.Decode(new(any)) != io.EOF {
		return nil, errors.New("ops must be one JSON array")
	}
	after, err := pixelart.Paint(img, w, h, pal, ops)
	if err != nil {
		return nil, err
	}
	var guards []pipeline.SourceEdit
	if src != rel {
		guards = append(guards, pipeline.SourceEdit{Path: src, Before: srcBytes, After: srcBytes})
	}
	return s.drawnFrom(a, rel, after, nil, nil, false, nil, nil, guards, src == rel, srcBytes)
}

type DrawingEdit struct {
	Sources []pipeline.SourceEdit
	Files   []string
	Created []string
}

func (s *Session) prepareDrawing(rel string, data []byte, cells []pipeline.SubAsset, flip bool, settings pipeline.Settings, replace bool, view *DrawnView, guards []pipeline.SourceEdit, checkInput bool, input []byte) (*DrawingEdit, error) {
	e := &DrawingEdit{Sources: guards, Files: []string{rel}}
	old, err := s.Pipeline.SourceBytes(rel)
	if err != nil {
		return nil, err
	}
	if checkInput && ((input == nil) != (old == nil) || !bytes.Equal(input, old)) {
		return nil, fmt.Errorf("%s changed while drawing", rel)
	}
	if old != nil && !replace {
		return nil, fmt.Errorf("%s exists; pass replace to draw over it", rel)
	}
	e.Sources = append(e.Sources, pipeline.SourceEdit{Path: rel, Before: old, After: data})
	meta, oldMeta, err := s.sourceMeta(rel, old != nil)
	if err != nil {
		return nil, err
	}
	view.ID = meta.ID.String()
	if old == nil {
		e.Created = append(e.Created, view.ID)
	}
	for k, v := range settings {
		meta.Settings[k] = v
	}
	metaChanged := oldMeta == nil || len(settings) != 0
	if cells != nil {
		named := map[string]pipeline.SubAsset{}
		for _, sub := range meta.Subs {
			named[sub.Name] = sub
		}
		for i, c := range cells {
			prior, ok := named[c.Name]
			if !ok {
				prior = c
				prior.ID = ids.NewAssetID()
				e.Created = append(e.Created, prior.ID.String())
			}
			prior.Rect = c.Rect
			cells[i] = prior
		}
		meta.Subs = cells
		metaChanged = true
	}
	bounds := image.Rect(0, 0, view.Size[0], view.Size[1])
	for _, v := range meta.Subs {
		r := v.Rect
		if r[0] < 0 || r[1] < 0 || r[2] <= 0 || r[3] <= 0 || r[0] > bounds.Dx()-r[2] || r[1] > bounds.Dy()-r[3] {
			return nil, errors.New("frame is outside the new canvas: " + v.Name)
		}
		view.Frames = append(view.Frames, FrameView{Name: v.Name, ID: v.ID.String(), Rect: r})
	}
	afterMeta := oldMeta
	if metaChanged {
		afterMeta = pipeline.WriteMeta(meta)
	}
	e.Sources = append(e.Sources, pipeline.SourceEdit{Path: rel + pipeline.MetaSuffix, Before: oldMeta, After: afterMeta})
	if flip {
		clip := strings.TrimSuffix(rel, path.Ext(rel)) + ".aeanim"
		view.Flipbook = clip
		before, err := s.Pipeline.SourceBytes(clip)
		if err != nil {
			return nil, err
		}
		if before != nil && !replace {
			return nil, fmt.Errorf("%s already exists; pass replace to overwrite the flipbook", clip)
		}
		cm, prev, err := s.sourceMeta(clip, before != nil)
		if err != nil {
			return nil, err
		}
		target := prev
		if prev == nil {
			target = pipeline.WriteMeta(cm)
			e.Created = append(e.Created, cm.ID.String())
		}
		frames := make([]ids.AssetID, len(cells))
		for i, c := range cells {
			frames[i] = c.ID
		}
		e.Sources = append(e.Sources, pipeline.SourceEdit{Path: clip, Before: before, After: anim.FlipbookText(frames)}, pipeline.SourceEdit{Path: clip + pipeline.MetaSuffix, Before: prev, After: target})
		e.Files = append(e.Files, clip)
	}
	return e, nil
}

func (s *Session) sourceMeta(rel string, exists bool) (*pipeline.Meta, []byte, error) {
	raw, err := s.Pipeline.SourceBytes(rel + pipeline.MetaSuffix)
	if err != nil {
		return nil, nil, err
	}
	if (raw != nil) != exists {
		return nil, nil, fmt.Errorf("%s: source and sidecar must exist together; refresh the asset index", rel)
	}
	if exists {
		meta, err := pipeline.ParseMeta(raw, rel+pipeline.MetaSuffix)
		return meta, raw, err
	}
	meta, err := s.Pipeline.NewSourceMeta(rel)
	return meta, nil, err
}
