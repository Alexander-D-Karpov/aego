package scene

import (
	"aego/schema"
	"fmt"
)

// PropertyBinding is an asset-driven component property resolved outside the
// animation hot path. Get/Set must reject an obsolete resource generation and
// reacquire the component through the node handle instead of retaining pointers.
type PropertyBinding struct {
	Key   string
	Kind  schema.Kind
	Valid func(*Scene, NodeHandle) bool
	Get   func(*Scene, NodeHandle, *schema.Value) bool
	Set   func(*Scene, NodeHandle, schema.Value) bool
}

type PropertyResolver func(*Scene, NodeHandle, string) (*PropertyBinding, error)

// BoundProperty carries the resource identity alongside its shared access
// methods. A dynamic property can therefore be resolved for the first time
// during playback without allocating a closure per operation.
type BoundProperty struct {
	Key        string
	Kind       schema.Kind
	Resource   any
	Generation uint64
	Access     *PropertyAccess
}

type PropertyAccess struct {
	Valid func(BoundProperty, *Scene, NodeHandle) bool
	Get   func(BoundProperty, *Scene, NodeHandle, *schema.Value) bool
	Set   func(BoundProperty, *Scene, NodeHandle, schema.Value) bool
}

func (p BoundProperty) Valid(s *Scene, h NodeHandle) bool {
	return p.Access != nil && (p.Access.Valid == nil || p.Access.Valid(p, s, h))
}
func (p BoundProperty) Get(s *Scene, h NodeHandle, v *schema.Value) bool {
	return p.Access != nil && p.Access.Get != nil && p.Access.Get(p, s, h, v)
}
func (p BoundProperty) Set(s *Scene, h NodeHandle, v schema.Value) bool {
	return p.Access != nil && p.Access.Set != nil && p.Access.Set(p, s, h, v)
}

type BoundPropertyResolver func(*Scene, NodeHandle, string) (BoundProperty, error)

var callbackAccess = PropertyAccess{
	Valid: func(p BoundProperty, s *Scene, h NodeHandle) bool {
		b := p.Resource.(*PropertyBinding)
		return b.Valid == nil || b.Valid(s, h)
	},
	Get: func(p BoundProperty, s *Scene, h NodeHandle, v *schema.Value) bool {
		return p.Resource.(*PropertyBinding).Get(s, h, v)
	},
	Set: func(p BoundProperty, s *Scene, h NodeHandle, v schema.Value) bool {
		return p.Resource.(*PropertyBinding).Set(s, h, v)
	},
}

// RegisterProperties preserves the callback provider API for modules.
func (t *Types) RegisterProperties(component string, resolve PropertyResolver) error {
	if resolve == nil {
		return fmt.Errorf("invalid property provider for %s", component)
	}
	return t.RegisterBoundProperties(component, func(s *Scene, h NodeHandle, key string) (BoundProperty, error) {
		b, err := resolve(s, h, key)
		if b == nil || err != nil {
			return BoundProperty{}, err
		}
		return BoundProperty{Key: b.Key, Kind: b.Kind, Resource: b, Access: &callbackAccess}, nil
	})
}

// RegisterBoundProperties extends a registered component through the existing type
// registry. Runtime property consumers do not know the defining module.
func (t *Types) RegisterBoundProperties(component string, resolve BoundPropertyResolver) error {
	if t.schema.Frozen() {
		return fmt.Errorf("property registration after schema freeze")
	}
	td, ok := t.schema.ByName(component)
	if !ok || td.Kind != schema.TypeComponent || resolve == nil {
		return fmt.Errorf("invalid property provider for %s", component)
	}
	if t.properties == nil {
		t.properties = make(map[schema.TypeID]BoundPropertyResolver)
	}
	if t.properties[td.ID] != nil {
		return fmt.Errorf("duplicate property provider for %s", component)
	}
	t.properties[td.ID] = resolve
	return nil
}

func (t *Types) BindProperty(s *Scene, h NodeHandle, typ schema.TypeID, name string) (BoundProperty, error) {
	if resolve := t.properties[typ]; resolve != nil {
		return resolve(s, h, name)
	}
	return BoundProperty{}, nil
}

func (t *Types) ResolveProperty(s *Scene, h NodeHandle, typ schema.TypeID, name string) (*PropertyBinding, error) {
	b, err := t.BindProperty(s, h, typ, name)
	if err != nil || b.Access == nil {
		return nil, err
	}
	if b.Access == &callbackAccess {
		return b.Resource.(*PropertyBinding), nil
	}
	return &PropertyBinding{Key: b.Key, Kind: b.Kind, Valid: b.Valid, Get: b.Get, Set: b.Set}, nil
}
