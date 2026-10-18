package data

import "aego/core/ids"

type Ref[T any] struct {
	ID ids.AssetID
}

func (r *Ref[T]) AssetRefID() ids.AssetID      { return r.ID }
func (r *Ref[T]) SetAssetRefID(id ids.AssetID) { r.ID = id }
func (r *Ref[T]) AssetKind() string            { return KindData }
func (r Ref[T]) IsZero() bool                  { return r.ID.IsZero() }

func (r Ref[T]) Get(l *Library) (*T, error) {
	return Get[T](l, r.ID)
}

type TableRef[T any] struct {
	ID ids.AssetID
}

func (r *TableRef[T]) AssetRefID() ids.AssetID      { return r.ID }
func (r *TableRef[T]) SetAssetRefID(id ids.AssetID) { r.ID = id }
func (r *TableRef[T]) AssetKind() string            { return KindTable }
func (r TableRef[T]) IsZero() bool                  { return r.ID.IsZero() }

func (r TableRef[T]) Row(l *Library, id string) (*T, bool, error) {
	return Row[T](l, r.ID, id)
}
