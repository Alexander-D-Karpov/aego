package data

import (
	"reflect"
	"slices"
	"strconv"
	"unsafe"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/scene/prefab"
	"aego/scene/sceneio"
	"aego/schema"
)

const (
	MaxDepth  = 16
	KindData  = string(asset.ArtData)
	KindTable = string(asset.ArtTable)
	keyRecord = "data.record"
	keyTable  = "data.table"
)

var ErrNoSource = errs.New(errs.AssetNotMounted, "data assets need a mounted asset loader")

type Record struct {
	ID     ids.AssetID
	Type   *schema.TypeDescriptor
	Ptr    unsafe.Pointer
	Base   ids.AssetID
	Origin unsafe.Pointer
	Issues []string
}

type Table struct {
	ID     ids.AssetID
	Type   *schema.TypeDescriptor
	IDs    []string
	Rows   []unsafe.Pointer
	Issues []string
	index  map[string]int
}

func (t *Table) Row(id string) (unsafe.Pointer, bool) {
	i, ok := t.index[id]
	if !ok {
		return nil, false
	}
	return t.Rows[i], true
}

type cached struct {
	v   any
	err error
}

type Library struct {
	reg     *schema.Registry
	src     asset.DataSource
	dec     asset.Decoder
	records map[ids.AssetID]cached
	tables  map[ids.AssetID]cached
}

func NewLibrary(reg *schema.Registry, src asset.DataSource) *Library {
	l := &Library{reg: reg, src: src}
	l.dec, _ = src.(asset.Decoder)
	return l
}

func (l *Library) Schema() *schema.Registry {
	return l.reg
}

func (l *Library) cache(m *map[ids.AssetID]cached, id ids.AssetID, key string, build func() (any, error)) (any, error) {
	if l.dec != nil {
		return l.dec.Decoded(id, key, build)
	}
	if c, ok := (*m)[id]; ok {
		return c.v, c.err
	}
	v, err := build()
	if *m == nil {
		*m = map[ids.AssetID]cached{}
	}
	(*m)[id] = cached{v: v, err: err}
	return v, err
}

func failf(id ids.AssetID, msg string) error {
	return errs.New(errs.AssetBadArtifact, "data asset "+id.Short()+": "+msg, log.F("asset", id.String()))
}

func (l *Library) typeOf(name string) (*schema.TypeDescriptor, error) {
	td, ok := l.reg.ByName(name)
	if !ok || td.Kind != schema.TypeAsset {
		return nil, errs.New(errs.SchemaUnknownType, "unknown data type "+name, log.F("type", name))
	}
	return td, nil
}

func fresh(td *schema.TypeDescriptor) unsafe.Pointer {
	p := td.New()
	if td.Default != nil && td.Copy != nil {
		td.Copy(p, td.Default())
	}
	return p
}

func (l *Library) apply(td *schema.TypeDescriptor, p unsafe.Pointer, fields []aetext.Field, prefix string) []string {
	return applyFields(l.reg, td, p, fields, prefix)
}

func (l *Library) Record(id ids.AssetID) (*Record, error) {
	return l.record(id, nil)
}

type cachedDecoder interface {
	Cached(id ids.AssetID, key string) (any, bool, error)
}

func (l *Library) record(id ids.AssetID, stack []ids.AssetID) (*Record, error) {
	if c, ok := l.dec.(cachedDecoder); ok {
		if v, hit, err := c.Cached(id, keyRecord); hit {
			if err != nil {
				return nil, err
			}
			return v.(*Record), nil
		}
	}
	v, err := l.cache(&l.records, id, keyRecord, func() (any, error) { return l.build(id, stack) })
	if err != nil {
		return nil, err
	}
	return v.(*Record), nil
}

