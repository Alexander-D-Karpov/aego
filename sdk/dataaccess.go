package sdk

import (
	"errors"
	"reflect"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/data"
	"aego/engine/module"
	"aego/schema"
	"aego/script/bind"
)

type dataLibs struct {
	src asset.DataSource
	reg *schema.Registry
	lib *data.Library
}

func (d *dataLibs) of(c *bind.Ctx) (*data.Library, error) {
	if c.Scene == nil {
		return nil, data.ErrNoSource
	}
	src, ok := c.Scene.Resolver().(asset.DataSource)
	if !ok {
		return nil, data.ErrNoSource
	}
	if reg := c.Scene.Registry(); d.lib == nil || d.src != src || d.reg != reg {
		d.src, d.reg, d.lib = src, reg, data.NewLibrary(reg, src)
	}
	return d.lib, nil
}

func dataShort(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

func dataHook[T any]() module.DataHook {
	var td *schema.TypeDescriptor
	libs := &dataLibs{}
	return module.DataHook{
		Refs: func(r *module.Registry) error {
			b := r.Bindings()
			t, ok := r.Schema().ByGoType(reflect.TypeFor[T]())
			if b == nil || !ok || t.Kind != schema.TypeAsset {
				return nil
			}
			td = t
			name := dataShort(t.FullName)
			return errors.Join(
				bind.AssetRefType[data.Ref[T]](b, bind.DataPrefix+name),
				bind.AssetRefType[data.TableRef[T]](b, bind.TablePrefix+name),
			)
		},
		Bind: func(r *module.Registry) error {
			b := r.Bindings()
			if b == nil || td == nil {
				return nil
			}
			skipped, err := bind.DataAccess[T](b, dataShort(td.FullName), bind.DataLoaders[T]{
				Record: func(c *bind.Ctx, id ids.AssetID) (*T, error) {
					l, err := libs.of(c)
					if err != nil {
						return nil, err
					}
					return data.Get[T](l, id)
				},
				Row: func(c *bind.Ctx, id ids.AssetID, row string) (*T, bool, error) {
					l, err := libs.of(c)
					if err != nil {
						return nil, false, err
					}
					return data.Row[T](l, id, row)
				},
				Rows: func(c *bind.Ctx, id ids.AssetID) (int, error) {
					l, err := libs.of(c)
					if err != nil {
						return 0, err
					}
					t, err := l.Table(id)
					if err != nil {
						return 0, err
					}
					if t.Type != td {
						return 0, errors.New("table " + id.Short() + " holds " + t.Type.FullName + ", not " + td.FullName)
					}
					return len(t.IDs), nil
				},
			}, bind.Doc(td.Doc))
			if err != nil {
				return err
			}
			if len(skipped) > 0 {
				r.Warn(td.FullName, "fields "+strings.Join(skipped, ", ")+" have no script form and are hidden from scripts")
			}
			return nil
		},
	}
}
