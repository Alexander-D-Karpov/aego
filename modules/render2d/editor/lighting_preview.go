package editor

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/render"
	"aego/render/lighting"
	"aego/render/texture"
	"aego/render/view"
	"aego/scene"
	"aego/ui/widgets"
	"fmt"
	"slices"
)

const shadowPreviewSize = 192

type shadowResourcePreview struct {
	device   gpu.Device
	target   gpu.TextureHandle
	output   view.ID
	index    int
	light    ids.NodeID
	layers   []render.ShadowInfo
	ready    bool
	seen     bool
	reason   string
	labels   [lighting.MaxShadows]string
	choices  [lighting.MaxShadows]shadowChoice
	language string
	large    bool
}

type shadowChoice struct {
	id    ids.NodeID
	name  string
	layer int
}

func (p *shadowResourcePreview) syncChoices(s *scene.Scene) {
	lang := i18n.Default().Language()
	p.index = 0
	for i, info := range p.layers {
		if info.Light == p.light {
			p.index = i
		}
		choice := shadowChoice{id: info.Light, layer: info.Layer}
		if s != nil {
			if h := s.Handle(info.Light); s.IsAlive(h) {
				choice.name = s.Name(h)
			}
		}
		if p.choices[i] != choice || lang != p.language {
			name := choice.name
			if name == "" {
				name = info.Light.Short()
			}
			p.labels[i] = name
			p.choices[i] = choice
		}
	}
	// Show the layer only for names that collide. Keep ordinary names legible
	// in a narrow Inspector and restore short labels when a light disappears.
	for i := range p.layers {
		choice := p.choices[i]
		if choice.name == "" {
			continue
		}
		ambiguous := false
		for j := range p.layers {
			if i != j && p.choices[j].name == choice.name {
				ambiguous = true
				break
			}
		}
		if ambiguous {
			p.labels[i] = i18n.Tr("editor.lighting.shadow_owner", "{light}, array layer {layer}", i18n.Str("light", choice.name), i18n.Int("layer", int64(choice.layer)))
		} else {
			p.labels[i] = choice.name
		}
	}
	p.language = lang
}

