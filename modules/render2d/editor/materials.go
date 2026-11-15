package editor

import (
	"errors"
	"path"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/assetfiles"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/engine/problems"
	"aego/format/aetext"
	"aego/modules/render2d"
	"aego/render/material"
	"aego/render/shader"
	"aego/scene/mutate"
	"aego/ui/widgets"
)

const newShaderSource = `#pragma aego abi 1
#pragma domain sprite
#include "engine/sprite.glsl"

#pragma material
uniform float Opacity; // @instance @default(1) @range(0, 1)
#pragma end

void fragment() {
    aego_out = aego_sample() * Opacity;
}
`

type shaderDraft struct {
	revision uint64
	shader   *material.Shader
	err      error
}
type materialView struct {
	doc      *document.ScriptDocument
	gesture  string
	revision uint64
	preview  materialPreview
	seen     bool
	scroll   float32
	content  float32
	bar      widgets.ScrollState
}
type materialsEditor struct {
	ctx                *context.Editor
	views              map[ids.AssetID]*materialView
	shaders            map[ids.AssetID]shaderDraft
	currentShader      ids.AssetID
	diagnosticRevision uint64
	diagnostics        []problems.Diagnostic
}

func materialHooks(r *ext.Registry) error {
	ed := r.Editor()
	if ed == nil {
		return nil
	}
	if err := materialDocumentHandlers(r); err != nil {
		return err
	}
	s := &materialsEditor{ctx: ed, views: map[ids.AssetID]*materialView{}, shaders: map[ids.AssetID]shaderDraft{}}
	if err := s.registerThumbnails(r); err != nil {
		return err
	}
	if err := r.AssetInspector("material", s.drawMaterial); err != nil {
		return err
	}
	if err := r.AssetInspector("shader", s.drawShader); err != nil {
		return err
	}
	overrides := &overrideInspector{}
	r.Tick(overrides.tick)
	if err := r.Inspector("engine.MaterialOverride", overrides); err != nil {
		return err
	}
	r.Tick(func() {
		for _, v := range s.views {
			if !v.seen {
				v.preview.close(ed)
			}
			v.seen = false
		}
	})
	if ed.Problems != nil {
		ed.Problems.Add(problems.Source{Revision: func() uint64 { return s.diagnosticRevision }, Append: func(dst []problems.Diagnostic) []problems.Diagnostic { return append(dst, s.diagnostics...) }})
	}
	for _, action := range []context.CreateAction{
		{Key: "render2d.shader", Name: i18n.Tr("editor.assets.shader_file", "Shader (.aeshader)…"), Category: i18n.Tr("editor.material.rendering", "Rendering"), Icon: "file.script", Doc: i18n.Tr("editor.material.create_shader_help", "Create a portable Sprite shader and open it in the code editor."), Run: func(ids.NodeID) error { s.createShaderIn(ed.AssetCreateDirectory("assets/shaders")); return nil }, RunInFolder: func(dir string) error { s.createShaderIn(dir); return nil }},
		{Key: "render2d.shader-include", Name: i18n.Tr("editor.assets.shader_include", "Shader Include (.glsl)…"), Category: i18n.Tr("editor.material.rendering", "Rendering"), Icon: "file.script", Run: func(ids.NodeID) error { s.createShaderInclude(ed.AssetCreateDirectory("assets/shaders")); return nil }, RunInFolder: func(dir string) error { s.createShaderInclude(dir); return nil }},
		{Key: "render2d.material", Name: i18n.Tr("editor.material.material", "Material") + " (.aemat)…", Category: i18n.Tr("editor.material.rendering", "Rendering"), Icon: "comp.sprite", Doc: i18n.Tr("editor.material.create_material_help", "Create a material together with its shader."), Run: func(ids.NodeID) error {
			s.createMaterialIn(false, ed.AssetCreateDirectory("assets/materials"))
			return nil
		}, RunInFolder: func(dir string) error { s.createMaterialIn(false, dir); return nil }},
		{Key: "render2d.material-from-shader", Name: i18n.Tr("editor.material.from_shader", "Material from Shader"), Category: i18n.Tr("editor.material.rendering", "Rendering"), Icon: "comp.sprite", Doc: i18n.Tr("editor.material.from_shader_help", "Create a material using the most recently inspected shader or a shader AssetID."), Run: func(ids.NodeID) error { s.createMaterial(true); return nil }},
	} {
		if err := r.CreateAction(action); err != nil {
			return err
		}
	}
	return nil
}

