//go:build aego_editor

package docsview

import (
	"fmt"
	"strings"

	"aego/contracts/persistence"
	"aego/engine"
	"aego/modules/audio"
	"aego/modules/camera"
	"aego/modules/gameplay"
	"aego/modules/particles"
	"aego/modules/render2d"
	"aego/save"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

// observeValue reads actual runtime state; it does not execute an action or
// manufacture a result for the contract harness.
func (a *App) observeValue(sc *scene.Scene, node scene.NodeHandle, component, field string) (any, error) {
	w := a.session.Controller().World()
	switch field {
	case "Node.X":
		return float64(sc.WorldPosition(node).X), nil
	case "Node.Y":
		return float64(sc.WorldPosition(node).Y), nil
	case "Node.Rotation":
		return float64(sc.Local(node).Rotation), nil
	case "Node.ScaleX":
		return float64(sc.Local(node).Scale.X), nil
	case "Node.Enabled":
		return sc.Enabled(node), nil
	case "World.Nodes":
		return sc.Stats().Alive - 1, nil
	case "World.PrefabInstances":
		count := 0
		for _, h := range sc.PreOrder() {
			if sc.IsAlive(h) {
				if _, ok := sceneio.InstanceOf(sc, h); ok {
					count++
				}
			}
		}
		return count, nil
	case "LiveParticles":
		if component == particles.NameSystem {
			if s := particles.Of(w); s != nil {
				return s.Count(node), nil
			}
		}
	case "Audio.ListenerIsSelected":
		if s := audio.Of(w); s != nil {
			return s.ListenerNode() == node, nil
		}
	case "Audio.WorldGain":
		if zone, ok := scene.Get[audio.AudioZone2D](sc, node); ok {
			if s := audio.Of(w); s != nil {
				return float64(s.WorldBusGain(zone.Bus)), nil
			}
		}
	case "Audio.Voices":
		if s := audio.Of(w); s != nil {
			return s.Voices(), nil
		}
	case "Gameplay.Hits", "Gameplay.Deaths", "Gameplay.Respawns", "Gameplay.Pickups":
		if s := gameplay.Of(w); s != nil {
			st := s.Stats()
			switch field {
			case "Gameplay.Hits":
				return st.Hits, nil
			case "Gameplay.Deaths":
				return st.Deaths, nil
			case "Gameplay.Respawns":
				return st.Respawns, nil
			default:
				return st.Pickups, nil
			}
		}
	case "Camera.Trauma":
		if s := camera.Of(w); s != nil {
			return float64(s.Trauma("")), nil
		}
	}
	if tag, ok := strings.CutPrefix(field, "Tags.Count."); ok && scene.ValidTag(tag) {
		out := make([]scene.NodeHandle, sc.Stats().Alive)
		return sc.WithTag(tag, out), nil
	}
	if name, ok := strings.CutPrefix(field, "Material."); ok && name != "" {
		v, err := render2d.GetMaterialParameter(sc, node, name)
		if err != nil {
			return nil, err
		}
		return scalarValue(v, "")
	}
	if slot, ok := strings.CutPrefix(field, "Save.Transform."); ok && slot != "" {
		st, ok := engine.Service[*save.Store](a.e, persistence.StoreID)
		if !ok || st == nil {
			return nil, fmt.Errorf("the Try memory save store is unavailable")
		}
		if !st.Exists(slot) {
			return false, nil
		}
		doc, err := st.Load(slot)
		if err != nil {
			return nil, err
		}
		for _, n := range doc.Nodes {
			if n.Label != sc.ID(node).String() {
				continue
			}
			for _, b := range n.Blocks {
				if b.Head == "transform" || b.Head == "transform3d" {
					return true, nil
				}
			}
		}
		return false, nil
	}
	td, ok := sc.Registry().ByName(component)
	if !ok {
		return nil, fmt.Errorf("observable component is not registered: %s", component)
	}
	name, axis, _ := strings.Cut(field, ".")
	for i := range td.Fields {
		fd := &td.Fields[i]
		if strings.EqualFold(fd.Name, name) {
			v, err := sc.GetProperty(node, td.ID, fd.ID)
			if err != nil {
				return nil, err
			}
			return scalarValue(v, axis)
		}
	}
	return nil, fmt.Errorf("observable field is missing: %s.%s", component, field)
}

func scalarValue(v schema.Value, axis string) (any, error) {
	if axis != "" {
		if v.Kind == schema.KindVec2 {
			switch axis {
			case "X":
				return float64(v.V.X), nil
			case "Y":
				return float64(v.V.Y), nil
			}
		}
		return nil, fmt.Errorf("observable kind %s has no axis %s", v.Kind, axis)
	}
	switch v.Kind {
	case schema.KindBool:
		return v.I != 0, nil
	case schema.KindInt32, schema.KindInt64, schema.KindEnum:
		return v.I, nil
	case schema.KindFloat32, schema.KindFloat64:
		return v.F, nil
	case schema.KindString:
		return v.S, nil
	case schema.KindAssetRef:
		return v.Asset.String(), nil
	case schema.KindNodeRef:
		return v.Node.String(), nil
	case schema.KindVec2:
		return []float64{float64(v.V.X), float64(v.V.Y)}, nil
	case schema.KindList:
		out := make([]any, len(v.List))
		for i, item := range v.List {
			value, err := scalarValue(item, "")
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	}
	return nil, fmt.Errorf("observable kind %s is not supported", v.Kind)
}
