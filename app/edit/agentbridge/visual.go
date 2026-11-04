package agentbridge

import (
	"aego/asset/pipeline"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"aego/app/agent"
	"aego/app/agent/mcp"
	"aego/core/ids"
	"aego/editor/core/preview"
)

type contentThumbs interface {
	Content(ids.AssetID) (image.Image, string, error)
}

func visualSchema(req []string, props map[string]any) map[string]any {
	schema := map[string]any{"type": "object", "additionalProperties": false, "properties": props}
	if len(req) > 0 {
		schema["required"] = req
	}
	return schema
}
func (b *Bridge) visualTools() []mcp.Tool {
	str := map[string]any{"type": "string"}
	rect := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "minItems": 4, "maxItems": 4}
	return append([]mcp.Tool{b.generatorTool(), b.compareTool(),
		{Name: "asset_view", Description: "See a project asset as a PNG. preview uses the Assets tile; source and imported select image/frame representations. Optional rect [x,y,width,height] returns original pixels at 1:1 (up to 1024 per side).", ReadOnly: true, Schema: visualSchema([]string{"asset"}, map[string]any{"asset": str, "view": map[string]any{"type": "string", "enum": []string{"preview", "source", "imported"}}, "rect": rect}), Output: b.viewAsset},
		{Name: "asset_contact_sheet", Description: "See up to 12 cells from an explicit list of asset IDs, a project folder, or an animation/sheet. Supply exactly one of assets, folder or sequence. Use offset to page.", ReadOnly: true, Schema: visualSchema(nil, map[string]any{"folder": str, "sequence": str, "offset": map[string]any{"type": "integer", "minimum": 0}, "assets": map[string]any{"type": "array", "items": str, "minItems": 1, "maxItems": 12}}), Output: b.contactSheet},
		{Name: "editor_view_capture", Description: "See the most recently rendered Game View or Scene View of the open document. Game requires Play. For reproducible future ticks use scene_run with shot, seed and input.", ReadOnly: true, Schema: visualSchema([]string{"view"}, map[string]any{"view": map[string]any{"type": "string", "enum": []string{"game", "scene"}}}), Output: b.captureView},
	}, b.deliveryTools()...)
}

func (b *Bridge) resolveImage(name string) (ids.AssetID, error) {
	if b.ctx.Pipeline == nil {
		return ids.AssetID{}, errors.New("project asset index unavailable")
	}
	if id, err := ids.ParseAssetID(name); err == nil && !id.IsZero() {
		return id, nil
	}
	if strings.TrimSpace(name) != "" {
		if id, ok := b.ctx.Pipeline.Lookup(name); ok {
			return id, nil
		}
	}
	if t, ok := b.ctx.Thumbs.(interface {
		SourceID(string) (ids.AssetID, bool)
	}); ok {
		if id, found := t.SourceID(name); found {
			return id, nil
		}
	}
	return ids.AssetID{}, fmt.Errorf("unknown project asset %q", name)
}
func scaleImg(src image.Image, r image.Rectangle, width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, src.At(r.Min.X+x*r.Dx()/width, r.Min.Y+y*r.Dy()/height))
		}
	}
	return img
}
func encodePNG(img image.Image) (agent.Image, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return agent.Image{}, err
	}
	if buf.Len() > 5<<20 {
		return agent.Image{}, errors.New("PNG exceeds the tool image budget; choose a smaller region")
	}
	return agent.Image{Mime: "image/png", Data: buf.Bytes()}, nil
}
func imageArea(v any, b image.Rectangle) (image.Rectangle, error) {
	if v == nil {
		return b, nil
	}
	a, ok := v.([]any)
	if !ok || len(a) != 4 {
		return image.Rectangle{}, errors.New("rect needs four integers")
	}
	var n [4]int
	for i, it := range a {
		x, ok := it.(float64)
		if !ok || x < 0 || x > 65535 || math.Trunc(x) != x {
			return image.Rectangle{}, errors.New("invalid rect coordinate")
		}
		n[i] = int(x)
	}
	r := image.Rect(b.Min.X+n[0], b.Min.Y+n[1], b.Min.X+n[0]+n[2], b.Min.Y+n[1]+n[3])
	if r.Empty() || !r.In(b) || r.Dx() > 1024 || r.Dy() > 1024 {
		return image.Rectangle{}, errors.New("crop must fit the image and the 1024px limit")
	}
	return r, nil
}

