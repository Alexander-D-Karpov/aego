package sdk

import (
	"aego/asset"
	"aego/data"
	"aego/engine/module"
	"aego/schema"
)

type (
	DataBuilder[T any] = module.DataBuilder[T]
	DataRef[T any]     = data.Ref[T]
	TableRef[T any]    = data.TableRef[T]
	Weighted[T any]    = data.Weighted[T]
)

func DataType[T any](r *Registry, opts ...schema.Option) *DataBuilder[T] {
	b := module.DataType[T](r.r, opts...)
	r.r.AddDataHook(dataHook[T]())
	return b
}

func Live[T any](r *Registry, fields ...string) {
	module.Live[T](r.r, fields...)
}

func DataLibrary(ctx *Context) (*data.Library, bool) {
	s := SceneOf(ctx)
	src, ok := AssetsOf(ctx).(asset.DataSource)
	if s == nil || !ok {
		return nil, false
	}
	return data.NewLibrary(s.Registry(), src), true
}

func Data[T any](ctx *Context, r DataRef[T]) (*T, error) {
	l, ok := DataLibrary(ctx)
	if !ok {
		return nil, data.ErrNoSource
	}
	return r.Get(l)
}
