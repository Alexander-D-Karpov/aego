package editor

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"aego/art"
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
	return r.Command(command.Desc{
		ID: "asset.new_art", Name: "New Pixel Art…", Category: "Assets", Order: 220, Icon: "file.art",
		Label:   func() string { return i18n.Tr("editor.art.new", "New Pixel Art…") },
		Enabled: func() bool { return ed != nil && ed.Pipeline != nil && ed.Docs != nil },
		Run: func() error {
			ed.Ask(context.Question{
				Title:   i18n.Tr("editor.art.new", "New Pixel Art…"),
				Fields:  []string{i18n.Tr("editor.art.source_path", "Source path"), i18n.Tr("editor.art.width", "Width"), i18n.Tr("editor.art.height_pixels", "Height"), i18n.Tr("editor.art.color_mode", "Color mode: rgba / grayscale / indexed")},
				Values:  []string{assetops.UniqueFile(ed, "assets", "sprite", ".aeart"), "32", "32", "rgba"},
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
					d, err := art.New(w, h, art.Mode(strings.ToLower(strings.TrimSpace(values[3]))), art.DefaultLimits())
					if err != nil {
						fail(err)
						return
					}
					defer d.Close()
					data, err := d.Encode()
					if err != nil {
						fail(err)
						return
					}
					id, err := assetops.CreateFile(ed, rel, data)
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
		},
	})
}