func (l *Library) build(id ids.AssetID, stack []ids.AssetID) (*Record, error) {
	switch {
	case slices.Contains(stack, id):
		return nil, failf(id, "its base chain refers back to itself")
	case len(stack) >= MaxDepth:
		return nil, failf(id, "its base chain is deeper than "+strconv.Itoa(MaxDepth))
	case l.src == nil:
		return nil, failf(id, "no data source")
	}
	src, ok := l.src.Data(id)
	if !ok {
		return nil, failf(id, "is not an imported data asset")
	}
	doc, err := ParseDoc(src, id.String())
	if err != nil {
		return nil, err
	}
	td, err := l.typeOf(doc.Type)
	if err != nil {
		return nil, err
	}
	rec := &Record{ID: id, Type: td, Ptr: fresh(td), Base: doc.Base}
	if !doc.Base.IsZero() {
		base, err := l.record(doc.Base, append(slices.Clip(stack), id))
		if err != nil {
			return nil, err
		}
		if base.Type != td {
			return nil, failf(id, "its base has type "+base.Type.FullName+", it has "+td.FullName)
		}
		td.Copy(rec.Ptr, base.Ptr)
		rec.Origin = base.Ptr
	}
	rec.Issues = l.apply(td, rec.Ptr, doc.Fields, "")
	return rec, nil
}

func (l *Library) Table(id ids.AssetID) (*Table, error) {
	v, err := l.cache(&l.tables, id, keyTable, func() (any, error) { return l.buildTable(id) })
	if err != nil {
		return nil, err
	}
	return v.(*Table), nil
}

func (l *Library) buildTable(id ids.AssetID) (*Table, error) {
	if l.src == nil {
		return nil, failf(id, "no data source")
	}
	src, ok := l.src.Table(id)
	if !ok {
		return nil, failf(id, "is not an imported table")
	}
	doc, err := ParseTable(src, id.String())
	if err != nil {
		return nil, err
	}
	td, err := l.typeOf(doc.Type)
	if err != nil {
		return nil, err
	}
	t := &Table{ID: id, Type: td, index: make(map[string]int, len(doc.Rows))}
	for _, r := range doc.Rows {
		if r.ID == "" {
			t.Issues = append(t.Issues, "a row without an id is ignored")
			continue
		}
		if _, dup := t.index[r.ID]; dup {
			t.Issues = append(t.Issues, "row "+r.ID+" is declared twice, the second is ignored")
			continue
		}
		p := fresh(td)
		t.Issues = append(t.Issues, l.apply(td, p, r.Fields, "row "+r.ID+": ")...)
		t.index[r.ID] = len(t.Rows)
		t.IDs = append(t.IDs, r.ID)
		t.Rows = append(t.Rows, p)
	}
	return t, nil
}

func sameType[T any](reg *schema.Registry, td *schema.TypeDescriptor) error {
	want, ok := reg.ByGoType(reflect.TypeFor[T]())
	if !ok || want != td {
		return errs.New(errs.SchemaUnknownType, "data has type "+td.FullName+", not "+reflect.TypeFor[T]().String())
	}
	return nil
}

func Get[T any](l *Library, id ids.AssetID) (*T, error) {
	rec, err := l.Record(id)
	if err != nil {
		return nil, err
	}
	if err := sameType[T](l.reg, rec.Type); err != nil {
		return nil, err
	}
	return (*T)(rec.Ptr), nil
}

func Row[T any](l *Library, table ids.AssetID, row string) (*T, bool, error) {
	t, err := l.Table(table)
	if err != nil {
		return nil, false, err
	}
	if err := sameType[T](l.reg, t.Type); err != nil {
		return nil, false, err
	}
	p, ok := t.Row(row)
	return (*T)(p), ok, nil
}

func Fields(reg *schema.Registry, td *schema.TypeDescriptor, p, base unsafe.Pointer) []aetext.Field {
	var out []aetext.Field
	td.Serializable(func(fd *schema.FieldDescriptor) {
		var v schema.Value
		fd.Get(p, &v)
		if base != nil {
			var b schema.Value
			fd.Get(base, &b)
			if prefab.Equal(v, b) {
				return
			}
		}
		out = append(out, aetext.Field{Name: fd.Name, Value: sceneio.WriteValue(reg, fd, v)})
	})
	return out
}
