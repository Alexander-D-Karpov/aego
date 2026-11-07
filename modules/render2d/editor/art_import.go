package editor

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"aego/art"
	"aego/asset/pipeline"
	"aego/editor/core/assetops"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/ext"
	"aego/engine/i18n"
)

func registerAsepriteImport(r *ext.Registry) error {
	if !r.CommandsAvailable() {
		return nil
	}
	ed := r.Editor()
	return r.Command(command.Desc{
		ID: "asset.import_aseprite", Name: "Import Aseprite…", Category: "Assets", Order: 221, Icon: "file.art",
		Label:   func() string { return i18n.Tr("editor.art.import_aseprite", "Import Aseprite…") },
		Enabled: func() bool { return ed != nil && ed.Pipeline != nil && ed.Docs != nil },
		Run: func() error {
			pl, docs, project := ed.Pipeline, ed.Docs, ed.Project
			ed.Ask(context.Question{
				Title:   i18n.Tr("editor.art.import_aseprite", "Import Aseprite…"),
				Message: i18n.Tr("editor.art.import_aseprite_hint", "The original file is not overwritten."),
				Fields:  []string{i18n.Tr("editor.art.import_aseprite_input", "Aseprite file path"), i18n.Tr("editor.art.import_aseprite_output", "New project source path")},
				Values:  []string{"", assetops.UniqueFile(ed, "assets", "imported", ".aeart")},
				Buttons: []string{i18n.Tr("editor.art.import_action", "Import"), i18n.Tr("editor.art.cancel", "Cancel")},
				Submit: func(values []string) {
					if len(values) != 2 {
						return
					}
					fail := func(err error) { ed.Notify(context.LevelError, err.Error()) }
					if ed.Pipeline != pl || ed.Docs != docs || ed.Project != project {
						fail(fmt.Errorf("%s", i18n.Tr("editor.art.import_project_changed", "The project changed before the import completed.")))
						return
					}
					source, target := strings.TrimSpace(values[0]), strings.TrimSpace(values[1])
					switch strings.ToLower(filepath.Ext(source)) {
					case ".ase", ".aseprite":
					default:
						fail(fmt.Errorf("%s", i18n.Tr("editor.art.import_aseprite_type", "The input file must use .ase or .aseprite.")))
						return
					}
					if path.Ext(target) != ".aeart" {
						fail(fmt.Errorf("%s", i18n.Tr("editor.art.import_aseprite_target", "The new source must use .aeart.")))
						return
					}
					if !filepath.IsAbs(source) {
						source = docs.Abs(source)
					}
					data, err := readAsepriteImport(source)
					if err != nil {
						fail(err)
						return
					}
					d, notes, err := art.ImportAseprite(data, art.Limits{})
					if err != nil {
						fail(err)
						if len(notes) > 0 {
							ed.Ask(context.Question{Title: i18n.Tr("editor.art.import_diagnostics", "Import diagnostics"), Message: err.Error() + "\n\n" + asepriteDiagnostics(notes), Buttons: []string{i18n.Tr("editor.art.close", "Close")}})
						}
						return
					}
					defer d.Close()
					encoded, err := d.Encode()
					if err != nil {
						fail(err)
						return
					}
					publish := func() {
						if ed.Pipeline != pl || ed.Docs != docs || ed.Project != project {
							fail(fmt.Errorf("%s", i18n.Tr("editor.art.import_project_changed", "The project changed before the import completed.")))
							return
						}
						// Use the guarded project source transaction: Before=nil means
						// the destination must still be absent at publication time.
						batch, err := pl.ApplySources([]pipeline.SourceEdit{{Path: target, After: encoded}}, false)
						if err != nil {
							fail(err)
							return
						}
						ed.ApplyBatch(batch)
						id, ok := pl.Lookup(target)
						if !ok {
							fail(fmt.Errorf("art: imported source is not in the asset index"))
							return
						}
						if _, err = ed.OpenAsset(id); err != nil {
							fail(err)
						}
					}
					if len(notes) == 0 {
						publish()
						return
					}
					message := i18n.Tr("editor.art.import_loss_warning", "Some metadata will not be retained. Review the diagnostics before creating the new source:") + "\n\n" + asepriteDiagnostics(notes)
					ed.Ask(context.Question{
						Title: i18n.Tr("editor.art.import_diagnostics", "Import diagnostics"), Message: message,
						Buttons: []string{i18n.Tr("editor.art.import_anyway", "Import Anyway"), i18n.Tr("editor.art.cancel", "Cancel")},
						Then: func(choice int) {
							if choice == 0 {
								publish()
							}
						},
					})
				},
			})
			return nil
		},
	})
}

func readAsepriteImport(name string) ([]byte, error) {
	data, err := pipeline.ReadExternalFile(name, art.DefaultLimits().MaxFileBytes)
	if errors.Is(err, pipeline.ErrExternalFileLimit) {
		return nil, fmt.Errorf("%s", i18n.Tr("editor.art.import_aseprite_size", "The input must be a regular Aseprite file within the import size limit."))
	}
	return data, err
}

func asepriteDiagnostics(notes []art.ImportDiagnostic) string {
	lines := make([]string, 0, len(notes))
	for _, note := range notes {
		message := note.Message
		switch note.Code {
		case "layer-uuid":
			message = i18n.Tr("editor.art.ase_uuid", "Aseprite's layer UUID is replaced by an Art Document ID.")
		case "tag-repeat":
			message = i18n.Tr("editor.art.ase_repeat", "A finite tag repeat count is not retained; the imported tag loops.")
		case "slice-ninepatch":
			message = i18n.Tr("editor.art.ase_ninepatch", "The nine-patch center is not retained; slice bounds and pivot are retained.")
		case "metadata-skipped":
			message = i18n.Tr("editor.art.ase_metadata", "Metadata in this chunk is not imported.")
		}
		lines = append(lines, i18n.Tr("editor.art.ase_diagnostic", "{code} (frame {frame}, chunk {chunk}): {message}",
			i18n.Str("code", note.Code), i18n.Str("frame", fmt.Sprint(note.Frame+1)), i18n.Str("chunk", fmt.Sprintf("%#x", note.Chunk)), i18n.Str("message", message)))
	}
	return strings.Join(lines, "\n")
}