func (p *shadowResourcePreview) draw(ed *context.Editor) {
	p.seen = true
	if ed.SceneLighting == nil || ed.Renderer == nil || ed.Renderer.Caps().Renderer == "null" {
		return // The Inspector already explains an unavailable graphics device.
	}
	r := ed.Renderer
	id := ed.SceneLighting.LightingOutput()
	p.layers = r.LightingShadowLayers(id, p.layers[:0])
	if len(p.layers) == 0 {
		p.ready = false
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.no_shadow_masks", "This Scene View has no active shadow masks. Enable lighting and a local shadowed light that reaches a Lit surface."), false)
		return
	}
	if p.output != id {
		p.output, p.ready, p.reason = id, false, ""
		p.index, p.light = 0, ids.NodeID{}
	}
	// The renderer can reorder layers as lights enter or leave the view.
	// Selection is a stable light ID. The widgets receive its current index.
	var activeScene *scene.Scene
	if ed.Docs != nil && ed.Doc() != nil {
		activeScene = ed.Doc().Scene()
	}
	p.syncChoices(activeScene)
	widgets.Combo(ed.UI, ed.UI.ID("lighting.shadow.layer"), widgets.Field(ed.UI, i18n.Tr("editor.lighting.shadow_layer", "Preview light")), &p.index, p.labels[:len(p.layers)], false)
	selected := p.layers[p.index]
	if p.light != selected.Light {
		p.light, p.ready = selected.Light, false
	}
	widgets.Tooltip(ed.UI, ed.UI.ID("lighting.shadow.layer"), i18n.Tr("editor.lighting.shadow_resource", "Shadow masks: {size} × {size}, {layers} active layers", i18n.Int("size", int64(selected.Resolution)), i18n.Int("layers", int64(len(p.layers)))))
	widgets.LabelDim(ed.UI, i18n.Tr("editor.lighting.shadow_summary_short", "{size} px · {count} masks · layer {layer}", i18n.Int("size", int64(selected.Resolution)), i18n.Int("count", int64(len(p.layers))), i18n.Int("layer", int64(selected.Layer))))
	if p.device != r.Device() {
		p.close()
		p.device = r.Device()
	}
	if p.target.IsZero() {
		h, err := p.device.CreateTexture(gpu.TextureDesc{Width: shadowPreviewSize, Height: shadowPreviewSize, Format: gpu.TexSRGBA8, Label: "editor.shadow-preview"})
		if err != nil {
			widgets.LabelWrapped(ed.UI, err.Error(), true)
			return
		}
		p.target = h
	}
	target, light := p.target, p.light
	ed.OnRenderAfterViews(func() {
		if p.target != target || p.output != id || p.light != light || ed.SceneLighting.LightingOutput() != id {
			return
		}
		// Layers can be reused between the UI and render phases. Resolve the
		// stable light ID again instead of previewing another light's old slot.
		var storage [lighting.MaxShadows]render.ShadowInfo
		layer := -1
		for _, info := range r.LightingShadowLayers(id, storage[:0]) {
			if info.Light == light {
				layer = info.Layer
				break
			}
		}
		if err := r.PreviewShadowLayer(id, layer, target, shadowPreviewSize, shadowPreviewSize); err != nil {
			p.ready, p.reason = false, err.Error()
		} else {
			p.ready, p.reason = true, ""
		}
	})
	if p.reason != "" {
		widgets.LabelWrapped(ed.UI, p.reason, true)
	} else if !p.ready {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.preview_pending", "Preparing the current shadow layer preview…"), false)
	} else {
		size := float32(112)
		if p.large {
			size = shadowPreviewSize
		}
		size = min(size, ed.UI.Cursor().W)
		area := ed.UI.Cursor().Next(size)
		area.W = size
		region := texture.Whole(target, shadowPreviewSize, shadowPreviewSize).FlippedY()
		ed.UI.List().Texture(region, area, mathx.Color{R: 1, G: 1, B: 1, A: 1})
		maskID := ed.UI.ID("lighting.shadow.legend")
		ed.UI.MouseOver(maskID, area)
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.shadow_key", "White: visible  ·  Black: occluded"), false)
		widgets.Tooltip(ed.UI, maskID, i18n.Tr("editor.lighting.shadow_legend", "White is visible; black is occluded. This is a light-space mask, before the surface normal and light colour are applied."))
		widgets.Checkbox(ed.UI, i18n.Tr("editor.lighting.large_shadow_preview", "Large mask preview"), &p.large)
	}
}

func (p *shadowResourcePreview) close() {
	if p.device != nil && !p.target.IsZero() {
		p.device.DestroyTexture(p.target)
	}
	p.target = gpu.TextureHandle{}
	p.device = nil
	p.ready, p.reason = false, ""
}

