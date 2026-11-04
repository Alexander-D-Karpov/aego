package editor

import (
	"slices"
	"unsafe"

	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/engine/i18n"
	"aego/modules/render2d"
	"aego/render/material"
	"aego/render/shader"
	"aego/scene"
	"aego/schema"
	"aego/ui/widgets"
)

type overrideInspector struct {
	tx    *history.Transaction
	doc   *document.SceneDocument
	node  ids.NodeID
	field string
	seen  bool
}

func (i *overrideInspector) finish() {
	if i.tx != nil {
		i.tx.Commit()
		i.tx = nil
	}
	i.field = ""
}
func (i *overrideInspector) tick() {
	if !i.seen {
		i.finish()
	}
	i.seen = false
}

func (i *overrideInspector) Draw(ed *context.Editor, nodes []ids.NodeID, defaults func()) {
	i.seen = true
	if len(nodes) != 1 || ed.Doc() == nil {
		i.finish()
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.select_one", "Select one node to edit material parameters."), false)
		defaults()
		return
	}
	if i.doc != ed.Doc() || i.node != nodes[0] {
		i.finish()
		i.doc, i.node = ed.Doc(), nodes[0]
	}
	sc := i.doc.Scene()
	h := sc.Handle(nodes[0])
	o, ok := scene.Get[render2d.MaterialOverride](sc, h)
	if !ok {
		return
	}
	mat, err := render2d.NodeMaterial(sc, h)
	if err != nil {
		widgets.LabelWrapped(ed.UI, err.Error(), true)
		defaults()
		return
	}
	td, _ := sc.Registry().ByName("engine.MaterialOverride")
	fd, _ := td.Field("values")
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.node_inheritance", "Inherited: shader default → material value. Changes below belong to this node."), false)
	for pidx := range mat.Shader().Schema.Params {
		p := &mat.Shader().Schema.Params[pidx]
		if !p.Instance || p.Kind == shader.KindTexture || p.Hidden {
			continue
		}
		ed.UI.PushID("override:" + p.Name)
		value, err := o.Effective(mat, p.Name)
		if err != nil {
			widgets.LabelWrapped(ed.UI, err.Error(), true)
		} else {
			change := parameterControl(ed, p, &value)
			i.change(ed, o, mat, td, fd, p.Name, &value, false, change)
		}
		if o.ExplicitFor(mat, p.Name) {
			if widgets.Button(ed.UI, i18n.Tr("editor.material.inherit_material", "Inherit material value")) {
				i.change(ed, o, mat, td, fd, p.Name, nil, true, oneEdit())
			}
		} else {
			widgets.LabelDim(ed.UI, i18n.Tr("editor.material.inherited", "Inherited"))
		}
		ed.UI.PopID()
	}
	for _, value := range o.Values {
		if _, ok := mat.Shader().Schema.Param(value.Name); !ok {
			widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.unknown", "Unknown parameter preserved: {parameter}", i18n.Str("parameter", value.Name)), false)
		}
	}
}

func (i *overrideInspector) change(ed *context.Editor, o *render2d.MaterialOverride, mat *material.Material, td *schema.TypeDescriptor, fd *schema.FieldDescriptor, name string, value *schema.Value, reset bool, edit widgets.Edit) {
	if edit.Begin || (edit.Changed && i.field != name) {
		i.finish()
		i.tx = i.doc.History.Begin(i18n.Tr("editor.material.edit_override", "Material override {parameter}", i18n.Str("parameter", name)))
		i.field = name
	}
	if edit.Changed {
		copy := render2d.MaterialOverride{Values: slices.Clone(o.Values)}
		if reset {
			copy.ResetFor(mat, name)
		} else if err := copy.Set(mat, name, *value); err != nil {
			ed.Notify(context.LevelError, err.Error())
			i.finish()
			return
		}
		var data schema.Value
		fd.Get(unsafe.Pointer(&copy), &data)
		if err := ed.Props().Set(i.tx, []ids.NodeID{i.node}, td.ID, fd.ID, data); err != nil {
			ed.Notify(context.LevelError, err.Error())
			i.finish()
		}
	}
	if edit.End {
		i.finish()
	}
}