func (b *Bridge) assetPixels(name, mode string) (ids.AssetID, image.Image, string, string, error) {
	generation := b.panelGeneration.Load()
	until := time.Now().Add(5 * time.Second)
	for {
		var id ids.AssetID
		var img image.Image
		var rev, role string
		var err error
		if e := b.main(func() {
			if b.cancelled(generation) {
				err = errCancelled
				return
			}
			id, err = b.resolveImage(name)
			if err != nil {
				return
			}
			switch mode {
			case "preview":
				t, ok := b.ctx.Thumbs.(contentThumbs)
				if !ok {
					err = errors.New("content previews unavailable in this session")
					return
				}
				img, rev, err = t.Content(id)
			case "source", "imported":
				if b.ctx.ImageContent == nil {
					err = errors.New("image viewer not registered")
					return
				}
				img, role, err = b.ctx.ImageContent(id, mode)
			default:
				err = errors.New("view must be preview, source or imported")
			}
		}); e != nil {
			return id, nil, "", "", e
		}
		if !errors.Is(err, preview.ErrPending) || time.Now().After(until) {
			return id, img, rev, role, err
		}
		time.Sleep(35 * time.Millisecond)
	}
}
func (b *Bridge) viewAsset(a map[string]any) (*agent.Output, error) {
	name, _ := a["asset"].(string)
	mode, _ := a["view"].(string)
	if mode == "" {
		mode = "preview"
	}
	id, img, rev, role, err := b.assetPixels(name, mode)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, errors.New("image unavailable")
	}
	bounds := img.Bounds()
	rect, err := imageArea(a["rect"], bounds)
	if err != nil {
		return nil, err
	}
	w, h := rect.Dx(), rect.Dy()
	if a["rect"] == nil && max(w, h) > 1024 {
		m := max(w, h)
		w = max(1, w*1024/m)
		h = max(1, h*1024/m)
	}
	result, err := encodePNG(scaleImg(img, rect, w, h))
	if err != nil {
		return nil, err
	}
	if rev == "" {
		if raw, ok := img.(*image.NRGBA); ok {
			h := sha256.New()
			fmt.Fprintf(h, "%s:%s:%d:%d:", id, role, bounds.Dx(), bounds.Dy())
			h.Write(raw.Pix)
			rev = fmt.Sprintf("%x", h.Sum(nil))
		} else {
			rev = fmt.Sprintf("%x", sha256.Sum256(result.Data))
		}
	}
	return &agent.Output{OK: true, Value: map[string]any{"assetID": id.String(), "view": mode, "role": role, "revision": rev, "sourceWidth": bounds.Dx(), "sourceHeight": bounds.Dy(), "width": w, "height": h}, Images: []agent.Image{result}}, nil
}
func (b *Bridge) contactSheet(a map[string]any) (*agent.Output, error) {
	generation := b.panelGeneration.Load()
	list, total, offset, err := b.selectCells(a, generation)
	if err != nil {
		return nil, err
	}
	const cell = 96
	cols := min(4, len(list))
	sheet := image.NewNRGBA(image.Rect(0, 0, cols*cell, ((len(list)+cols-1)/cols)*cell))
	cells := make([]map[string]any, 0, len(list))
	for n, v := range list {
		name, ok := v.(string)
		if !ok {
			return nil, errors.New("expected an asset path or ID")
		}
		if b.cancelled(generation) {
			return nil, errCancelled
		}
		id, img, rev, _, err := b.assetPixels(name, "preview")
		if err != nil {
			return nil, fmt.Errorf("cell %d (%s): %w", n, name, err)
		}
		if img == nil || img.Bounds().Empty() {
			return nil, errors.New("empty preview")
		}
		r := img.Bounds()
		m := max(r.Dx(), r.Dy())
		w, h := max(1, r.Dx()*(cell-4)/m), max(1, r.Dy()*(cell-4)/m)
		ox, oy := (n%cols)*cell+(cell-w)/2, (n/cols)*cell+(cell-h)/2
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				sheet.Set(ox+x, oy+y, img.At(r.Min.X+x*r.Dx()/w, r.Min.Y+y*r.Dy()/h))
			}
		}
		cellLabel(sheet, (n%cols)*cell, (n/cols)*cell, n)
		cells = append(cells, map[string]any{"cell": n, "assetID": id.String(), "name": name, "revision": rev})
	}
	if b.cancelled(generation) {
		return nil, errCancelled
	}
	result, err := encodePNG(sheet)
	if err != nil {
		return nil, err
	}
	return &agent.Output{OK: true, Value: map[string]any{"offset": offset, "total": total, "columns": cols, "cellSize": cell, "cells": cells}, Images: []agent.Image{result}}, nil
}
func (b *Bridge) captureView(a map[string]any) (*agent.Output, error) {
	view, _ := a["view"].(string)
	if view != "game" && view != "scene" {
		return nil, errors.New("choose game or scene")
	}
	var raw []byte
	var tick uint64
	var path string
	var err error
	gen := b.panelGeneration.Load()
	if e := b.main(func() {
		if b.cancelled(gen) {
			err = errCancelled
			return
		}
		if b.cfg.Capture == nil {
			err = errors.New("view capture unavailable")
			return
		}
		raw, tick, path, err = b.cfg.Capture(view)
	}); e != nil {
		return nil, e
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 5<<20 {
		return nil, errors.New("capture exceeds 5 MiB; reduce viewport size")
	}
	return &agent.Output{OK: true, Value: map[string]any{"view": view, "tick": tick, "scene": path}, Images: []agent.Image{{Mime: "image/png", Data: raw}}}, nil
}

func (b *Bridge) selectCells(a map[string]any, gen uint64) ([]any, int, int, error) {
	offset, err := agent.Args(a).Int("offset", 0)
	if err != nil || offset < 0 {
		return nil, 0, 0, errors.New("invalid offset")
	}
	selectors := 0
	for _, k := range []string{"assets", "folder", "sequence"} {
		if _, ok := a[k]; ok {
			selectors++
		}
	}
	if selectors != 1 {
		return nil, 0, 0, errors.New("provide exactly one of assets, folder or sequence")
	}
	var values []any
	if raw, ok := a["assets"]; ok {
		var yes bool
		values, yes = raw.([]any)
		if !yes || len(values) < 1 || len(values) > 12 {
			return nil, 0, 0, errors.New("select 1 to 12 assets")
		}
	}
	if raw, ok := a["folder"]; ok {
		folder, _ := raw.(string)
		if folder == "" || !fs.ValidPath(folder) {
			return nil, 0, 0, errors.New("choose a project-relative folder")
		}
		err = b.main(func() {
			if b.ctx.Pipeline == nil {
				return
			}
			paths := []string{}
			entries := b.ctx.Pipeline.Assets()
			if t, ok := b.ctx.Thumbs.(interface{ SourceFiles() []pipeline.AssetInfo }); ok {
				entries = append(entries, t.SourceFiles()...)
			}
			for _, it := range entries {
				if path.Dir(it.Path) == folder && !strings.Contains(it.Path, pipeline.SubSep) {
					paths = append(paths, it.Path)
				}
			}
			sort.Strings(paths)
			for _, name := range paths {
				values = append(values, name)
			}
		})
		if err != nil {
			return nil, 0, 0, err
		}
	}
	if raw, ok := a["sequence"]; ok {
		name, _ := raw.(string)
		id, _, _, _, e := b.assetPixels(name, "preview")
		if e != nil {
			return nil, 0, 0, e
		}
		until := time.Now().Add(5 * time.Second)
		for {
			if b.cancelled(gen) {
				return nil, 0, 0, errCancelled
			}
			var frameIDs []ids.AssetID
			err = b.main(func() {
				s, ok := b.ctx.Thumbs.(interface {
					FrameIDs(ids.AssetID) ([]ids.AssetID, error)
				})
				if !ok {
					e = errors.New("frame preview unavailable")
					return
				}
				frameIDs, e = s.FrameIDs(id)
			})
			if err != nil {
				return nil, 0, 0, err
			}
			if e == nil {
				for _, v := range frameIDs {
					values = append(values, v.String())
				}
				break
			}
			if !errors.Is(e, preview.ErrPending) || time.Now().After(until) {
				return nil, 0, 0, e
			}
			time.Sleep(35 * time.Millisecond)
		}
	}
	if offset >= len(values) {
		return nil, len(values), offset, errors.New("no assets at this offset")
	}
	return values[offset : offset+min(12, len(values)-offset)], len(values), offset, nil
}
func cellLabel(img *image.NRGBA, x, y, n int) {
	glyphs := [10][5]byte{{7, 5, 5, 5, 7}, {2, 6, 2, 2, 7}, {7, 1, 7, 4, 7}, {7, 1, 7, 1, 7}, {5, 5, 7, 1, 1}, {7, 4, 7, 1, 7}, {7, 4, 7, 5, 7}, {7, 1, 1, 1, 1}, {7, 5, 7, 5, 7}, {7, 5, 7, 1, 7}}
	for i, c := range strconv.Itoa(n) {
		g := glyphs[c-'0']
		for dy, row := range g {
			for dx := 0; dx < 3; dx++ {
				v := uint8(35)
				if row&(4>>dx) != 0 {
					v = 255
				}
				img.SetNRGBA(x+2+i*4+dx, y+2+dy, color.NRGBA{v, v, v, 255})
			}
		}
	}
}