func (s *materialsEditor) report(err error) {
	if err != nil {
		s.ctx.Notify(context.LevelError, err.Error())
	}
}

func (s *materialsEditor) stamp() uint64 {
	h := uint64(14695981039346656037) ^ s.ctx.AssetRev
	for _, d := range s.ctx.Docs.Scripts() {
		if d.Dirty() && d.Code.GLSL {
			h = (h ^ uint64(d.ID)) * 1099511628211
			h = (h ^ d.Code.Rev()) * 1099511628211
		}
	}
	return h
}

// schema uses the actual importer and dirty include buffers. It is separate
// from the renderer's committed last-good generation and never replaces it.
func (s *materialsEditor) schema(id ids.AssetID) (*material.Shader, bool) {
	if asset.IsBuiltin(id) && s.ctx.Loader != nil {
		return s.ctx.Loader.Shader(id)
	}
	if s.ctx.Pipeline == nil || s.ctx.Docs == nil {
		return nil, false
	}
	rev := s.stamp()
	if old, ok := s.shaders[id]; ok && old.revision == rev {
		return old.shader, old.err == nil && old.shader != nil
	}
	rel, ok := s.ctx.Pipeline.PathOf(id)
	if !ok {
		return nil, false
	}
	d, err := s.ctx.Docs.OpenScript(rel)
	var sh *material.Shader
	if err == nil {
		result, importErr := s.ctx.Pipeline.Preview(rel, d.Code.Bytes(), func(name string) ([]byte, bool) {
			if doc := s.ctx.Docs.Script(name); doc != nil {
				return doc.Code.Bytes(), true
			}
			return nil, false
		})
		err = importErr
		if err == nil && result != nil {
			err = result.Error
			for _, a := range result.Artifacts {
				if a.Kind != asset.ArtShader {
					continue
				}
				artifact, e := asset.DecodeShader(a.Data)
				if e != nil {
					err = e
					break
				}
				sh = artifact.Runtime(id, rel)
			}
		}
		if err == nil && sh == nil {
			err = errors.New(i18n.Tr("editor.material.importer_no_artifact", "shader importer produced no AESH artifact"))
		}
	}
	s.shaders[id] = shaderDraft{revision: rev, shader: sh, err: err}
	s.updateDiagnostics(id, rel, err)
	return sh, err == nil && sh != nil
}

func (s *materialsEditor) updateDiagnostics(id ids.AssetID, rel string, err error) {
	out := s.diagnostics[:0]
	for _, d := range s.diagnostics {
		if d.Subject.Asset != id {
			out = append(out, d)
		}
	}
	s.diagnostics = out
	if err != nil {
		d := problems.Diagnostic{Owner: "render2d", Code: "SHD001", Severity: problems.Error, Message: err.Error(), Subject: problems.Subject{Kind: problems.SubjectFile, Asset: id, File: rel}}
		// Importer diagnostics carry exact source/include locations as structured
		// fields. Keep unknown columns at zero rather than inventing offsets.
		for cause := err; cause != nil; cause = errors.Unwrap(cause) {
			if e, ok := cause.(*errs.Error); ok {
				for _, f := range e.Fields {
					switch f.Key {
					case "code":
						if code, ok := f.Val.(string); ok {
							d.Code = code
						}
					case "file":
						if file, ok := f.Val.(string); ok && file != "" {
							d.Subject.File = file
						}
					case "line":
						if line, ok := f.Val.(int); ok && line > 0 {
							d.Subject.Line = line
						}
					}
				}
			}
		}
		s.diagnostics = append(s.diagnostics, d)
	}
	s.diagnosticRevision++
}

