package scene

import (
	"reflect"

	"aego/core/errs"
	"aego/core/log"
	"aego/schema"
)

type storeFactory func(*Scene) anyStore

type Types struct {
	byID       map[schema.TypeID]storeFactory
	byGo       map[reflect.Type]storeFactory
	schema     *schema.Registry
	ordered    []schema.TypeID
	overlay    Overlay
	properties map[schema.TypeID]BoundPropertyResolver
}

func NewTypes(reg *schema.Registry) *Types {
	return &Types{
		byID:   map[schema.TypeID]storeFactory{},
		byGo:   map[reflect.Type]storeFactory{},
		schema: reg,
	}
}

func (t *Types) Schema() *schema.Registry {
	return t.schema
}

func RegisterComponent[T any](t *Types, opts ...schema.Option) error {
	td, err := schema.Register[T](t.schema, opts...)
	if err != nil {
		return err
	}
	rt := reflect.TypeFor[T]()
	factory := func(s *Scene) anyStore {
		return newStore[T](s, td)
	}
	t.byID[td.ID] = factory
	t.byGo[rt] = factory
	t.ordered = append(t.ordered, td.ID)
	return nil
}

func (t *Types) Each(fn func(schema.TypeID)) {
	for _, id := range t.ordered {
		fn(id)
	}
}

func (t *Types) Has(id schema.TypeID) bool {
	_, ok := t.byID[id]
	return ok
}

func (t *Types) factoryFor(id schema.TypeID) (storeFactory, error) {
	f, ok := t.byID[id]
	if !ok {
		td, known := t.schema.ByID(id)
		name := "unknown"
		if known {
			name = td.FullName
		}
		return nil, errs.New(errs.SceneUnknownComponent,
			"component type is registered in the schema but not as a scene component",
			log.F("type", name))
	}
	return f, nil
}
