package dataedit

import (
	"unsafe"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/data"
	"aego/format/aetext"
	"aego/schema"
)

const maxUndo = 200

type state struct {
	base   ids.AssetID
	fields []aetext.Field
	seq    uint64
}

func cloneFields(fs []aetext.Field) []aetext.Field {
	if len(fs) == 0 {
		return nil
	}
	out := make([]aetext.Field, len(fs))
	for i, f := range fs {
		out[i] = aetext.Field{Name: f.Name, Value: f.Value.Clone()}
	}
	return out
}

func (s state) clone() state {
	return state{base: s.base, fields: cloneFields(s.fields), seq: s.seq}
}

func dataType(reg *schema.Registry, name, path string) (*schema.TypeDescriptor, error) {
	td, ok := reg.ByName(name)
	if !ok || td.Kind != schema.TypeAsset {
		return nil, errs.New(errs.SchemaUnknownType, "unknown data type "+name, log.F("path", path))
	}
	return td, nil
}

type Doc struct {
	ID     ids.AssetID
	Path   string
	Type   *schema.TypeDescriptor
	reg    *schema.Registry
	lib    *data.Library
	cur    state
	next   uint64
	saved  uint64
	ptr    unsafe.Pointer
	origin unsafe.Pointer
	issues []string
	undo   []state
	redo   []state
	open   string
	disk   []byte
	rev    uint64
}

func NewDoc(reg *schema.Registry, lib *data.Library, id ids.AssetID, path string, src *data.Doc) (*Doc, error) {
	if lib == nil {
		return nil, data.ErrNoSource
	}
	td, err := dataType(reg, src.Type, path)
	if err != nil {
		return nil, err
	}
	d := &Doc{ID: id, Path: path, Type: td, reg: reg, lib: lib, cur: state{base: src.Base, fields: cloneFields(src.Fields)}}
	if err := d.load(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Doc) compose(base ids.AssetID) (*data.Record, error) {
	return d.lib.Compose(d.ID, &data.Doc{Type: d.Type.FullName, Base: base, Fields: d.cur.fields})
}

func (d *Doc) load() error {
	rec, err := d.compose(d.cur.base)
	if err == nil {
		d.ptr, d.origin, d.issues = rec.Ptr, rec.Origin, rec.Issues
		return nil
	}
	if d.cur.base.IsZero() {
		return err
	}
	rec, err2 := d.compose(ids.AssetID{})
	if err2 != nil {
		return err2
	}
	d.ptr, d.origin = rec.Ptr, rec.Origin
	d.issues = append([]string{"the base is ignored: " + err.Error()}, rec.Issues...)
	return nil
}

func (d *Doc) push() {
	d.undo = append(d.undo, d.cur.clone())
	if len(d.undo) > maxUndo {
		d.undo = append(d.undo[:0], d.undo[1:]...)
	}
	d.redo = d.redo[:0]
}

func (d *Doc) bump() {
	d.next++
	d.cur.seq = d.next
}

func (d *Doc) diff() []aetext.Field {
	out := data.Fields(d.reg, d.Type, d.ptr, d.origin)
	for _, f := range d.cur.fields {
		if _, known := d.Type.Field(f.Name); !known {
			out = append(out, f)
		}
	}
	return out
}

func (d *Doc) Ptr() unsafe.Pointer {
	return d.ptr
}

func (d *Doc) Base() ids.AssetID {
	return d.cur.base
}

func (d *Doc) Issues() []string {
	return d.issues
}

func (d *Doc) Value(fd *schema.FieldDescriptor) schema.Value {
	var v schema.Value
	fd.Get(d.ptr, &v)
	return v
}

func (d *Doc) Overridden(name string) bool {
	for _, f := range d.cur.fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

func (d *Doc) Set(fd *schema.FieldDescriptor, v schema.Value, step bool) error {
	if step || d.open != fd.Name {
		d.push()
		d.open = fd.Name
	}
	if err := fd.Set(d.ptr, v); err != nil {
		return err
	}
	d.cur.fields = d.diff()
	d.bump()
	return nil
}

func (d *Doc) Revert(name string) error {
	if !d.Overridden(name) {
		return nil
	}
	d.push()
	d.open = ""
	out := make([]aetext.Field, 0, len(d.cur.fields))
	for _, f := range d.cur.fields {
		if f.Name != name {
			out = append(out, f)
		}
	}
	d.cur.fields = out
	d.bump()
	return d.load()
}

func (d *Doc) SetBase(id ids.AssetID) error {
	if id == d.cur.base {
		return nil
	}
	if id == d.ID {
		return errs.New(errs.EditorCommand, "a data asset cannot be its own base")
	}
	rec, err := d.compose(id)
	if err != nil {
		return err
	}
	d.push()
	d.open = ""
	d.cur.base = id
	d.ptr, d.origin, d.issues = rec.Ptr, rec.Origin, rec.Issues
	d.bump()
	return nil
}

func (d *Doc) CanUndo() bool {
	return len(d.undo) > 0
}

func (d *Doc) CanRedo() bool {
	return len(d.redo) > 0
}

func (d *Doc) Undo() error {
	n := len(d.undo)
	if n == 0 {
		return nil
	}
	d.redo = append(d.redo, d.cur)
	d.cur = d.undo[n-1]
	d.undo = d.undo[:n-1]
	d.open = ""
	return d.load()
}

func (d *Doc) Redo() error {
	n := len(d.redo)
	if n == 0 {
		return nil
	}
	d.undo = append(d.undo, d.cur)
	d.cur = d.redo[n-1]
	d.redo = d.redo[:n-1]
	d.open = ""
	return d.load()
}

func (d *Doc) Dirty() bool {
	return d.cur.seq != d.saved
}

func (d *Doc) MarkSaved() {
	d.saved = d.cur.seq
}

func (d *Doc) Bytes() []byte {
	return data.WriteDoc(&data.Doc{Type: d.Type.FullName, Base: d.cur.base, Fields: d.cur.fields})
}
