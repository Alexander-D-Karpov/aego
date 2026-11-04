package editor

import (
	"fmt"
	"slices"
	"strings"
	"unsafe"

	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/ext"
	"aego/engine"
	"aego/engine/i18n"
	"aego/modules/render2d"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/scene"
	"aego/schema"
	"aego/ui/widgets"
)

type postInspector struct {
	reg     *postfx.Registry
	td      *schema.TypeDescriptor
	field   *schema.FieldDescriptor
	choice  int
	search  string
	closed  map[string]bool
	tx      *history.Transaction
	doc     *document.SceneDocument
	node    ids.NodeID
	gesture string
	bypass  string
	seen    bool
}

func postHooks(r *ext.Registry) error {
	if r.Editor() == nil || r.Schema() == nil {
		return nil
	}
	td, ok := r.Schema().ByName(render2d.NamePostProcess)
	if !ok {
		return nil
	}
	field, _ := td.Field("effects")
	ed := r.Editor()
	ed.CameraPreview = func() *scene.Scene {
		sc, nodes := ed.Scene(), ed.Selected()
		if sc != nil && len(nodes) == 1 {
			h := sc.Handle(nodes[0])
			if scene.Has[render2d.PostProcess](sc, h) && scene.Has[render2d.Camera2D](sc, h) {
				return sc
			}
		}
		return nil
	}
	w := &postInspector{td: td, field: field, closed: make(map[string]bool)}
	ed.EffectPreview = func() (ids.NodeID, string) {
		if w.bypass != "" && w.doc == ed.Doc() {
			if nodes := ed.Selected(); len(nodes) == 1 && nodes[0] == w.node {
				return w.node, w.bypass
			}
		}
		return ids.NodeID{}, ""
	}
	// Test hosts without an engine still expose the built-in descriptor palette.
	if eng := r.Editor().Engine; eng != nil {
		w.reg, _ = engine.Service[*postfx.Registry](eng, render2d.PostEffectsService)
	}
	if w.reg == nil {
		w.reg = postfx.NewRegistry()
	}
	r.Tick(func() {
		if !w.seen {
			w.finish()
		}
		w.seen = false
	})
	if err := w.registerPresetPreview(r, ed); err != nil {
		return err
	}
	return r.Inspector(render2d.NamePostProcess, w)
}

func (*postInspector) FieldVisible(_ *context.Editor, _ []ids.NodeID, f *schema.FieldDescriptor) bool {
	return f.Name != "effects"
}