func (s *materialsEditor) open(id ids.AssetID) (*materialView, error) {
	if s.ctx.Pipeline == nil || s.ctx.Docs == nil {
		return nil, errors.New(i18n.Tr("editor.material.editing_unavailable", "material editing requires an asset pipeline"))
	}
	rel, ok := s.ctx.Pipeline.PathOf(id)
	if !ok {
		return nil, errors.New(i18n.Tr("editor.material.unresolved", "material {asset} is unresolved", i18n.Str("asset", id.Short())))
	}
	d, err := s.ctx.Docs.OpenScript(rel)
	if err != nil {
		return nil, err
	}
	v := s.views[id]
	if v == nil {
		v = &materialView{}
		s.views[id] = v
	}
	v.doc = d
	v.seen = true
	return v, nil
}

func (s *materialsEditor) mutate(v *materialView, key string, edit widgets.Edit, ops ...mutate.Op) {
	if !edit.Changed {
		if edit.End {
			v.gesture = ""
		}
		return
	}
	continuing := v.gesture == key && !edit.Begin && v.revision == v.doc.Code.Rev()
	_, err := document.ApplyMaterialMutation(v.doc, s.schema, mutate.Tx{Label: i18n.Tr("editor.material.edit_parameter", "Material {parameter}", i18n.Str("parameter", key)), Ops: ops}, continuing)
	s.report(err)
	v.revision = v.doc.Code.Rev()
	if edit.End {
		v.gesture = ""
	} else {
		v.gesture = key
	}
}

func oneEdit() widgets.Edit { return widgets.Edit{Changed: true, Begin: true, End: true} }

func (s *materialsEditor) save(v *materialView) {
	if s.ctx.Docs.InPlay() {
		s.report(errors.New(i18n.Tr("editor.material.stop_play_before_save", "save material assets after stopping Play")))
		return
	}
	if err := s.ctx.Docs.SaveScript(v.doc); err != nil {
		s.report(err)
		return
	}
}

