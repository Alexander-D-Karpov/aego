package mutate

import (
	"fmt"
	"slices"
	"strings"

	"aego/scene"
	"aego/schema"
)

func displayName(td *schema.TypeDescriptor) string {
	if td.DisplayName != "" {
		return td.DisplayName
	}
	return td.FullName
}

func inScene(s *scene.Scene, t schema.TypeID) bool {
	for _, h := range s.PreOrder() {
		if _, ok := s.GetByType(h, t); ok && s.IsAlive(h) {
			return true
		}
	}
	return false
}

// Required lists the components td needs that node h lacks, dependencies first (D144).
func Required(s *scene.Scene, td *schema.TypeDescriptor, h scene.NodeHandle) ([]*schema.TypeDescriptor, error) {
	reg := s.Registry()
	var out []*schema.TypeDescriptor
	var walk func(t *schema.TypeDescriptor) error
	walk = func(t *schema.TypeDescriptor) error {
		for _, name := range t.Requires {
			req, ok := reg.ByName(name)
			if !ok {
				return fmt.Errorf("%s needs %s, which no module registers", displayName(t), name)
			}
			if slices.Contains(out, req) {
				continue
			}
			if _, has := s.GetByType(h, req.ID); has {
				continue
			}
			if !s.Accepts(h, req.ID) {
				return fmt.Errorf("%s needs %s, which does not fit a %s node", displayName(t), displayName(req), strings.ToUpper(spaceName(s.Space(h))))
			}
			if req.SceneUnique && inScene(s, req.ID) {
				return fmt.Errorf("%s needs %s, and the scene already has one", displayName(t), displayName(req))
			}
			if err := walk(req); err != nil {
				return err
			}
			out = append(out, req)
		}
		return nil
	}
	return out, walk(td)
}

// Component resolves a component name the way add-comp does: full, or a unique short name.
func (d *Document) Component(name string) (*schema.TypeDescriptor, error) {
	return d.component(name)
}
