package editor

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"aego/art"
	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/ext"
	"aego/engine/i18n"
)

func registerArtCreate(r *ext.Registry) error {
	if !r.CommandsAvailable() {
		return nil
	}
	ed := r.Editor()
	if err := r.Command(command.Desc{
		ID: "asset.new_art", Name: "New Pixel Art…", Category: "Assets", Order: 220, Icon: "file.art",
		Label:   func() string { return i18n.Tr("editor.art.new", "New Pixel Art…") },
		Enabled: func() bool { return ed != nil && ed.Pipeline != nil && ed.Docs != nil },
		Run:     func() error { return askNewArt(ed, "assets") },
	}); err != nil {
		return err
	}
	if err := r.CreateAction(context.CreateAction{
		Key: "render2d.pixel_art", Name: i18n.Tr("editor.art.new_file", "Pixel Art (.aeart)…"),
		Category: i18n.Tr("editor.art.create_category", "Assets"), Icon: "file.art",
		Doc:         i18n.Tr("editor.art.create_help", "Create an editable pixel art and animation source."),
		Run:         func(ids.NodeID) error { return askNewArt(ed, ed.AssetCreateDirectory("assets")) },
		RunInFolder: func(dir string) error { return askNewArt(ed, dir) },
	}); err != nil {
		return err
	}
	return r.CreateAction(context.CreateAction{
		Key: "render2d.png", Name: i18n.Tr("editor.art.png_file", "Blank PNG (.png, view only)…"),
		Category: i18n.Tr("editor.assets.create_category", "Files"), Icon: "file.texture",
		Doc:         i18n.Tr("editor.art.png_help", "Create a blank PNG. Choose Pixel Art to edit pixels in Aego."),
		Run:         func(ids.NodeID) error { return askNewPNG(ed, ed.AssetCreateDirectory("assets")) },
		RunInFolder: func(dir string) error { return askNewPNG(ed, dir) },
	})
}

func askNewArt(ed *context.Editor, dir string) error {
	if ed == nil || ed.Pipeline == nil || ed.Docs == nil {
		return fmt.Errorf("art: project asset services are unavailable")
	}
	ed.Ask(context.Question{
		Title:   i18n.Tr("editor.art.new", "New Pixel Art…"),
		Fields:  []string{i18n.Tr("editor.art.source_path", "Source path"), i18n.Tr("editor.art.width", "Width"), i18n.Tr("editor.art.height_pixels", "Height"), i18n.Tr("editor.art.color_mode", "Color mode: rgba / grayscale / indexed")},
		Values:  []string{assetops.UniqueFile(ed, dir, "sprite", ".aeart"), "32", "32", "rgba"},
		Buttons: []string{i18n.Tr("editor.art.create", "Create"), i18n.Tr("editor.art.cancel", "Cancel")},
		Submit: func(values []string) {
			if len(values) != 4 {
				return
			}
			rel := strings.TrimSpace(values[0])
			fail := func(err error) { ed.Notify(context.LevelError, err.Error()) }
			if path.Ext(rel) != ".aeart" {
				fail(fmt.Errorf("art: source must use .aeart"))
				return
			}
			data, err := blankArtSource(values[1], values[2], values[3])
			if err != nil {
				fail(err)
				return
			}
			create := ed.CreateAsset
			if create == nil {
				create = func(rel string, data []byte) (ids.AssetID, error) { return assetops.CreateFile(ed, rel, data) }
			}
			id, err := create(rel, data)
			if err != nil {
				fail(err)
				return
			}
			if _, err = ed.OpenAsset(id); err != nil {
				fail(err)
			}
		},
	})
	return nil
}

func blankArtSource(width, height, mode string) ([]byte, error) {
	w, err := strconv.Atoi(strings.TrimSpace(width))
	if err != nil {
		return nil, err
	}
	h, err := strconv.Atoi(strings.TrimSpace(height))
	if err != nil {
		return nil, err
	}
	d, err := art.New(w, h, art.Mode(strings.ToLower(strings.TrimSpace(mode))), art.DefaultLimits())
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return d.Encode()
}

func askNewPNG(ed *context.Editor, dir string) error {
	if ed == nil || ed.Pipeline == nil || ed.Docs == nil {
		return fmt.Errorf("image: project asset services are unavailable")
	}
	ed.Ask(context.Question{
		Title:   i18n.Tr("editor.art.png_file", "Blank PNG (.png, view only)…"),
		Message: i18n.Tr("editor.art.png_prompt", "For editable pixels, choose Pixel Art."),
		Fields:  []string{i18n.Tr("editor.art.source_path", "Source path"), i18n.Tr("editor.art.width", "Width"), i18n.Tr("editor.art.height_pixels", "Height")},
		Values:  []string{assetops.UniqueFile(ed, dir, "image", ".png"), "32", "32"},
		Buttons: []string{i18n.Tr("editor.art.create", "Create"), i18n.Tr("editor.art.cancel", "Cancel")},
		Submit: func(values []string) {
			if len(values) != 3 {
				return
			}
			fail := func(err error) { ed.Notify(context.LevelError, err.Error()) }
			rel := strings.TrimSpace(values[0])
			if !strings.EqualFold(path.Ext(rel), ".png") {
				fail(fmt.Errorf("image: source must use .png"))
				return
			}
			w, err := strconv.Atoi(strings.TrimSpace(values[1]))
			if err != nil {
				fail(err)
				return
			}
			h, err := strconv.Atoi(strings.TrimSpace(values[2]))
			if err != nil {
				fail(err)
				return
			}
			d, err := art.New(w, h, art.RGBA, art.DefaultLimits())
			if err != nil {
				fail(err)
				return
			}
			defer d.Close()
			data, err := d.PNG(d.Info().Frames[0].ID, art.Albedo)
			if err != nil {
				fail(err)
				return
			}
			create := ed.CreateAsset
			if create == nil {
				create = func(rel string, data []byte) (ids.AssetID, error) { return assetops.CreateFile(ed, rel, data) }
			}
			id, err := create(rel, data)
			if err == nil {
				_, err = ed.OpenAsset(id)
			}
			if err != nil {
				fail(err)
			}
		},
	})
	return nil
}