func (s *materialsEditor) drawMaterial(ed *context.Editor, id ids.AssetID) {
	if asset.IsBuiltin(id) {
		s.drawBuiltin(ed, id)
		return
	}
	ui := ed.UI
	v, err := s.open(id)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	ui.PushID("material:" + id.String())
	defer ui.PopID()
	defer s.beginScroll(v)()
	widgets.Label(ui, path.Base(v.doc.Path))
	widgets.LabelDim(ui, v.doc.Path)
	widgets.LabelWrapped(ui, i18n.Tr("editor.material.current_source", "Inspector: current material and shader source."), false)
	if widgets.Button(ui, i18n.Tr("editor.material.open_source", "Open material source")) {
		ed.Open(context.At(v.doc.Path, 1, 0))
	}
	if widgets.Button(ui, i18n.Tr("editor.material.save", "Save material")) {
		s.save(v)
	}
	if widgets.Button(ui, i18n.Tr("editor.material.undo", "Undo material edit")) {
		v.doc.Code.Undo()
		v.gesture = ""
	}
	if widgets.Button(ui, i18n.Tr("editor.material.redo", "Redo material edit")) {
		v.doc.Code.Redo()
		v.gesture = ""
	}
	if v.doc.Dirty() {
		widgets.LabelWrapped(ui, i18n.Tr("editor.material.unsaved", "Unsaved material edits. Save material to apply them to the scene."), false)
	}
	s.drawRuntimeState(id)
	sourceRevision := v.doc.Code.Rev()
	parsed, err := asset.ParseMaterial(v.doc.Code.Bytes(), v.doc.Path)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		v.preview.drawLast(ed)
		return
	}
	shaderID := parsed.Shader
	if assetPicker(ed, ui.ID("shader"), i18n.Tr("editor.material.shader", "Shader"), "shader", &shaderID) {
		s.mutate(v, "shader", oneEdit(), mutate.MaterialShader(shaderID.String()))
		return
	}
	sh, ok := s.schema(parsed.Shader)
	if !ok {
		widgets.LabelWrapped(ui, i18n.Tr("editor.material.invalid_source", "Current shader source is invalid or unresolved. The running scene keeps its last-good generation."), true)
		if state, found := s.shaders[parsed.Shader]; found && state.err != nil {
			widgets.LabelWrapped(ui, state.err.Error(), true)
		}
		v.preview.drawLast(ed)
		return
	}
	widgets.LabelDim(ui, i18n.Tr("editor.material.domain", "Domain: {domain}", i18n.Str("domain", sh.Domain.String())))
	if s.shaderDirty(sh) {
		widgets.LabelWrapped(ui, i18n.Tr("editor.material.unsaved_shader", "Shader or include edits are unsaved. The Inspector uses that current source; save those files to apply it to the scene."), false)
	}
	if rel, found := ed.Pipeline.PathOf(parsed.Shader); found && widgets.Button(ui, i18n.Tr("editor.material.edit_shader", "Edit shader")) {
		ed.Open(context.At(rel, 1, 0))
	}
	names := make([]string, len(sh.Variants))
	selected := 0
	var mask uint32
	for _, name := range parsed.Variants {
		for i, key := range sh.Keywords {
			if name == key {
				mask |= 1 << i
			}
		}
	}
	for i, variant := range sh.Variants {
		var keys []string
		for k, name := range sh.Keywords {
			if variant.Mask&(1<<k) != 0 {
				keys = append(keys, name)
			}
		}
		names[i] = strings.Join(keys, " + ")
		if names[i] == "" {
			names[i] = i18n.Tr("editor.material.default", "Default")
		}
		if variant.Mask == mask {
			selected = i
		}
	}
	if widgets.Combo(ui, ui.ID("variant"), widgets.Field(ui, i18n.Tr("editor.material.variant", "Variant")), &selected, names, false) {
		var keys []string
		for i, name := range sh.Keywords {
			if sh.Variants[selected].Mask&(1<<i) != 0 {
				keys = append(keys, name)
			}
		}
		s.mutate(v, "variant", oneEdit(), mutate.MaterialVariants(keys))
	}
	blends := []string{i18n.Tr("editor.material.inherit_shader", "Inherit shader"), i18n.Tr("editor.material.premultiplied", "Premultiplied"), i18n.Tr("editor.material.opaque", "Opaque"), i18n.Tr("editor.material.additive", "Additive")}
	blendKeys := []string{"", "premultiplied", "opaque", "additive"}
	blend := 0
	if parsed.HasBlend {
		blend = int(parsed.Blend) + 1
	}
	if widgets.Combo(ui, ui.ID("blend"), widgets.Field(ui, i18n.Tr("editor.material.blend", "Blend")), &blend, blends, false) {
		s.mutate(v, "blend", oneEdit(), mutate.MaterialBlend(blendKeys[blend]))
	}
	m, issues := parsed.Build(i18n.Tr("editor.material.preview_name", "Material preview"), sh, func(id ids.AssetID) (material.Tex, bool) {
		if ed.Loader == nil {
			return material.Tex{}, false
		}
		return ed.Loader.MaterialParameterTexture(id)
	})
	for _, issue := range issues {
		widgets.LabelWrapped(ui, issue.Code+": "+issue.Msg, true)
	}
	if m == nil {
		widgets.LabelWrapped(ui, i18n.Tr("editor.material.incompatible_help", "Fix incompatible values in source, or reset them to the current shader defaults."), true)
		if widgets.Button(ui, i18n.Tr("editor.material.reset_incompatible", "Reset incompatible values")) {
			var ops []mutate.Op
			for _, param := range parsed.Values {
				if p, known := sh.Schema.Param(param.Name); known && (p.Kind == shader.KindTexture) != param.Asset {
					ops = append(ops, mutate.MaterialReset(param.Name))
				}
			}
			// The explicit reset applies all fields as one atomic, undoable operation.
			if len(ops) == 0 {
				for _, param := range parsed.Values {
					if _, known := sh.Schema.Param(param.Name); known {
						ops = append(ops, mutate.MaterialReset(param.Name))
					}
				}
			}
			s.mutate(v, "reset incompatible", oneEdit(), ops...)
		}
		v.preview.drawLast(ed)
		return
	}
	for i := range sh.Schema.Params {
		p := &sh.Schema.Params[i]
		if p.Hidden {
			continue
		}
		ui.PushID(p.Name)
		explicit := declaredExplicit(parsed, p)
		if p.Kind == shader.KindTexture {
			texture := parsed.Textures[p.Name]
			if texture.IsZero() {
				for _, former := range p.Former {
					if id, ok := parsed.Textures[former]; ok {
						texture = id
						break
					}
				}
			}
			if assetPicker(ed, ui.ID("texture"), p.Name, "texture", &texture) {
				s.mutate(v, p.Name, oneEdit(), mutate.MaterialSetValue(p.Name, typedAsset(texture)))
			}
		} else {
			values, _ := m.Get(p.Name)
			integer, _ := m.GetInt(p.Name)
			value := render2d.ParameterValue(p, values, integer)
			change := parameterControl(ed, p, &value)
			s.mutate(v, p.Name, change, mutate.MaterialSetValue(p.Name, typedValue(value)))
		}
		if explicit {
			if widgets.Button(ui, i18n.Tr("editor.material.reset_default", "Reset to shader default")) {
				s.mutate(v, p.Name, oneEdit(), mutate.MaterialReset(p.Name))
			}
		} else {
			widgets.LabelWrapped(ui, i18n.Tr("editor.material.inherited_default", "Inherited shader default"), false)
		}
		ui.PopID()
	}
	for _, value := range parsed.Values {
		if _, known := sh.Schema.Param(value.Name); !known {
			widgets.LabelWrapped(ui, i18n.Tr("editor.material.unknown", "Unknown parameter preserved: {parameter}", i18n.Str("parameter", value.Name)), false)
		}
	}
	v.preview.update(ed, m, s.stamp(), sourceRevision)
	v.preview.draw(ed)
}

