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
	return RequiredPick(s, td, h, nil)
}

// Choice is a RequiresOneOf group the node lacks; Pick is the index of the alternative to add.
type Choice struct {
	Of   *schema.TypeDescriptor
	Alts []*schema.TypeDescriptor
	Pick int
}

// Choices lists the RequiresOneOf groups of td and its requirements that node h does not satisfy (D344).
func Choices(s *scene.Scene, td *schema.TypeDescriptor, h scene.NodeHandle) []Choice {
	var out []Choice
	_, _ = requiredWith(s, td, h, func(of *schema.TypeDescriptor, alts []*schema.TypeDescriptor) *schema.TypeDescriptor {
		out = append(out, Choice{Of: of, Alts: alts})
		return alts[0]
	})
	return out
}

// RequiredPick is Required with the alternative of each RequiresOneOf group taken from picks, in Choices order.
func RequiredPick(s *scene.Scene, td *schema.TypeDescriptor, h scene.NodeHandle, picks []int) ([]*schema.TypeDescriptor, error) {
	n := 0
	return requiredWith(s, td, h, func(_ *schema.TypeDescriptor, alts []*schema.TypeDescriptor) *schema.TypeDescriptor {
		i := 0
		if n < len(picks) && picks[n] >= 0 && picks[n] < len(alts) {
			i = picks[n]
		}
		n++
		return alts[i]
	})
}

func requiredWith(s *scene.Scene, td *schema.TypeDescriptor, h scene.NodeHandle, pick func(of *schema.TypeDescriptor, alts []*schema.TypeDescriptor) *schema.TypeDescriptor) ([]*schema.TypeDescriptor, error) {
	reg := s.Registry()
	var out []*schema.TypeDescriptor
	var walk func(t *schema.TypeDescriptor) error
	need := func(t, req *schema.TypeDescriptor) error {
		if slices.Contains(out, req) {
			return nil
		}
		if _, has := s.GetByType(h, req.ID); has {
			return nil
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
		return nil
	}
	walk = func(t *schema.TypeDescriptor) error {
		for _, name := range t.Requires {
			req, ok := reg.ByName(name)
			if !ok {
				return fmt.Errorf("%s needs %s, which no module registers", displayName(t), name)
			}
			if err := need(t, req); err != nil {
				return err
			}
		}
		for _, names := range t.RequiresAny {
			var alts []*schema.TypeDescriptor
			satisfied := false
			for _, name := range names {
				req, ok := reg.ByName(name)
				if !ok {
					continue
				}
				if _, has := s.GetByType(h, req.ID); has || slices.Contains(out, req) {
					satisfied = true
					break
				}
				if s.Accepts(h, req.ID) {
					alts = append(alts, req)
				}
			}
			if satisfied {
				continue
			}
			if len(alts) == 0 {
				return fmt.Errorf("%s needs one of %s, and none fits this node", displayName(t), strings.Join(names, ", "))
			}
			if err := need(t, pick(t, alts)); err != nil {
				return err
			}
		}
		return nil
	}
	return out, walk(td)
}

// Conflict reports a component on node h that td cannot share the node with (D344).
func Conflict(s *scene.Scene, td *schema.TypeDescriptor, h scene.NodeHandle) (*schema.TypeDescriptor, bool) {
	reg := s.Registry()
	for _, other := range reg.Components() {
		if _, has := s.GetByType(h, other.ID); !has {
			continue
		}
		if slices.Contains(td.Conflicts, other.FullName) || slices.Contains(other.Conflicts, td.FullName) {
			return other, true
		}
	}
	return nil, false
}

// Component resolves a component name the way add-comp does: full, or a unique short name.
func (d *Document) Component(name string) (*schema.TypeDescriptor, error) {
	return d.component(name)
}
