package editor

import (
	"fmt"
	"path"
	"strings"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/modules/particles"
	"aego/scene/sceneio"
)

func (p *adapter) registerEffectCreate() error {
	ed, r := p.ed, p.reg
	if r.CommandsAvailable() {
		if err := r.Command(command.Desc{
			ID: "asset.new_effect", Name: "New Effect…", Category: "Assets", Order: 225, Icon: "particles.system",
			Label:   func() string { return i18n.Tr("editor.particles.new_effect", "New Effect…") },
			Enabled: func() bool { return ed.Pipeline != nil && ed.Docs != nil && ed.CreateAssetSources != nil },
			Run:     func() error { return p.askNewEffect(ed.AssetCreateDirectory("assets")) },
		}); err != nil {
			return err
		}
	}
	return r.CreateAction(context.CreateAction{
		Key: "particles.create-effect", Name: i18n.Tr("editor.particles.new_effect_file", "Animated Effect (.aeart + .aepreset)…"),
		Category: i18n.Tr("editor.art.create_category", "Assets"), Icon: "particles.system",
		Doc:         i18n.Tr("editor.particles.new_effect_help", "Create an editable eight-frame drawing and its linked particle preset."),
		Run:         func(ids.NodeID) error { return p.askNewEffect(ed.AssetCreateDirectory("assets")) },
		RunInFolder: p.askNewEffect,
	})
}

func effectTemplateName(template art.EffectTemplate) string {
	if i18n.Default().Language() == i18n.Russian {
		return template.NameRU
	}
	return template.NameEN
}

func (p *adapter) askNewEffect(dir string) error {
	ed := p.ed
	if ed == nil || ed.Pipeline == nil || ed.Docs == nil || ed.Project == nil || ed.Types == nil || ed.CreateAssetSources == nil {
		return fmt.Errorf("%s", i18n.Tr("editor.particles.effect_services", "This host does not support atomic effect creation."))
	}
	pl, docs, project, types, create := ed.Pipeline, ed.Docs, ed.Project, ed.Types, ed.CreateAssetSources
	templates := art.EffectTemplates()
	names := make([]string, len(templates))
	initial := ""
	for i, template := range templates {
		names[i] = effectTemplateName(template)
		if template.ID == "explosion" {
			initial = names[i]
		}
	}
	rel := ""
	for n := 1; ; n++ {
		stem := "effect"
		if n > 1 {
			stem += fmt.Sprint(n)
		}
		base := path.Join(dir, stem)
		if !assetops.Exists(ed, base+art.SourceExtension) && !assetops.Exists(ed, base+art.SourceExtension+pipeline.MetaSuffix) &&
			!assetops.Exists(ed, base+".aepreset") && !assetops.Exists(ed, base+".aepreset"+pipeline.MetaSuffix) {
			rel = base + art.SourceExtension
			break
		}
	}
	ed.Ask(context.Question{
		Title:   i18n.Tr("editor.particles.new_effect", "New Effect…"),
		Message: i18n.Tr("editor.particles.effect_pair", "Creates a drawing and a linked particle preset beside it."),
		Fields:  []string{i18n.Tr("editor.art.source_path", "Source path"), i18n.Tr("editor.particles.effect_template", "Effect template")},
		Values:  []string{rel, initial},
		Choices: [][]string{nil, names},
		Buttons: []string{i18n.Tr("editor.art.create", "Create"), i18n.Tr("editor.art.cancel", "Cancel")},
		Submit: func(values []string) {
			if ed.Pipeline != pl || ed.Docs != docs || ed.Project != project || ed.Types != types {
				ed.Notify(context.LevelError, i18n.Tr("editor.art.import_project_changed", "The project changed before the import completed."))
				return
			}
			if len(values) != 2 {
				return
			}
			templateID := ""
			for i, template := range templates {
				if values[1] == names[i] {
					templateID = template.ID
					break
				}
			}
			edits, err := effectSourceEdits(ed, strings.TrimSpace(values[0]), templateID)
			if err == nil {
				var created []ids.AssetID
				created, err = create(edits)
				if err == nil {
					p.reg.SelectWorkspace(workspace.VFX)
					// Open both existing authoring panels. The scene and its
					// shared materials are untouched until an explicit assignment.
					_, err = ed.OpenAsset(created[1])
					if err == nil {
						_, err = ed.OpenAsset(created[0])
					}
				}
			}
			if err != nil {
				ed.Notify(context.LevelError, err.Error())
			}
		},
	})
	return nil
}

// effectSourceEdits prepares source identities before either document exists.
// Stable frame references therefore survive import, creation Undo/Redo, save
// and restart. The existing pipeline imports the whole group transactionally.
func effectSourceEdits(ed *context.Editor, rel, templateID string) ([]pipeline.SourceEdit, error) {
	if ed == nil || ed.Pipeline == nil || ed.Types == nil {
		return nil, fmt.Errorf("effect: project services are unavailable")
	}
	if path.Ext(rel) != art.SourceExtension {
		return nil, fmt.Errorf("%s", i18n.Tr("editor.particles.effect_extension", "Effect drawing path must end with .aeart."))
	}
	template, ok := art.EffectTemplateOf(templateID)
	if !ok {
		return nil, fmt.Errorf("%s", i18n.Tr("editor.particles.effect_unknown", "Unknown effect template: {id}", i18n.Str("id", templateID)))
	}
	drawing, err := art.NewEffectDocument(template.ID, art.DefaultLimits())
	if err != nil {
		return nil, err
	}
	defer drawing.Close()
	artBytes, err := drawing.Encode()
	if err != nil {
		return nil, err
	}
	artMeta, err := ed.Pipeline.NewSourceMeta(rel)
	if err != nil {
		return nil, err
	}
	presetPath := strings.TrimSuffix(rel, art.SourceExtension) + ".aepreset"
	presetMeta, err := ed.Pipeline.NewSourceMeta(presetPath)
	if err != nil {
		return nil, err
	}
	preset, err := particles.PresetSource(template.ParticlePreset)
	if err != nil {
		return nil, err
	}
	node, component, err := particleNode(preset, presetPath)
	if err != nil {
		return nil, err
	}
	info := drawing.Info()
	frames := make([]aetext.Value, len(info.Frames))
	for i, frame := range info.Frames {
		id := artsource.StableID(artMeta.ID, artsource.FrameName(frame.ID, art.Albedo))
		frames[i] = aetext.AssetRef(id.String())
	}
	component.SetField("sprite", frames[0])
	component.SetField("frames", aetext.List(frames...))
	component.SetField("frameFPS", aetext.Float32(1000/float32(info.Frames[0].DurationMS)))
	td, ok := ed.Types.Schema().ByName(particles.NameSystem)
	if !ok {
		return nil, fmt.Errorf("%s", i18n.Tr("editor.particles.effect_component", "The particle component is unavailable."))
	}
	if err := sceneio.ValidatePresetFields(ed.Types.Schema(), td, component); err != nil {
		return nil, err
	}
	return []pipeline.SourceEdit{
		{Path: rel, After: artBytes},
		{Path: rel + pipeline.MetaSuffix, After: pipeline.WriteMeta(artMeta)},
		{Path: presetPath, After: aetext.Write(node)},
		{Path: presetPath + pipeline.MetaSuffix, After: pipeline.WriteMeta(presetMeta)},
	}, nil
}