func (w *postInspector) finish() {
	if w.tx != nil {
		w.tx.Commit()
		w.tx = nil
	}
	w.gesture = ""
}
func (w *postInspector) Draw(ed *context.Editor, nodes []ids.NodeID, fields func()) {
	fields()
	w.seen = true
	if len(nodes) != 1 || ed.Doc() == nil {
		w.finish()
		return
	}
	d := ed.Doc()
	if w.doc != d || w.node != nodes[0] {
		w.finish()
		w.doc, w.node = d, nodes[0]
		clear(w.closed)
		w.bypass = ""
	}
	c, ok := scene.Get[render2d.PostProcess](d.Scene(), d.Scene().Handle(w.node))
	if !ok {
		return
	}
	reg := w.reg
	ui := ed.UI
	factoryNames := []string{i18n.Tr("editor.postfx.choose_preset", "Apply preset…"), i18n.Tr("editor.postfx.original", "Original"), i18n.Tr("editor.postfx.subtle_bloom", "Subtle Bloom"), i18n.Tr("editor.postfx.color_grade", "Color Grade"), i18n.Tr("editor.postfx.pixel_crt", "Pixel/CRT"), i18n.Tr("editor.postfx.custom_preset", "Custom")}
	selected := 0
	if widgets.Combo(ui, ui.ID("post-preset"), widgets.Field(ui, i18n.Tr("editor.postfx.presets", "Stack preset")), &selected, factoryNames, false) && selected > 0 {
		if err := w.applyFactoryPreset(c, selected-1, factoryNames[selected]); err != nil {
			ed.Notify(context.LevelError, err.Error())
		}
		return
	}
	desc := reg.Descriptors()
	widgets.TextFieldRect(ui, ui.ID("effect-filter"), widgets.Field(ui, i18n.Tr("editor.postfx.search", "Find effect")), &w.search, true)
	var labels []string
	var palette []*postfx.Descriptor
	for _, v := range desc {
		if w.search != "" && !strings.Contains(strings.ToLower(v.Label), strings.ToLower(w.search)) {
			continue
		}
		labels = append(labels, v.Label)
		palette = append(palette, v)
	}
	if len(labels) > 0 {
		w.choice = min(w.choice, len(labels)-1)
		widgets.Combo(ui, ui.ID("effect-type"), widgets.Field(ui, i18n.Tr("editor.postfx.type", "Effect")), &w.choice, labels, false)
		if widgets.Button(ui, i18n.Tr("editor.postfx.add", "Add effect")) {
			w.action(ed, c, func(n *render2d.PostProcess) error {
				src, _ := d.Scene().Resolver().(postfx.MaterialSource)
				return addEffectWithMaterials(reg, n, palette[w.choice], ids.NewNodeID().String(), src)
			})
			return
		}
	}
	src, _ := d.Scene().Resolver().(postfx.MaterialSource)
	// A material reimport can change its phase after the authored stack was saved.
	// Let the author repair that order and keep every instance/animation ID.
	phaseOf := func(e *postfx.Instance) (postfx.Phase, bool) {
		desc, ok := reg.Lookup(e.TypeID)
		if !ok {
			return 0, false
		}
		return desc.PhaseFor(e, src), true
	}
	if phaseOrderBroken(c.Effects, phaseOf) && widgets.Button(ui, i18n.Tr("editor.postfx.restore_phase_order", "Restore phase order")) {
		w.action(ed, c, func(n *render2d.PostProcess) error { restorePhaseOrder(n.Effects, phaseOf); return nil })
		return
	}
	var prevTitle string
	for i := range c.Effects {
		e := &c.Effects[i]
		phaseTitle := i18n.Tr("editor.postfx.unavailable_group", "Unavailable effects")
		if desc, ok := reg.Lookup(e.TypeID); ok {
			if desc.PhaseFor(e, src) == postfx.SceneLinear {
				phaseTitle = i18n.Tr("editor.postfx.scene_group", "Scene linear · before exposure")
			} else {
				phaseTitle = i18n.Tr("editor.postfx.display_group", "Display linear · after tone mapping")
			}
		}
		if prevTitle != phaseTitle {
			widgets.Label(ui, phaseTitle)
			prevTitle = phaseTitle
		}
		ui.PushID(e.InstanceID)
		change := w.drawEffect(ed, c, i)
		ui.PopID()
		if change {
			return
		}
	}
}

func gamePostStatus(ed *context.Editor, d *document.SceneDocument, node ids.NodeID, instance string) string {
	if ed.GamePostOutput != nil {
		world, output, preview := ed.GamePostOutput(d)
		if state, ok := render2d.ObservedPostEffect(world, output, node, instance, preview); ok {
			switch state.State {
			case postfx.Active:
				return i18n.Tr("editor.postfx.status_active", "Game View plan: active")
			case postfx.Neutral:
				return i18n.Tr("editor.postfx.status_neutral", "Game View: neutral parameters")
			case postfx.Unknown:
				return i18n.Tr("editor.postfx.status_unknown", "Game View: unavailable effect type (disabled)")
			}
			switch state.Reason {
			case "component disabled":
				return i18n.Tr("editor.postfx.status_component_off", "Game View: component disabled")
			case "disabled":
				return i18n.Tr("editor.postfx.status_effect_off", "Game View: effect disabled")
			case "preview bypass":
				return i18n.Tr("editor.postfx.status_preview_off", "Game View: bypassed for preview")
			case "conflicting output owners":
				return i18n.Tr("editor.postfx.status_conflict", "Game View: multiple camera stacks own this output")
			case "reduced motion":
				return i18n.Tr("editor.postfx.status_motion", "Game View: disabled by Reduced Motion")
			case "post-processing setting disabled":
				return i18n.Tr("editor.postfx.status_setting", "Game View: post-processing disabled in player settings")
			case "effect requires an opaque output":
				return i18n.Tr("editor.postfx.status_alpha", "Game View: this effect requires an opaque output")
			case "display effect requires a display output":
				return i18n.Tr("editor.postfx.status_linear", "Game View: display effect bypassed on a linear output")
			default:
				return i18n.Tr("editor.postfx.status_invalid", "Game View: stack bypassed · {reason}", i18n.Str("reason", state.Reason))
			}
		}
	}
	return i18n.Tr("editor.postfx.status_unobserved", "No recent Game View frame for this camera")
}