func (p *lightingEditor) profile(ed *context.Editor) {
	if ed.Renderer == nil {
		widgets.LabelDim(ed.UI, i18n.Tr("editor.lighting.no_gpu", "Lighting preview unavailable: this session has no graphics device."))
		return
	}
	s := ed.Renderer.Stats()
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.profile_cpu_prepare", "CPU preparation: select {select} ms, shadow geometry {geometry} ms, auxiliary submission {aux} ms", i18n.Float("select", float64(s.LightingSelectNS)/1e6, 3), i18n.Float("geometry", float64(s.ShadowGeometryNS)/1e6, 3), i18n.Float("aux", float64(s.LightingAuxNS)/1e6, 3)), false)
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.profile_cpu_draw", "CPU submission: scene {scene} ms, output resolve {resolve} ms. These are CPU timings, not GPU execution times.", i18n.Float("scene", float64(s.LightingSceneNS)/1e6, 3), i18n.Float("resolve", float64(s.LightingResolveNS)/1e6, 3)), false)
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.profile_sets", "All views this frame: {sets} light sets, {selected} selected entries, {breaks} lighting batch splits", i18n.Int("sets", int64(s.LightSets)), i18n.Int("selected", int64(s.SelectedLights)), i18n.Int("breaks", int64(s.LightingBreaks))), false)
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.profile_dropped", "Budget drops: {lights} light candidates, {shadows} shadows, {cookies} cookies", i18n.Int("lights", int64(s.DroppedLights)), i18n.Int("shadows", int64(s.DroppedShadowLights)), i18n.Int("cookies", int64(s.DroppedCookieLights))), s.DroppedLights+s.DroppedShadowLights+s.DroppedCookieLights > 0)
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.profile_cache", "Shadow masks: {dirty} redrawn, {hits} cache hits, {draws} draws; {copies} cookie copies", i18n.Int("dirty", int64(s.DirtyShadowMasks)), i18n.Int("hits", int64(s.ShadowCacheHits)), i18n.Int("draws", int64(s.ShadowDraws)), i18n.Int("copies", int64(s.CookieCopies))), false)
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.profile_uploads", "Uploads: {ubo} lighting UBO bytes, {shadow} shadow vertex bytes", i18n.Int("ubo", int64(s.LightingUBOBytes)), i18n.Int("shadow", int64(s.ShadowUploadBytes))), false)
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.profile_memory", "Pixel storage: {shadow} shadow bytes, {cookie} cookie bytes, {output} scene-target bytes; {resources} lighting resources", i18n.Int("shadow", int64(s.ShadowBytes)), i18n.Int("cookie", int64(s.CookieBytes)), i18n.Int("output", int64(s.LightingTargetBytes)), i18n.Int("resources", int64(s.LightingResources))), false)
	widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.profile_resolve", "{passes} output resolves; {failures} lighting failures", i18n.Int("passes", int64(s.ResolvePasses)), i18n.Int("failures", int64(s.LightingFailures))), s.LightingFailures != 0)
	p.drawOutputPasses(ed)
	if err := ed.Renderer.LightingError(); err != nil {
		widgets.LabelWrapped(ed.UI, err.Error(), true)
	}
}

func (p *lightingEditor) drawOutputPasses(ed *context.Editor) {
	ui := ed.UI
	widgets.Checkbox(ui, i18n.Tr("editor.render.inspect_passes", "Inspect output passes"), &p.inspectPasses)
	if !p.inspectPasses {
		return
	}
	rows := ed.Renderer.InspectOutputPasses()
	if len(rows) == 0 {
		return
	}
	var ids []view.ID
	var labels []string
	for _, row := range rows {
		if !slices.Contains(ids, row.Output) {
			ids = append(ids, row.Output)
			labels = append(labels, fmt.Sprintf("%d / %s", row.Output.Scope, row.Output.Node.String()))
		}
	}
	selection := max(slices.Index(ids, p.selectedOutput), 0)
	widgets.Combo(ui, ui.ID("render-output"), widgets.Field(ui, i18n.Tr("editor.render.output", "Output")), &selection, labels, false)
	p.selectedOutput = ids[selection]
	formats := [...]string{"RGBA8", "sRGBA8", "R8", "RGBA16F", "D24S8"}
	for _, row := range rows {
		if row.Output != p.selectedOutput {
			continue
		}
		target := "?"
		if row.Window {
			target = i18n.Tr("editor.render.window_target", "Window")
		} else if row.KnownFormat && int(row.Format) < len(formats) {
			target = formats[row.Format]
		}
		timing := i18n.Tr("editor.render.no_gpu_sample", "GPU sample unavailable")
		if row.GPUValid {
			timing = fmt.Sprintf("GPU %.3f ms", float64(row.GPUNS)/1e6)
		}
		label := row.Label
		if row.InstanceID != "" {
			label += " [" + row.InstanceID + "]"
		}
		if label == "" {
			label = i18n.Tr("editor.render.world_pass", "World")
		}
		widgets.LabelWrapped(ui, fmt.Sprintf("%s · %d × %d %s · %s", label, row.Width, row.Height, target, timing), false)
	}
	widgets.LabelDim(ui, i18n.Tr("editor.render.ui_after", "Game UI follows the camera output."))
}
