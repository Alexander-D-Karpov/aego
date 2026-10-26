package main

import (
	"aego/app/agent"
	"aego/app/docstool"
	"aego/engine/docs"
	updatemod "aego/modules/update"
	"slices"
	"strings"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/engine/settings"
	"aego/schema"
	"aego/script/bind"
	"aego/script/diag"
)

// dataDir holds the texts the engine reads from its registries rather than from Tr calls: component and field names and descriptions, enum values, settings and script diagnostics.
const dataDir = "engine/i18n/data"

// Data lists the registry texts of every builtin module under their catalog keys (D348).
func Data() (map[string]*Key, error) {
	types, err := assembly.NewTypes()
	if err != nil {
		return nil, err
	}
	man, err := builtin.Default().Load(types, bind.New(), assembly.Options{})
	if err != nil {
		return nil, err
	}
	out := map[string]*Key{}
	add := func(id, en, pos string) {
		if en = strings.TrimSpace(en); en != "" {
			if _, dup := out[id]; !dup {
				out[id] = &Key{ID: id, En: en, Pos: pos}
			}
		}
	}
	sreg := types.Schema()
	for _, td := range append(sreg.Components(), sreg.DataTypes()...) {
		if td.Hidden && td.Doc == "" {
			continue
		}
		pos := "schema " + td.FullName
		add(schema.TypeKey(td), schema.EnglishTitle(td), pos)
		add(schema.TypeKey(td)+".doc", td.Doc, pos)
		for i := range td.Fields {
			fd := &td.Fields[i]
			if !fd.Flags.Has(schema.FlagEditor) {
				continue
			}
			add(schema.FieldKey(td, fd), schema.Label(fd), pos)
			add(schema.FieldKey(td, fd)+".doc", fd.Doc, pos)
			if c := fd.Hints.Category; c != "" {
				add("schema.category."+c, c, pos)
			}
			if fd.EnumName != "" {
				for _, ev := range fd.Enum {
					add(schema.EnumKey(fd.EnumName, ev.Name), ev.Name, "enum "+fd.EnumName)
				}
			}
		}
	}
	for _, c := range agent.Commands() {
		add(agent.DocKey(c), c.Doc, "agent command "+c.Name)
	}
	for _, c := range settings.Categories {
		add("setting.category."+c, c, "settings")
	}
	settingDescs := append(slices.Clone(man.Settings), updatemod.Settings()...)
	for i := range settingDescs {
		d := &settingDescs[i]
		pos := "setting " + d.ID
		add("setting."+d.ID, d.Label(), pos)
		add("setting."+d.ID+".doc", d.Doc, pos)
		for _, c := range d.Choices {
			if c.Name != "" {
				en := c.Label
				if en == "" {
					en = c.Name
				}
				add("setting."+d.ID+"."+c.Name, en, pos)
			}
		}
	}
	reg, err := docstool.Registry(nil, nil)
	if err != nil {
		return nil, err
	}
	for _, g := range docs.Groups() {
		add("docs.group."+g.ID, g.Title, "docs group")
	}
	add("docs.fix", "Fix:", "docs")
	add("docs.module", "Components of the %s module.", "docs module page")
	add("docs.default_block", "Default block", "project section page heading")
	for _, e := range reg.All() {
		switch e.Kind {
		case docs.Function, docs.Method, docs.Signal, docs.Const, docs.Enum:
			add(docs.APIKey(e.ID), e.Body, "api "+e.ID)
		case docs.Field:
			if strings.HasPrefix(e.Parent, docs.SectionsPrefix) && !strings.HasPrefix(e.ID, docs.SettingPrefix) {
				add("docs.field."+e.ID, e.Body, "project section field")
			}
		case docs.Type:
			if e.Component == nil {
				add(docs.APIKey(e.ID), e.Body, "api "+e.ID)
			}
		case docs.Page:
			if e.Location.Source == "" {
				lead, _, _ := strings.Cut(e.Body, "\n\n")
				add("docs.page."+e.ID, e.Title, "generated page")
				add("docs.page."+e.ID+".doc", lead, "generated page")
			}
		case docs.Module:
			if strings.HasPrefix(e.ID, docs.ModulePrefix) {
				add("docs.module."+e.Owner, e.Title, "module name")
			}
		}
	}
	for _, c := range diag.Codes() {
		h, _ := diag.Explain(c)
		add("diag."+c, h.What, "diag "+c)
		add("diag."+c+".fix", h.Fix, "diag "+c)
		if ex, ok := diag.Example(c); ok && ex.Why != "" {
			add("diag."+c+".why", ex.Why, "diag "+c+" without an example")
		}
	}
	return out, nil
}
