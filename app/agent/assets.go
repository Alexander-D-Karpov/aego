package agent

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path"
	"strings"

	"aego/anim"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/format/pixelart"
	"aego/format/svg"
	"aego/modules/art"
	"aego/modules/render2d"
	"aego/project/refactor"
)

// paletteKeys name the art palette's colors in a grid when the call gives no palette: 0-9, then a-z.
const paletteKeys = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func assetCommands() []*Command {
	out := Arg{Name: "path", Type: "string", Doc: "PNG to write under assets/, such as assets/sprites/hero.png", Required: true, Pos: true}
	replace := Arg{Name: "replace", Type: "boolean", Doc: "draw over an existing PNG; its id and the ids of same-named frames stay"}
	return []*Command{
		{Name: "asset.pixels", Write: true, Run: (*Session).assetPixels,
			Doc: "draw a sprite as text: each character of grid is a pixel colored by palette; lines like idle: start frames of one size, which become an atlas with one sprite per frame and a looping flipbook clip. Answers with a preview image",
			Args: []Arg{out,
				{Name: "grid", Type: "string", Doc: "rows of characters, one per pixel; '.' is transparent; 'name:' lines start frames", Required: true},
				{Name: "palette", Type: "string", Doc: `characters to colors: {"r": "#d0312d", "s": "#f4c7a1"} or r=#d0312d s=#f4c7a1; without it, 0-9 and a-z are the colors of project.aego's art palette`},
				{Name: "scale", Type: "integer", Doc: "texture pixels per grid pixel; by default art.pixelsPerUnit scaled to the render pixelsPerUnit"},
				{Name: "flipbook", Type: "boolean", Doc: "with several frames, also write <name>.aeanim that plays them; on by default"},
				replace}},
		{Name: "asset.svg", Write: true, Run: (*Session).assetSVG,
			Doc: "draw a texture from SVG: path, rect, circle, ellipse, line, polyline, polygon, fill, stroke, opacity and transform; no filters, gradients or text. For tiles, UI and item icons. Answers with a preview image",
			Args: []Arg{out,
				{Name: "svg", Type: "string", Doc: "the SVG document, <svg viewBox=…>…</svg>", Required: true},
				{Name: "width", Type: "integer", Doc: "texture width in pixels, 64 by default"},
				{Name: "height", Type: "integer", Doc: "texture height in pixels, the width by default"},
				replace}},
	}
}

// art is the project's art section and the render pixels per unit, or zero values without them.
func (s *Session) art() (art.Art, render2d.Settings) {
	render := render2d.DefaultSettings()
	secs, err := s.sections()
	if err != nil {
		return art.Art{}, render
	}
	if v, ok := module.SectionValue[render2d.Settings](secs, render2d.SectionID); ok {
		render = *v
	}
	if v, ok := module.SectionValue[art.Art](secs, art.SectionID); ok {
		return *v, render
	}
	return art.Art{}, render
}

// DrawnView is what a drawing tool made.
type DrawnView struct {
	Path     string      `json:"path"`
	ID       string      `json:"id,omitempty"`
	Size     [2]int      `json:"size"`
	Frames   []FrameView `json:"frames,omitempty"`
	Flipbook string      `json:"flipbook,omitempty"`
}

type FrameView struct {
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
	Rect [4]int `json:"rect"`
}

func (s *Session) assetPixels(a Args) (*Output, error) {
	rel, err := pngPath(a.Str("path"))
	if err != nil {
		return nil, err
	}
	style, render := s.art()
	pal, err := pixelart.ParsePalette(a.Str("palette"))
	if err != nil {
		return nil, err
	}
	if !a.Has("palette") {
		if len(style.Palette) == 0 {
			return nil, errors.New("give a palette, or declare art { palette=[…] } in project.aego and use 0-9, a-z for its colors")
		}
		for i, c := range style.Palette {
			pal[rune(paletteKeys[i])] = c
		}
	}
	scale, err := a.Int("scale", style.Scale(render.PPU()))
	if err != nil {
		return nil, err
	}
	sheet, err := pixelart.Parse(a.Str("grid"), pal, scale)
	if err != nil {
		return nil, err
	}
	var notes []string
	if len(style.Palette) > 0 && a.Has("palette") {
		if off, opaque := pixelart.OffPalette(sheet.Image, style.Palette); off > 0 {
			notes = append(notes, fmt.Sprintf("%d of %d pixels are outside the art palette of project.aego", off, opaque))
		}
	}
	b := sheet.Image.Bounds()
	preview := pixelart.Upscale(sheet.Image, max(1, 256/max(b.Dx(), b.Dy())))
	base := strings.TrimSuffix(path.Base(rel), ".png")
	var cells []pipeline.SubAsset
	if len(sheet.Cells) > 1 {
		for i, c := range sheet.Cells {
			cells = append(cells, pipeline.SubAsset{Name: base + "_" + sheet.Names[i], Rect: [4]int{c.Min.X, c.Min.Y, c.Dx(), c.Dy()}})
		}
	}
	flip := len(cells) > 1 && (!a.Has("flipbook") || a.Bool("flipbook"))
	return s.drawn(a, rel, sheet.Image, preview, cells, flip, nil, notes)
}