// A saved, schema-valid draft may still fail GPU or live-node migration checks; the Inspector must distinguish it from the committed scene generation.
func (s *materialsEditor) drawRuntimeState(id ids.AssetID) {
	ed := s.ctx
	if ed.Loader == nil {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.runtime_no_loader", "Scene: no runtime material loader is available."), false)
		return
	}
	live, ok := ed.Loader.Material(id)
	if !ok {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.runtime_unavailable", "Scene: runtime material unavailable; the Inspector edits current source."), false)
		return
	}
	if ed.Renderer == nil || ed.Renderer.Caps().Renderer == "null" {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.runtime_cpu", "Scene: CPU material generation {generation}; GPU validation is unavailable.", i18n.Int("generation", int64(live.Generation()))), false)
		return
	}
	if ed.Renderer.Materials().Pending(live) {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.runtime_pending", "Scene: compiling a candidate; committed generation {generation} remains active until the frame boundary.", i18n.Int("generation", int64(live.Generation()))), false)
	} else if live.Failed() {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.runtime_failed", "Scene: reload failed; last-good generation {generation} remains active. See Problems for the reason.", i18n.Int("generation", int64(live.Generation()))), true)
	} else {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.material.runtime_committed", "Scene: committed material generation {generation}. Driver validation occurs when rendered.", i18n.Int("generation", int64(live.Generation()))), false)
	}
}

func (s *materialsEditor) shaderDirty(sh *material.Shader) bool {
	if sh == nil || s.ctx.Docs == nil {
		return false
	}
	if s.ctx.Pipeline != nil {
		if rel, ok := s.ctx.Pipeline.PathOf(sh.ID); ok {
			if doc := s.ctx.Docs.Script(rel); doc != nil && doc.Dirty() {
				return true
			}
		}
	}
	for _, name := range sh.Files {
		if doc := s.ctx.Docs.Script(name); doc != nil && doc.Dirty() {
			return true
		}
	}
	return false
}

