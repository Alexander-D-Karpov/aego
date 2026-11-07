//go:build aego_editor

package docsview

import (
	"fmt"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/modules/particles"
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
		if !ok || field == "" {
			return nil, fmt.Errorf("unsupported observable %s", observable)
		}
		if _, duplicate := out.Observables[observable]; duplicate {
			return nil, fmt.Errorf("duplicate observable %s", observable)
		}
		if component == particles.NameSystem && field == "LiveParticles" {
			s := particles.Of(a.session.Controller().World())
			if s == nil {
				return nil, fmt.Errorf("particle runtime is unavailable")
			}
			out.Observables[observable] = s.Count(h)
			continue
		}
		var fd *schema.FieldDescriptor
		for i := range td.Fields {
			if strings.EqualFold(td.Fields[i].Name, field) {
				fd = &td.Fields[i]
				break
			}
		}
		if fd == nil {
			return nil, fmt.Errorf("observable field is missing: %s", observable)
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
		default:
			return nil, fmt.Errorf("observable kind %v is not supported", v.Kind)
		}
	}
	return out, nil
}