func (w *postInspector) drawEffect(ed *context.Editor, c *render2d.PostProcess, idx int) bool {
	ui := ed.UI
	e := &c.Effects[idx]
	desc, known := w.reg.Lookup(e.TypeID)
	title := e.TypeID
	if known {
		title = desc.Label
	}
	open := !w.closed[e.InstanceID]
	if widgets.TreeNode(ui, ui.ID("effect"), title, &open, false) {
		open = !open
	}
	w.closed[e.InstanceID] = !open
	if !open {
		return false
	}
	on := e.Enabled
	if widgets.Checkbox(ui, i18n.Tr("editor.material.enabled", "Enabled"), &on) {
		w.action(ed, c, func(n *render2d.PostProcess) error { n.Effects[idx].Enabled = on; return nil })
		return true
	}
	bypassed := w.bypass == e.InstanceID
	if widgets.Checkbox(ui, i18n.Tr("editor.postfx.bypass_instance", "Preview without this effect"), &bypassed) {
		w.bypass = ""
		if bypassed {
			w.bypass = e.InstanceID
		}
		return true
	}
	widgets.LabelWrapped(ui, gamePostStatus(ed, w.doc, w.node, e.InstanceID), true)
	widgets.LabelDim(ui, e.InstanceID)
	src, _ := w.doc.Scene().Resolver().(postfx.MaterialSource)
	if !known {
		widgets.LabelWrapped(ui, i18n.Tr("editor.postfx.missing", "Effect type unavailable; stored values are preserved."), true)
	} else {
		if desc.LUT {
			id := e.LUT.ID
			if assetPicker(ed, ui.ID("lut"), "LUT", "texture", &id) {
				w.action(ed, c, func(n *render2d.PostProcess) error { n.Effects[idx].LUT.ID = id; return nil })
				return true
			}
		}
		if desc.UsesMaterial {
			id := e.Material.ID
			if assetPicker(ed, ui.ID("post-material"), i18n.Tr("editor.postfx.material", "PostFX material"), "material", &id) {
				w.action(ed, c, func(n *render2d.PostProcess) error {
					prev := desc.PhaseFor(&n.Effects[idx], src)
					n.Effects[idx].Material.ID = id
					if phase := desc.PhaseFor(&n.Effects[idx], src); phase != prev {
						v := n.Effects[idx]
						n.Effects = slices.Delete(n.Effects, idx, idx+1)
						insertEffect(w.reg, n, v, phase, src)
					}
					return nil
				})
				return true
			}
		}
		for i := range desc.Fields {
			f := &desc.Fields[i]
			v := render2d.EffectValue(e, f)
			edit := parameterControl(ed, &shader.Param{Name: f.Name, Kind: f.Kind, HasRange: f.Bounded, Min: f.Min, Max: f.Max}, &v)
			key := e.InstanceID + ":" + f.Name
			if edit.Begin || edit.Changed && w.gesture != key {
				w.finish()
				w.tx = w.doc.History.Begin(i18n.Tr("editor.postfx.edit", "Edit post effect"))
				w.gesture = key
			}
			if edit.Changed {
				draft := c.Clone()
				err := render2d.SetEffectValue(&draft.Effects[idx], f, v)
				if err == nil {
					err = w.write(&draft, w.tx)
				}
				if err != nil {
					ed.Notify(context.LevelError, err.Error())
					w.finish()
				}
			}
			if edit.End {
				w.finish()
			}
		}
	}
	if known && widgets.Button(ui, i18n.Tr("editor.postfx.reset", "Reset parameters")) {
		w.action(ed, c, func(n *render2d.PostProcess) error {
			v, err := w.reg.NewInstance(e.TypeID, e.InstanceID)
			if err == nil {
				v.Enabled = e.Enabled
				n.Effects[idx] = v
			}
			return err
		})
		return true
	}
	if widgets.Button(ui, i18n.Tr("editor.postfx.duplicate", "Duplicate effect")) {
		w.action(ed, c, func(n *render2d.PostProcess) error {
			v := n.Effects[idx]
			v.InstanceID = ids.NewNodeID().String()
			v.Parameters = slices.Clone(v.Parameters)
			n.Effects = slices.Insert(n.Effects, idx+1, v)
			return nil
		})
		return true
	}
	for _, step := range []int{-1, 1} {
		to := idx + step
		if to < 0 || to >= len(c.Effects) {
			continue
		}
		other, ok := w.reg.Lookup(c.Effects[to].TypeID)
		if !known || !ok {
			continue
		}
		if other.PhaseFor(&c.Effects[to], src) != desc.PhaseFor(e, src) {
			widgets.LabelDim(ui, i18n.Tr("editor.postfx.phase_boundary", "Effects can move within their colour phase."))

			continue
		}
		label := i18n.Tr("editor.postfx.up", "Move up")
		if step > 0 {
			label = i18n.Tr("editor.postfx.down", "Move down")
		}
		if widgets.Button(ui, label) {
			w.action(ed, c, func(n *render2d.PostProcess) error {
				n.Effects[idx], n.Effects[to] = n.Effects[to], n.Effects[idx]
				return nil
			})
			return true
		}
	}
	if widgets.Button(ui, i18n.Tr("editor.postfx.remove", "Remove effect")) {
		w.action(ed, c, func(n *render2d.PostProcess) error { n.Effects = slices.Delete(n.Effects, idx, idx+1); return nil })
		return true
	}
	return false
}

