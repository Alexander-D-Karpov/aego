package scene

import (
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
	"aego/schema"
)

func Add[T any](s *Scene, h NodeHandle) *T {
	p, _ := storeOf[T](s).add(h)
	return p
}

func Get[T any](s *Scene, h NodeHandle) (*T, bool) {
	return storeOf[T](s).get(h)
}

func Has[T any](s *Scene, h NodeHandle) bool {
	return storeOf[T](s).hasNode(h)
}

func Remove[T any](s *Scene, h NodeHandle) {
	storeOf[T](s).removeNode(h)
}

func (s *Scene) Add[T any](h NodeHandle) *T {
	return Add[T](s, h)
}

func (s *Scene) Get[T any](h NodeHandle) (*T, bool) {
	return Get[T](s, h)
}

func (s *Scene) Has[T any](h NodeHandle) bool {
	return Has[T](s, h)
}

func (s *Scene) Remove[T any](h NodeHandle) {
	Remove[T](s, h)
}

func (s *Scene) Store[T any]() *ComponentStore[T] {
	return storeOf[T](s)
}

func (s *Scene) storeFor(t schema.TypeID) (anyStore, error) {
	if st, ok := s.storesByID[t]; ok {
		return st, nil
	}
	factory, err := s.types.factoryFor(t)
	if err != nil {
		return nil, err
	}
	return factory(s), nil
}

func (s *Scene) AddByType(h NodeHandle, t schema.TypeID) (unsafe.Pointer, error) {
	st, err := s.storeFor(t)
	if err != nil {
		return nil, err
	}
	p, ok := st.addRaw(h)
	if !ok {
		return nil, errs.New(errs.SceneDeadNode, "cannot add a component to a destroyed node")
	}
	return p, nil
}

func (s *Scene) GetByType(h NodeHandle, t schema.TypeID) (unsafe.Pointer, bool) {
	st, ok := s.storesByID[t]
	if !ok {
		return nil, false
	}
	return st.pointer(h)
}

func (s *Scene) RemoveByType(h NodeHandle, t schema.TypeID) {
	if st, ok := s.storesByID[t]; ok {
		st.removeNode(h)
	}
}

func (s *Scene) ComponentsOf(h NodeHandle, fn func(schema.TypeID)) {
	for _, st := range s.storeList {
		if st.hasNode(h) {
			fn(st.typeID())
		}
	}
}

func (s *Scene) GetProperty(h NodeHandle, t schema.TypeID, f schema.FieldID) (schema.Value, error) {
	p, td, err := s.target(h, t)
	if err != nil {
		return schema.Value{}, err
	}
	fd, ok := td.FieldByID(f)
	if !ok {
		return schema.Value{}, errs.New(errs.SchemaBadField, "unknown field")
	}
	var v schema.Value
	fd.Get(p, &v)
	return v, nil
}

func (s *Scene) SetProperty(h NodeHandle, t schema.TypeID, f schema.FieldID, v schema.Value) error {
	p, td, err := s.target(h, t)
	if err != nil {
		return err
	}
	fd, ok := td.FieldByID(f)
	if !ok {
		return errs.New(errs.SchemaBadField, "unknown field")
	}
	if fd.Flags.Has(schema.FlagReadOnly) {
		return errs.New(errs.SceneReadOnly, "field is read-only", log.F("field", fd.Name))
	}
	if err := fd.Set(p, v); err != nil {
		return err
	}
	if fd.Flags.Has(schema.FlagSerialize) {
		s.touch()
	}
	s.emit(Event{Kind: EvPropertyChanged, Node: h, ID: s.ID(h), Type: t, Field: f})
	return nil
}

func (s *Scene) Stats() Stats {
	st := Stats{
		Nodes:      s.nodes.Len(),
		PendingOps: s.pendingOps,
	}
	for _, store := range s.storeList {
		st.Components += store.count()
	}
	for _, h := range s.PreOrder() {
		if s.IsAlive(h) {
			st.Alive++
		}
	}
	return st
}
