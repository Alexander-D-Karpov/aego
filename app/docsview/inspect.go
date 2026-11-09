//go:build aego_editor

package docsview

import (
	"bytes"
	"fmt"
	"strings"

	"aego/asset"
	"aego/audio/codec"
	"aego/core/ids"
	"aego/modules/audio"
	"aego/modules/particles"
	"aego/modules/render2d"
	"aego/modules/savegame"
	"aego/scene"
	"aego/scene/prefab"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
)

// Inspection is a read-only observation of the running Try world, not the
// source text or a simulated result. Fields refer to the selected component.
type Inspection struct {
	Components       []string       `json:"components"`
	Dependencies     []string       `json:"dependencies"`
	Observables      map[string]any `json:"observables"`
	RuntimeExecuting bool           `json:"runtimeExecuting"`
}

func (a *App) inspectTry(component string, fields, dependencies []string) (*Inspection, error) {
	if component == "" || len(fields) > 256 || len(dependencies) > 256 {
		return nil, fmt.Errorf("invalid Try inspection request")
	}
	sc := a.session.Controller().Scene()
	if sc == nil {
		return nil, fmt.Errorf("the Try scene is not running")
	}
	td, ok := sc.Registry().ByName(component)
	if !ok {
		return nil, fmt.Errorf("component is not registered: %s", component)
	}
	selected := a.session.Selected()
	if len(selected) != 1 {
		return nil, fmt.Errorf("the Try scene does not have exactly one selected node")
	}
	h := sc.Handle(selected[0])
	if h.IsZero() || !sc.IsAlive(h) {
		return nil, fmt.Errorf("selected node is absent from the running scene")
	}
	if _, ok := sc.GetByType(h, td.ID); !ok {
		return nil, fmt.Errorf("selected node does not have %s", component)
	}
	st := a.session.State()
	out := &Inspection{Components: []string{td.FullName}, Observables: map[string]any{}, RuntimeExecuting: st.Running && !st.Paused && st.Frames > 0}
	seen := make(map[string]bool, len(dependencies))
	for _, dep := range dependencies {
		if seen[dep] {
			return nil, fmt.Errorf("duplicate dependency %s", dep)
		}
		seen[dep] = true
		id, err := ids.ParseAssetID(dep)
		if err != nil {
			return nil, fmt.Errorf("invalid dependency %s: %w", dep, err)
		}
		kind := asset.ArtifactKind("")
		if builtin, ok := asset.LookupBuiltin(id); ok {
			kind = builtin.Kind
		} else if entry, ok := a.loader.Index().Lookup(id); ok && !entry.Missing {
			kind = entry.Kind
		}
		resolved := false
		switch kind {
		case asset.ArtMesh:
			_, resolved = a.loader.Mesh(id)
		case asset.ArtTexture, asset.ArtSpriteRegion, asset.ArtAtlasPage:
			_, resolved = a.loader.Texture(id)
		case asset.ArtMaterial:
			_, resolved = a.loader.Material(id)
		case asset.ArtShader:
			_, resolved = a.loader.Shader(id)
		case asset.ArtAnim:
			_, resolved = a.loader.Anim(id)
		case asset.ArtCtrl:
			_, resolved = a.loader.Controller(id)
		case asset.ArtPrefab:
			// A registered ID or raw bytes alone do not establish that an
			// instance can be loaded: parse and expand the actual prefab,
			// including its nested prefab dependencies. Expand preserves
			// missing nested prefabs as placeholders, so reject those too.
			lib := sceneio.NewLibrary(sc.Registry(), a.loader)
			if source, ok := lib.Prefab(id); ok {
				tree, _, err := prefab.Expand(lib, source, nil)
				if err == nil && tree != nil {
					resolved = true
					tree.Walk(func(n, _ *prefab.Node) { resolved = resolved && !n.Missing })
				}
			}
		case asset.ArtAudio:
			raw, ok := a.loader.Audio(id)
			resolved = ok && validTryAudio(raw)
		case "":
			return nil, fmt.Errorf("unresolved dependency %s", dep)
		default:
			return nil, fmt.Errorf("dependency kind %s is not supported by Try inspection", kind)
		}
		if !resolved {
			return nil, fmt.Errorf("unresolved dependency %s", dep)
		}
		out.Dependencies = append(out.Dependencies, dep)
	}
	for _, observable := range fields {
		field, ok := strings.CutPrefix(observable, component+".")
		target, targetComponent := h, component
		if path, crossNode := strings.CutPrefix(observable, "node:"); crossNode {
			name, property, valid := strings.Cut(path, ":")
			if !valid || name == "" || len(name) > 256 {
				return nil, fmt.Errorf("invalid target node observable %s", observable)
			}
			target = sc.Find(sc.Root(), name)
			if target.IsZero() || !sc.IsAlive(target) {
				return nil, fmt.Errorf("observable node %s is absent", name)
			}
			field, ok = property, strings.HasPrefix(property, "Node.")
			if !ok {
				for _, typ := range sc.Registry().Components() {
					if suffix, has := strings.CutPrefix(property, typ.FullName+"."); has {
						targetComponent, field, ok = typ.FullName, suffix, true
						break
					}
				}
			}
		}
		if !ok || field == "" || len(observable) > 512 {
			return nil, fmt.Errorf("unsupported observable %s", observable)
		}
		if _, duplicate := out.Observables[observable]; duplicate {
			return nil, fmt.Errorf("duplicate observable %s", observable)
		}
		// Cross-node observations must use that node and its component, never
		// the selected node handled by the legacy component-specific cases.
		if target != h || targetComponent != component {
			value, err := a.observeValue(sc, target, targetComponent, field)
			if err != nil {
				return nil, err
			}
			out.Observables[observable] = value
			continue
		}
		if component == particles.NameSystem && field == "LiveParticles" {
			s := particles.Of(a.session.Controller().World())
			if s == nil {
				return nil, fmt.Errorf("particle runtime is unavailable")
			}
			out.Observables[observable] = s.Count(h)
			continue
		}
		if component == audio.NameListener && field == "ListenerX" {
			s := audio.Of(a.session.Controller().World())
			if s == nil {
				return nil, fmt.Errorf("audio runtime is unavailable")
			}
			out.Observables[observable] = float64(s.Listener().X)
			continue
		}
		if component == audio.NameZone && field == "WorldGain" {
			s := audio.Of(a.session.Controller().World())
			zone, ok := scene.Get[audio.AudioZone2D](sc, h)
			if s == nil || !ok {
				return nil, fmt.Errorf("audio zone runtime is unavailable")
			}
			out.Observables[observable] = float64(s.WorldBusGain(zone.Bus))
			continue
		}
		if component == savegame.MarkerName && field == "PositionX" {
			out.Observables[observable] = float64(sc.WorldPosition(h).X)
			continue
		}
		if component == scene.TagsName && field == "HasEnemy" {
			out.Observables[observable] = sc.HasTag(h, "Enemy")
			continue
		}
		if component == scriptcomp.ScriptName && field == "AttachmentEnabled" {
			scripts, ok := scriptcomp.ScriptsOf(sc, h)
			if !ok || len(scripts.Attachments) != 1 {
				return nil, fmt.Errorf("exactly one script attachment is required")
			}
			out.Observables[observable] = scripts.Attachments[0].Enabled
			continue
		}
		if component == scriptcomp.ScriptName && field == "PositionX" {
			out.Observables[observable] = float64(sc.WorldPosition(h).X)
			continue
		}
		if component == sceneio.PrefabName {
			switch field {
			case "LinkedPrefab", "ReferencePrefab", "PositionX", "ReferencePositionX":
				target := h
				if field == "ReferencePrefab" || field == "ReferencePositionX" {
					target = sc.Find(sc.Root(), "Reference instance")
				}
				if target.IsZero() || !sc.IsAlive(target) {
					return nil, fmt.Errorf("prefab instance is absent")
				}
				instance, ok := sceneio.InstanceOf(sc, target)
				if !ok || !instance.Owner || instance.Missing || instance.Prefab.IsZero() {
					return nil, fmt.Errorf("node is not a resolved authored prefab instance")
				}
				if field == "LinkedPrefab" || field == "ReferencePrefab" {
					out.Observables[observable] = instance.Prefab.String()
				} else {
					out.Observables[observable] = float64(sc.WorldPosition(target).X)
				}
				continue
			}
		}
		if component == "engine.MaterialOverride" {
			switch field {
			case "FlashValue", "ReferenceFlash":
				target := h
				if field == "ReferenceFlash" {
					target = sc.Find(sc.Root(), "Reference instance")
					if target.IsZero() || !sc.IsAlive(target) {
						return nil, fmt.Errorf("material reference instance is absent")
					}
				}
				value, err := render2d.GetMaterialParameter(sc, target, "Flash")
				if err != nil || value.Kind != schema.KindFloat32 {
					return nil, fmt.Errorf("material Flash is unavailable: %v", err)
				}
				out.Observables[observable] = value.F
				continue
			case "FlashExplicit":
				o, ok := scene.Get[render2d.MaterialOverride](sc, h)
				m, err := render2d.NodeMaterial(sc, h)
				if !ok || err != nil {
					return nil, fmt.Errorf("material override is unavailable: %v", err)
				}
				out.Observables[observable] = o.ExplicitFor(m, "Flash")
				continue
			}
		}
		var fd *schema.FieldDescriptor
		for i := range td.Fields {
			if strings.EqualFold(td.Fields[i].Name, field) {
				fd = &td.Fields[i]
				break
			}
		}
		if fd == nil {
			value, err := a.observeValue(sc, target, targetComponent, field)
			if err != nil {
				return nil, err
			}
			out.Observables[observable] = value
			continue
		}
		v, err := sc.GetProperty(h, td.ID, fd.ID)
		if err != nil {
			return nil, err
		}
		switch v.Kind {
		case schema.KindBool:
			out.Observables[observable] = v.I != 0
		case schema.KindInt32, schema.KindInt64, schema.KindEnum:
			out.Observables[observable] = v.I
		case schema.KindFloat32, schema.KindFloat64:
			out.Observables[observable] = v.F
		case schema.KindString:
			out.Observables[observable] = v.S
		case schema.KindList:
			if fd.Elem != schema.KindString {
				return nil, fmt.Errorf("observable list kind %v is not supported", fd.Elem)
			}
			list := make([]string, len(v.List))
			for i, item := range v.List {
				if item.Kind != schema.KindString {
					return nil, fmt.Errorf("observable list contains a non-string value")
				}
				list[i] = item.S
			}
			out.Observables[observable] = list
		default:
			return nil, fmt.Errorf("observable kind %v is not supported", v.Kind)
		}
	}
	return out, nil
}

// Resolve the prepared artifact and open its real audio payload. Merely
// finding an audio ID is not enough evidence that a Try can play the sound.
func validTryAudio(raw []byte) bool {
	a, err := asset.DecodeAudio(raw)
	if err != nil || len(a.Data) == 0 || a.Frames <= 0 {
		return false
	}
	d, err := codec.Open(bytes.NewReader(a.Data), "")
	if err != nil {
		return false
	}
	info := d.Info()
	err = d.Close()
	return err == nil && info.SampleRate == a.Rate && info.Channels == a.Channels && (info.Frames < 0 || info.Frames == a.Frames)
}
