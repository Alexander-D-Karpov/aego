package editor

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/render/shader"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
	"math"
)

func typedValue(v schema.Value) aetext.Value {
	switch v.Kind {
	case schema.KindFloat32, schema.KindFloat64:
		return aetext.Float64(v.F)
	case schema.KindInt32, schema.KindInt64:
		return aetext.Int(v.I)
	case schema.KindBool:
		return aetext.Bool(v.I != 0)
	case schema.KindAssetRef:
		return aetext.AssetRef(v.Asset.String())
	}
	var a []float32
	switch v.Kind {
	case schema.KindVec2:
		a = []float32{v.V.X, v.V.Y}
	case schema.KindVec3:
		a = []float32{v.V3.X, v.V3.Y, v.V3.Z}
	case schema.KindColor:
		a = []float32{v.C.R, v.C.G, v.C.B, v.C.A}
	default:
		a = []float32{v.V4.X, v.V4.Y, v.V4.Z, v.V4.W}
	}
	out := aetext.Value{Kind: aetext.VList, List: make([]aetext.Value, len(a))}
	for i, f := range a {
		out.List[i] = aetext.Float64(float64(f))
	}
	return out
}

func assetPicker(ed *context.Editor, id core.ID, label, kind string, value *ids.AssetID) bool {
	ui := ed.UI
	current := 0
	names := []string{i18n.Tr("editor.material.none", "None")}
	idsList := []ids.AssetID{{}}
	if ed.Pipeline != nil {
		for _, a := range ed.Pipeline.Assets() {
			if !context.AssetMatches(kind, a.Kind) {
				continue
			}
			if a.ID == *value {
				current = len(names)
			}
			names = append(names, a.Path)
			idsList = append(idsList, a.ID)
		}
	}
	for _, a := range asset.Builtins() {
		if context.AssetMatches(kind, a.Kind) {
			if a.ID == *value {
				current = len(names)
			}
			names = append(names, a.Name)
			idsList = append(idsList, a.ID)
		}
	}
	if !value.IsZero() && current == 0 {
		current = len(names)
		names = append(names, i18n.Tr("editor.material.unresolved_choice", "Unresolved {asset}", i18n.Str("asset", value.Short())))
		idsList = append(idsList, *value)
	}
	r := widgets.Field(ui, label)
	if widgets.Combo(ui, id, r, &current, names, false) {
		*value = idsList[current]
		return true
	}
	if ed.DnD != nil {
		if payload, ok := ed.DnD.Payload(); ok && ed.DnD.Hovering(ui, r) {
			ap, isAsset := payload.Data.(context.AssetPayload)
			compatible := payload.Kind == context.KindAsset && isAsset && context.AssetMatches(kind, ap.Kind)
			// A secondary sampler needs the full texture. Sprite atlas regions
			// carry a different sampling contract and cannot be assigned here.
			if kind == "texture" && compatible {
				compatible = ap.Kind == asset.ArtTexture || ap.Kind == asset.ArtAtlasPage
			}
			if compatible {
				if got, dropped := ed.DnD.Target(ui, r, context.KindAsset); dropped {
					*value = got.Data.(context.AssetPayload).ID
					return true
				}
			} else {
				ed.DnD.Target(ui, r, "reject")
			}
		}
	}
	return false
}

func parameterControl(ed *context.Editor, p *shader.Param, value *schema.Value) widgets.Edit {
	ui := ed.UI
	label := p.Display
	if label == "" {
		label = p.Name
	}
	r := widgets.Field(ui, label)
	id := ui.ID(p.Name)
	opts := widgets.NumberOpts{Speed: 0.01, Prec: 4, Min: float64(p.Min), Max: float64(p.Max), HasMin: p.HasRange, HasMax: p.HasRange}
	switch value.Kind {
	case schema.KindFloat32:
		v := float32(value.F)
		e := widgets.DragFloat32(ui, id, r, &v, opts)
		*value = schema.Float32(v)
		return e
	case schema.KindInt32:
		v := value.I
		opts.Int = true
		opts.Speed = 1
		opts.Min, opts.Max, opts.HasMin, opts.HasMax = math.MinInt32, math.MaxInt32, true, true
		if p.HasRange {
			opts.Min = math.Max(opts.Min, float64(p.Min))
			opts.Max = math.Min(opts.Max, float64(p.Max))
		}
		e := widgets.DragInt64(ui, id, r, &v, opts)
		*value = schema.Int32(int32(v))
		return e
	case schema.KindBool:
		b := value.I != 0
		changed := widgets.ToggleButton(ui, id, r, i18n.Tr("editor.material.enabled", "Enabled"), &b)
		*value = schema.Bool(b)
		return widgets.Edit{Begin: changed, Changed: changed, End: changed}
	case schema.KindVec2:
		return widgets.Vec2Field(ui, id, r, &value.V, opts, false, false)
	case schema.KindVec3:
		return widgets.Vec3Field(ui, id, r, &value.V3, opts, [3]bool{})
	case schema.KindColor:
		return widgets.ColorField(ui, id, r, &value.C, false)
	case schema.KindVec4:
		ptrs := [4]*float32{&value.V4.X, &value.V4.Y, &value.V4.Z, &value.V4.W}
		var result widgets.Edit
		w := (r.W - 9) / 4
		for i, ptr := range ptrs {
			part := mathx.Rect{X: r.X + float32(i)*(w+3), Y: r.Y, W: w, H: r.H}
			result = result.Or(widgets.DragFloat32(ui, core.IDSeq(id, i), part, ptr, opts))
		}
		return result
	}
	return widgets.Edit{}
}