func (s *materialsEditor) drawShader(ed *context.Editor, id ids.AssetID) {
	s.currentShader = id
	if asset.IsBuiltin(id) {
		s.drawBuiltin(ed, id)
		return
	}
	ui := ed.UI
	if ed.Pipeline == nil {
		return
	}
	rel, ok := ed.Pipeline.PathOf(id)
	if !ok {
		return
	}
	v, err := s.open(id)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	ui.PushID("shader:" + id.String())
	defer ui.PopID()
	defer s.beginScroll(v)()
	widgets.Label(ui, path.Base(rel))
	if widgets.Button(ui, i18n.Tr("editor.material.open_shader", "Open shader code")) {
		ed.Open(context.At(rel, 1, 0))
	}
	if widgets.Button(ui, i18n.Tr("editor.material.create_from_this_shader", "Create material from this shader")) {
		s.createMaterial(true)
	}
	d := v.doc
	if d.Dirty() {
		widgets.LabelWrapped(ui, i18n.Tr("editor.material.shader_unsaved", "Unsaved source; running scenes keep the committed shader until Save."), false)
	}
	if widgets.Button(ui, i18n.Tr("editor.material.save_shader", "Save and compile shader")) {
		err = ed.Docs.SaveScript(d)
		s.report(err)
	}
	sh, ok := s.schema(id)
	if !ok {
		if draft, found := s.shaders[id]; found && draft.err != nil {
			widgets.LabelWrapped(ui, draft.err.Error(), true)
		}
		return
	}
	widgets.LabelDim(ui, i18n.Tr("editor.material.domain", "Domain: {domain}", i18n.Str("domain", sh.Domain.String())))
	widgets.LabelWrapped(ui, i18n.Tr("editor.material.shader_counts", "{parameters} parameters, {variants} permitted variants", i18n.Int("parameters", int64(len(sh.Schema.Params))), i18n.Int("variants", int64(len(sh.Variants)))), false)
	widgets.LabelWrapped(ui, i18n.Tr("editor.material.schema_checked", "Source/schema checked. Driver validation occurs on the active GPU."), false)
	for _, p := range sh.Schema.Params {
		label := p.Name + " : " + p.Kind.GLSL()
		if p.Instance {
			label += " · " + i18n.Tr("editor.material.instance_override", "instance override")
		}
		widgets.LabelDim(ui, label)
	}
}

// Asset inspectors can contain many fields or a long compiler log. Keep all
// content reachable with the normal editor wheel and scrollbar controls.
func (s *materialsEditor) beginScroll(v *materialView) func() {
	ui := s.ctx.UI
	clip, ok := ui.ClipRect()
	if !ok {
		return func() {}
	}
	cur := ui.Cursor()
	top, width := cur.Y, cur.W
	r := mathx.Rect{X: cur.X, Y: top, W: width, H: max(0, clip.Y+clip.H-top)}
	if ui.MouseOver(ui.ID("material-scroll"), r) {
		v.scroll -= ui.ScrollY(ui.Theme.RowHeight * 3)
	}
	v.scroll = mathx.Clamp(v.scroll, 0, max(0, v.content-r.H))
	cur.Y -= v.scroll
	if v.content > r.H {
		cur.W -= 8
	}
	return func() {
		v.content = cur.Y + v.scroll - top
		cur.W = width
		widgets.ScrollBar(ui, ui.ID("material-scrollbar"), mathx.Rect{X: r.X + r.W - 6, Y: r.Y, W: 6, H: r.H}, &v.scroll, v.content, r.H, &v.bar)
	}
}

func (s *materialsEditor) createShaderInclude(dir string) {
	ed := s.ctx
	if ed.Pipeline == nil {
		return
	}
	files := assetfiles.New(ed)
	rel := files.UniqueFile(dir, "include", ".glsl")
	ed.Ask(context.Question{Title: i18n.Tr("editor.assets.shader_include", "Shader Include (.glsl)…"), Fields: []string{i18n.Tr("editor.material.project_path", "Project path")}, Values: []string{rel}, Buttons: []string{i18n.Tr("editor.material.create", "Create"), i18n.Tr("editor.material.cancel", "Cancel")}, Submit: func(values []string) {
		if len(values) == 0 {
			return
		}
		name := strings.TrimSpace(values[0])
		if !strings.EqualFold(path.Ext(name), ".glsl") {
			s.report(errors.New(i18n.Tr("editor.assets.glsl_extension", "include path must end with .glsl")))
			return
		}
		_, err := s.createSource(name, []byte("// Shared shader functions.\n"))
		s.report(err)
		if err == nil {
			ed.Open(context.At(name, 1, 0))
		}
	}})
}