func (s *Session) assetSVG(a Args) (*Output, error) {
	rel, err := pngPath(a.Str("path"))
	if err != nil {
		return nil, err
	}
	w, err := a.Int("width", 64)
	if err != nil {
		return nil, err
	}
	h, err := a.Int("height", w)
	if err != nil {
		return nil, err
	}
	r, err := svg.Render([]byte(a.Str("svg")), w, h)
	if err != nil {
		return nil, err
	}
	var notes []string
	if len(r.Skipped) > 0 {
		notes = append(notes, "not drawn, outside the supported SVG: "+strings.Join(r.Skipped, ", "))
	}
	preview := r.Image
	if m := max(w, h); m < 128 {
		preview = pixelart.Upscale(r.Image, 256/m)
	}
	linear := pipeline.Settings{"filter": aetext.Ident("linear")}
	return s.drawn(a, rel, r.Image, preview, nil, false, linear, notes)
}

func pngPath(rel string) (string, error) {
	rel = path.Clean(strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "./"))
	if !strings.HasPrefix(rel, "assets/") || path.Ext(rel) != ".png" {
		return "", errs.New(errs.VFSBadPath, "the path is a .png under assets/, such as assets/sprites/hero.png")
	}
	return rel, nil
}

// drawn writes the PNG, its frames and settings, and the flipbook clip; a dry run only answers with the preview.
func (s *Session) drawn(a Args, rel string, img, preview image.Image, cells []pipeline.SubAsset, flip bool, settings pipeline.Settings, notes []string) (*Output, error) {
	var buf, pv bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	if err := png.Encode(&pv, preview); err != nil {
		return nil, err
	}
	b := img.Bounds()
	view := DrawnView{Path: rel, Size: [2]int{b.Dx(), b.Dy()}}
	out := &Output{OK: true, Notes: notes, Images: []Image{{Mime: "image/png", Data: pv.Bytes()}}, Value: &view}
	animRel := strings.TrimSuffix(rel, ".png") + ".aeanim"
	if flip {
		view.Flipbook = animRel
	}
	for _, c := range cells {
		view.Frames = append(view.Frames, FrameView{Name: c.Name, Rect: c.Rect})
	}
	if a.Bool("dry-run") {
		out.DryRun = true
		return out, nil
	}
	if err := s.writable(); err != nil {
		return nil, err
	}
	var id ids.AssetID
	if s.exists(rel) {
		if !a.Bool("replace") {
			return nil, errs.New(errs.VFSBadPath, rel+" already exists; pass replace to draw over it")
		}
		before, err := os.ReadFile(s.Pipeline.Abs(rel))
		if err != nil {
			return nil, err
		}
		if _, err := s.save(rel, before, buf.Bytes()); err != nil {
			return nil, err
		}
		id, _ = s.Pipeline.Lookup(rel)
	} else {
		var err error
		if id, err = s.create(rel, buf.Bytes()); err != nil {
			return nil, err
		}
		out.Created = append(out.Created, id.String())
	}
	view.ID = id.String()
	meta, _ := s.Pipeline.MetaOf(id)
	if settings != nil {
		merged := meta.Settings.Clone()
		for k, v := range settings {
			merged[k] = v
		}
		if err := s.Pipeline.SetSettings(id, merged); err != nil {
			return nil, err
		}
	}
	if len(cells) > 0 || len(meta.Subs) > 0 {
		old := map[string]ids.AssetID{}
		for _, sub := range meta.Subs {
			old[sub.Name] = sub.ID
		}
		for i := range cells {
			if cells[i].ID = old[cells[i].Name]; cells[i].ID.IsZero() {
				cells[i].ID = ids.NewAssetID()
			}
			view.Frames[i].ID = cells[i].ID.String()
			out.Created = append(out.Created, cells[i].ID.String())
		}
		if err := s.Pipeline.SetSubs(id, cells); err != nil {
			return nil, err
		}
	}
	if _, err := refactor.Reimport(s.Pipeline, []string{rel}); err != nil {
		return nil, err
	}
	out.Files = append(out.Files, rel)
	if flip {
		frames := make([]ids.AssetID, len(cells))
		for i, c := range cells {
			frames[i] = c.ID
		}
		text := anim.FlipbookText(frames)
		if s.exists(animRel) {
			before, err := os.ReadFile(s.Pipeline.Abs(animRel))
			if err != nil {
				return nil, err
			}
			if _, err := s.save(animRel, before, text); err != nil {
				return nil, err
			}
		} else {
			cid, err := s.create(animRel, text)
			if err != nil {
				return nil, err
			}
			out.Created = append(out.Created, cid.String())
		}
		out.Files = append(out.Files, animRel)
	}
	return out, nil
}
