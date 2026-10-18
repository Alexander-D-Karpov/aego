package data

import (
	"strconv"
	"unsafe"

	"aego/core/ids"
	"aego/format/aetext"
	"aego/scene/sceneio"
	"aego/schema"
)

func Fresh(td *schema.TypeDescriptor) unsafe.Pointer {
	return fresh(td)
}

func NewValue(reg *schema.Registry, td *schema.TypeDescriptor, fields []aetext.Field) (unsafe.Pointer, []string) {
	p := fresh(td)
	return p, applyFields(reg, td, p, fields, "")
}

func applyFields(reg *schema.Registry, td *schema.TypeDescriptor, p unsafe.Pointer, fields []aetext.Field, prefix string) []string {
	var issues []string
	for _, f := range fields {
		fd, ok := td.Field(f.Name)
		if !ok {
			issues = append(issues, prefix+"unknown field "+f.Name+" is ignored")
			continue
		}
		v, err := sceneio.ReadValue(reg, fd, f.Value)
		if err == nil {
			err = fd.Set(p, v)
		}
		if err != nil {
			issues = append(issues, prefix+f.Name+": "+err.Error())
		}
	}
	return issues
}

func (l *Library) Compose(id ids.AssetID, d *Doc) (*Record, error) {
	td, err := l.typeOf(d.Type)
	if err != nil {
		return nil, err
	}
	rec := &Record{ID: id, Type: td, Ptr: fresh(td), Base: d.Base}
	if d.Base.IsZero() {
		rec.Origin = fresh(td)
	} else {
		if err := l.chain(id, d.Base); err != nil {
			return nil, err
		}
		base, err := l.Record(d.Base)
		if err != nil {
			return nil, err
		}
		if base.Type != td {
			return nil, failf(id, "its base has type "+base.Type.FullName+", it has "+td.FullName)
		}
		td.Copy(rec.Ptr, base.Ptr)
		rec.Origin = base.Ptr
	}
	rec.Issues = applyFields(l.reg, td, rec.Ptr, d.Fields, "")
	return rec, nil
}

func (l *Library) chain(id, base ids.AssetID) error {
	for b, n := base, 0; !b.IsZero(); n++ {
		switch {
		case b == id:
			return failf(id, "its base chain refers back to itself")
		case n >= MaxDepth:
			return failf(id, "its base chain is deeper than "+strconv.Itoa(MaxDepth))
		}
		r, err := l.Record(b)
		if err != nil {
			return err
		}
		b = r.Base
	}
	return nil
}
