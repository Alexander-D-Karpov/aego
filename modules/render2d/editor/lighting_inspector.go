package editor

import (
	"aego/contracts/shapes2d"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/engine"
	"aego/engine/i18n"
	"aego/modules/render2d"
	"aego/render/lighting"
	"aego/scene"
	"aego/schema"
	"aego/ui/widgets"
)

type lightingInspector struct {
	p    *lightingEditor
	kind string
}

func visibleLightField(l *render2d.Light2D, name string) bool {
	switch name {
	case "range", "height", "falloff":
		return l.Type != lighting.Directional
	case "elevation":
		return l.Type == lighting.Directional
	case "innerAngle", "outerAngle":
		return l.Type == lighting.Spot
	case "cookie":
		return l.Type == lighting.Sprite
	case "shadows":
		return l.Type != lighting.Directional || l.Shadows
	case "shadowCasterMask", "shadowSoftness":
		return l.Type != lighting.Directional && l.Shadows
	}
	return true
}
func (i *lightingInspector) FieldVisible(ed *context.Editor, nodes []ids.NodeID, field *schema.FieldDescriptor) bool {
	if ed == nil || ed.Docs == nil || ed.Doc() == nil || len(nodes) == 0 {
		return true
	}
	s := ed.Doc().Scene()
	read := false
	for _, id := range nodes {
		h := s.Handle(id)
		switch i.kind {
		case "engine.Light2D":
			if l, ok := scene.Get[render2d.Light2D](s, h); ok {
				read = true
				if visibleLightField(l, field.Name) {
					return true
				}
			}
		case "engine.LightOccluder2D":
			if o, ok := scene.Get[render2d.LightOccluder2D](s, h); ok {
				read = true
				switch field.Name {
				case "polygon":
					if o.Source == render2d.OccluderPolygon && (len(nodes) != 1 || ed.Collapsed["lighting.coords/"+id.String()]) {
						return true
					}
				case "closed":
					if o.Source == render2d.OccluderPolygon {
						return true
					}
				case "shapeNode", "shape":
					if o.Source == render2d.OccluderShapeProvider {
						return true
					}
				default:
					return true
				}
			}
		default:
			return true
		}
	}
	// Mixed selections expose the union, so fields are never lost merely
	// because the first selected node has another light/geometry type.
	return !read
}

