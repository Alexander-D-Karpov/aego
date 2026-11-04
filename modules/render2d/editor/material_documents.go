package editor

import (
	"fmt"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/render/material"
	"aego/ui/widgets"
)

func materialDocumentHandlers(r *ext.Registry) error {
	ed := r.Editor()
	inspector := r.DocumentInspector()
	code := func(d *document.AssetDocument) error {
		if asset.IsBuiltin(d.Asset) {
			return inspector(d)
		}
		if ed.Code == nil {
			return fmt.Errorf("the shader source editor is unavailable")
		}
		ed.Code.Goto(context.At(d.Path, 1, 1))
		return nil
	}
	for _, h := range []document.Handler{
		{ID: "render2d.material", Name: i18n.T("editor.document.material", "Material"), Icon: "comp.sprite", Kinds: []asset.ArtifactKind{asset.ArtMaterial}, Extensions: []string{".aemat"}, Capabilities: document.View | document.EditSource | document.Preview | document.RevealSource, TextSource: true, Open: inspector},
		{ID: "render2d.shader", Name: i18n.T("editor.document.shader", "Shader"), Icon: "file.script", Kinds: []asset.ArtifactKind{asset.ArtShader}, Extensions: []string{".aeshader"}, Capabilities: document.View | document.EditSource | document.RevealSource, TextSource: true, Open: code},
		{ID: "render2d.shader-include", Name: i18n.T("editor.document.shader_include", "Shader Include"), Icon: "file.script", Kinds: []asset.ArtifactKind{asset.ArtShaderInclude}, Extensions: []string{".glsl", ".vert", ".frag"}, Capabilities: document.View | document.EditSource | document.RevealSource, TextSource: true, Open: code},
	} {
		if err := r.DocumentHandler(h); err != nil {
			return err
		}
	}
	return nil
}

func (s *materialsEditor) drawBuiltin(ed *context.Editor, id ids.AssetID) {
	b, _ := asset.LookupBuiltin(id)
	widgets.Label(ed.UI, b.Name)
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.document.builtin_readonly", "Built-in asset · read-only. Create a project material to author changes."), false)
	if ed.Loader == nil {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.document.no_loader", "The asset loader is unavailable."), true)
		return
	}
	var sh *material.Shader
	if b.Kind == asset.ArtMaterial {
		m, ok := ed.Loader.Material(id)
		if !ok {
			widgets.LabelWrapped(ed.UI, i18n.Tr("editor.document.builtin_unavailable", "The built-in asset could not be loaded."), true)
			return
		}
		sh = m.Shader()
		v := s.views[id]
		if v == nil {
			v = &materialView{}
			s.views[id] = v
		}
		v.seen = true
		if v.revision != m.Generation() {
			copy, err := m.CloneDefinition()
			if err != nil {
				widgets.LabelWrapped(ed.UI, err.Error(), true)
				return
			}
			v.preview.update(ed, copy, m.Generation(), 0)
			v.revision = m.Generation()
		}
		v.preview.draw(ed)
	} else {
		sh, _ = ed.Loader.Shader(id)
	}
	if sh == nil {
		return
	}
	widgets.LabelDim(ed.UI, i18n.Tr("editor.material.domain", "Domain: {domain}", i18n.Str("domain", sh.Domain.String())))
	for _, p := range sh.Schema.Params {
		widgets.LabelDim(ed.UI, p.Name+" : "+p.Kind.GLSL())
	}
}
