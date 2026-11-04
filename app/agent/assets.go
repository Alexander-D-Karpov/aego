package agent

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"path"
	"strings"

	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/format/pixelart"
	"aego/format/svg"
	"aego/modules/art"
	"aego/modules/render2d"
)

// paletteKeys name the art palette's colors in a grid when the call gives no palette: 0-9, then a-z.
const paletteKeys = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func assetCommands() []*Command {
	out := Arg{Name: "path", Type: "string", Doc: "PNG to write under assets/, such as assets/sprites/hero.png", Required: true, Pos: true}
	replace := Arg{Name: "replace", Type: "boolean", Doc: "draw over an existing PNG; its id and the ids of same-named frames stay"}
	return []*Command{
		paintCommand(out, replace),
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
	return s.drawnFrom(a, rel, img, preview, cells, flip, settings, notes, nil, false, nil)
}

func (s *Session) drawnFrom(a Args, rel string, img, preview image.Image, cells []pipeline.SubAsset, flip bool, settings pipeline.Settings, notes []string, guards []pipeline.SourceEdit, checkInput bool, input []byte) (*Output, error) {
	if preview == nil {
		preview = img
		if n := max(img.Bounds().Dx(), img.Bounds().Dy()); n < 128 {
			preview = pixelart.Upscale(img, 256/n)
		}
	}
	var buf, pv bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	b := preview.Bounds()
	if side := max(b.Dx(), b.Dy()); side > 256 {
		thumb := image.NewNRGBA(image.Rect(0, 0, max(1, b.Dx()*256/side), max(1, b.Dy()*256/side)))
		for y := 0; y < thumb.Bounds().Dy(); y++ {
			for x := 0; x < thumb.Bounds().Dx(); x++ {
				thumb.Set(x, y, preview.At(b.Min.X+x*b.Dx()/thumb.Bounds().Dx(), b.Min.Y+y*b.Dy()/thumb.Bounds().Dy()))
			}
		}
		preview = thumb
	}
	if err := png.Encode(&pv, preview); err != nil {
		return nil, err
	}
	view := &DrawnView{Path: rel, Size: [2]int{img.Bounds().Dx(), img.Bounds().Dy()}}
	edit, err := s.prepareDrawing(rel, buf.Bytes(), cells, flip, settings, a.Bool("replace"), view, guards, checkInput, input)
	if err != nil {
		return nil, err
	}
	out := &Output{OK: true, DryRun: a.Bool("dry-run"), Notes: notes, Images: []Image{{Mime: "image/png", Data: pv.Bytes()}}, Value: view, Drawing: edit}
	if out.DryRun {
		return out, nil
	}
	if s.host != nil {
		return nil, errors.New("drawings in the editor must be applied through a preview")
	}
	if _, err := s.Pipeline.ApplySources(edit.Sources, false); err != nil {
		return nil, err
	}
	out.Created, out.Files, out.Drawing = edit.Created, edit.Files, nil
	return out, nil
}