func (i *lightingInspector) Draw(ed *context.Editor, nodes []ids.NodeID, defaults func()) {
	if ed != nil && ed.Docs != nil && ed.Doc() != nil && len(nodes) == 1 && i.kind == "engine.LightOccluder2D" {
		sc := ed.Doc().Scene()
		if o, ok := scene.Get[render2d.LightOccluder2D](sc, sc.Handle(nodes[0])); ok && o.Source == render2d.OccluderPolygon {
			lightingFold(ed, "lighting.coords/"+nodes[0].String(), i18n.Tr("editor.lighting.vertex_values", "Vertex coordinates ({n})", i18n.Int("n", int64(len(o.Polygon)))))
		}
	}
	defaults()
	if ed == nil || ed.Docs == nil || ed.Doc() == nil {
		return
	}
	if ed.Renderer == nil || ed.Renderer.Caps().Renderer == "null" {
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.no_gpu", "Lighting preview unavailable: this session has no graphics device."), false)
	}
	if len(nodes) != 1 {
		return
	}
	s := ed.Doc().Scene()
	h := s.Handle(nodes[0])
	switch i.kind {
	case "engine.Light2D":
		l, ok := scene.Get[render2d.Light2D](s, h)
		if !ok {
			return
		}
		if l.Type == lighting.Directional {
			widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.directional_help", "Direction follows local +X; elevation points light toward the surface. Directional shadows are unsupported."), false)
		} else {
			widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.height_help", "Height affects normal-mapped shading. Shadows remain planar; range is measured in world units."), false)
		}
		if l.Type == lighting.Spot {
			widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.spot_handles", "Drag the range, direction and cone handles in Scene View. Cone values are full angles in radians."), false)
		}
		if !l.Cookie.IsZero() {
			if _, ok := l.Cookie.Region(s.Resolver()); !ok {
				widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.missing_cookie", "Cookie texture is unresolved. Its AssetID is preserved; restore it or choose another texture."), true)
			}
		}
	case "engine.LightOccluder2D":
		o, ok := scene.Get[render2d.LightOccluder2D](s, h)
		if !ok {
			return
		}
		if o.Source == render2d.OccluderSpriteAlpha {
			widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.sprite_frame_shadow", "Uses the current sprite frame’s alpha."), false)
		}
		if o.Source == render2d.OccluderPolygon {
			widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.polygon_handles", "Drag vertices to reshape the occluder, drag an edge midpoint to insert a vertex, or Alt-click a vertex to remove it. One gesture is one Undo."), false)
			if err := shapes2d.ValidateContour(o.Polygon, o.Closed); err != nil {
				widgets.LabelWrapped(ed.UI, err.Error(), true)
			}
		} else if o.Source == render2d.OccluderShapeProvider && ed.SceneLighting != nil {
			world := ed.SceneLighting.LightingWorld()
			if _, ok := engine.ServiceOf[shapes2d.Provider](world, shapes2d.ServiceID); !ok {
				widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.missing_provider", "No shape provider is available in this view. Use Polygon or load a module that provides local geometry."), true)
			}
		}
	case "engine.LightingEnvironment2D":
		widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.environment_help", "These settings belong to this camera output. UI stays outside scene tone mapping. Emission does not illuminate neighbouring objects."), false)
		if ed.SceneLighting != nil {
			selected := ed.SceneLighting.LightingEnvironmentPreview()
			if selected == nodes[0] {
				widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.environment_active", "This environment is previewed in Scene View. Inspector changes apply there immediately; Game View and Play keep their own environment selection."), false)
			} else if widgets.Button(ed.UI, i18n.Tr("editor.lighting.environment_preview", "Preview in Scene View")) {
				ed.SceneLighting.SetLightingEnvironmentPreview(nodes[0])
			}
			if !selected.IsZero() && widgets.Button(ed.UI, i18n.Tr("editor.lighting.environment_reset", "Use scene environment defaults")) {
				ed.SceneLighting.SetLightingEnvironmentPreview(ids.NodeID{})
			}
		}
		if ed.Renderer != nil {
			if err := ed.Renderer.LightingError(); err != nil {
				widgets.LabelWrapped(ed.UI, err.Error(), true)
			}
			if ed.SceneLighting != nil {
				if info, ok := ed.Renderer.LightingOutput(ed.SceneLighting.LightingOutput()); ok {
					mode := "LDR"
					if info.Active == lighting.HDR {
						mode = "HDR"
					}
					widgets.LabelWrapped(ed.UI, i18n.Tr("editor.lighting.output_mode", "Current Scene View output: {mode}, {width} × {height}, {bytes} pixel-storage bytes", i18n.Str("mode", mode), i18n.Int("width", int64(info.Width)), i18n.Int("height", int64(info.Height)), i18n.Int("bytes", int64(info.Bytes))), false)
					if info.Reason != "" {
						widgets.LabelWrapped(ed.UI, info.Reason, true)
					}
				}
			}
		}
	}
	if lightingFold(ed, "lighting.debug/"+nodes[0].String(), i18n.Tr("editor.lighting.shadow_details", "Shadow mask diagnostics")) {
		i.p.preview.draw(ed)
	}
}

// Expanded controls are editor state only; default to the editable outline in Scene View.
func lightingFold(ed *context.Editor, key, label string) bool {
	on := ed.Collapsed[key]
	if widgets.TreeNode(ed.UI, ed.UI.ID(key), label, &on, false) {
		on = !on
	}
	if on {
		if ed.Collapsed == nil {
			ed.Collapsed = make(map[string]bool)
		}
		ed.Collapsed[key] = true
	} else {
		delete(ed.Collapsed, key)
	}
	return on
}