func addEffect(reg *postfx.Registry, c *render2d.PostProcess, desc *postfx.Descriptor, id string) error {
	return addEffectWithMaterials(reg, c, desc, id, nil)
}

func addEffectWithMaterials(reg *postfx.Registry, c *render2d.PostProcess, desc *postfx.Descriptor, id string, src postfx.MaterialSource) error {
	e, err := reg.NewInstance(desc.ID, id)
	if err != nil {
		return err
	}
	if _, ok := c.Effect(id); ok {
		return fmt.Errorf("duplicate effect ID %q", id)
	}
	insertEffect(reg, c, e, desc.PhaseFor(&e, src), src)
	return nil
}

func insertEffect(reg *postfx.Registry, c *render2d.PostProcess, e postfx.Instance, phase postfx.Phase, src postfx.MaterialSource) {
	at := len(c.Effects)
	for i, v := range c.Effects {
		if d, ok := reg.Lookup(v.TypeID); ok && d.PhaseFor(&v, src) > phase {
			at = i
			break
		}
	}
	c.Effects = slices.Insert(c.Effects, at, e)
}

func (w *postInspector) action(ed *context.Editor, c *render2d.PostProcess, edit func(*render2d.PostProcess) error) {
	w.finish()
	n := c.Clone()
	err := edit(&n)
	if err == nil {
		err = w.write(&n, nil)
	}
	if err != nil {
		ed.Notify(context.LevelError, err.Error())
	}
}

func (w *postInspector) write(c *render2d.PostProcess, tx *history.Transaction) error {
	var v schema.Value
	w.field.Get(unsafe.Pointer(c), &v)
	return w.doc.Props.Set(tx, []ids.NodeID{w.node}, w.td.ID, w.field.ID, v)
}

func phaseOrderBroken(items []postfx.Instance, phaseOf func(*postfx.Instance) (postfx.Phase, bool)) bool {
	var prev postfx.Phase
	for i := range items {
		phase, known := phaseOf(&items[i])
		if !known {
			continue
		}
		if phase < prev {
			return true
		}
		prev = phase
	}
	return false
}

// Unavailable effects keep their serialized slot. The two known phase groups
// each retain their previous relative order.
func restorePhaseOrder(items []postfx.Instance, phaseOf func(*postfx.Instance) (postfx.Phase, bool)) {
	var slots []int
	var groups [2][]postfx.Instance
	for i := range items {
		if p, ok := phaseOf(&items[i]); ok {
			slots = append(slots, i)
			groups[p] = append(groups[p], items[i])
		}
	}
	j := 0
	for _, group := range groups {
		for _, e := range group {
			items[slots[j]] = e
			j++
		}
	}
}