func (s *materialsEditor) createShaderIn(dir string) {
	ed := s.ctx
	if ed.Pipeline == nil {
		return
	}
	files := assetfiles.New(ed)
	rel := files.UniqueFile(dir, "material", ".aeshader")
	ed.Ask(context.Question{Title: i18n.Tr("editor.material.create_shader", "Create Shader"), Fields: []string{i18n.Tr("editor.material.project_path", "Project path")}, Values: []string{rel}, Buttons: []string{i18n.Tr("editor.material.create", "Create"), i18n.Tr("editor.material.cancel", "Cancel")}, Submit: func(values []string) {
		if len(values) == 0 {
			return
		}
		name := strings.TrimSpace(values[0])
		if path.Ext(name) != ".aeshader" {
			s.report(errors.New(i18n.Tr("editor.material.shader_extension", "shader path must end with .aeshader")))
			return
		}
		id, err := s.createSource(name, []byte(newShaderSource))
		s.report(err)
		if err == nil {
			s.currentShader = id
			ed.Open(context.At(name, 1, 0))
		}
	}})
}

func (s *materialsEditor) createMaterial(existing bool) {
	s.createMaterialIn(existing, "assets/materials")
}

func (s *materialsEditor) createMaterialIn(existing bool, dir string) {
	ed := s.ctx
	if ed.Pipeline == nil {
		return
	}
	files := assetfiles.New(ed)
	rel := files.UniqueFile(dir, "material", ".aemat")
	fields := []string{i18n.Tr("editor.material.project_path", "Project path")}
	values := []string{rel}
	if existing {
		fields = append(fields, i18n.Tr("editor.material.shader_id", "Shader AssetID"))
		id := ""
		if !s.currentShader.IsZero() {
			id = s.currentShader.String()
		}
		values = append(values, id)
	}
	ed.Ask(context.Question{Title: i18n.Tr("editor.material.create_material", "Create Material"), Fields: fields, Values: values, Buttons: []string{i18n.Tr("editor.material.create", "Create"), i18n.Tr("editor.material.cancel", "Cancel")}, Submit: func(input []string) {
		if len(input) == 0 {
			return
		}
		name := strings.TrimSpace(input[0])
		if path.Ext(name) != ".aemat" {
			s.report(errors.New(i18n.Tr("editor.material.material_extension", "material path must end with .aemat")))
			return
		}
		var shaderID ids.AssetID
		if existing {
			if len(input) < 2 {
				return
			}
			id, err := ids.ParseAssetID(strings.TrimSpace(input[1]))
			if err != nil {
				s.report(err)
				return
			}
			shaderID = id
			if _, ok := s.schema(shaderID); !ok {
				s.report(errors.New(i18n.Tr("editor.material.choose_shader", "choose an existing, valid shader asset")))
				return
			}
		}
		var id ids.AssetID
		create := func() error {
			if !existing {
				shaderPath := files.UniqueFile("assets/shaders", strings.TrimSuffix(path.Base(name), ".aemat"), ".aeshader")
				var err error
				shaderID, err = s.createSource(shaderPath, []byte(newShaderSource))
				if err != nil {
					return err
				}
			}
			source := []byte("material version=1 shader=asset(\"" + shaderID.String() + "\") {\n  params {}\n}\n")
			var err error
			id, err = s.createSource(name, source)
			return err
		}
		var err error
		if ed.AssetTransaction != nil {
			err = ed.AssetTransaction(create)
		} else {
			err = errors.New(i18n.Tr("editor.material.creation_history", "asset creation requires an editor Assets history"))
		}

		s.report(err)
		if err == nil {
			s.currentShader = shaderID
			ed.Open(context.AssetAt(name))
			if ed.InspectAsset != nil {
				ed.InspectAsset(id)
			}
			_, _ = s.open(id)
		}
	}})
}

func typedAsset(id ids.AssetID) aetext.Value { return aetext.AssetRef(id.String()) }

func declaredExplicit(d *asset.MaterialDoc, p *shader.Param) bool {
	if _, ok := d.Value(p.Name); ok {
		return true
	}
	for _, former := range p.Former {
		if _, ok := d.Value(former); ok {
			return true
		}
	}
	return false
}

func (s *materialsEditor) createSource(rel string, source []byte) (ids.AssetID, error) {
	if s.ctx.CreateAsset == nil {
		return ids.AssetID{}, errors.New(i18n.Tr("editor.material.creation_history", "asset creation requires an editor Assets history"))
	}
	return s.ctx.CreateAsset(rel, source)
}
