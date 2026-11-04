package dataedit

import (
	"bytes"
	"unsafe"

	"aego/core/errs"
	"aego/core/ids"
	"aego/data"
	"aego/format/aetext"
	"aego/scene/mutate"
	"aego/scene/prefab"
	"aego/scene/sceneio"
	"aego/schema"
)

const maxUndo = 200

type entry struct {
	key  string
	inv  mutate.Tx
	redo mutate.Tx
}

type steps struct {
	undo []entry
	redo []entry
	open string
}

func (h *steps) record(res *mutate.Result, key string, step bool) {
	n := len(h.undo)
	if !step && key != "" && h.open == key && n > 0 {
		h.undo[n-1].redo = res.Redo
	} else {
		h.undo = append(h.undo, entry{key: key, inv: res.Inverse, redo: res.Redo})
		if len(h.undo) > maxUndo {
			h.undo = append(h.undo[:0], h.undo[1:]...)
		}
	}
	h.redo = h.redo[:0]
	h.open = key
}

func (h *steps) back(dd *mutate.DataDocument) error {
	n := len(h.undo)
	if n == 0 {
		return nil
	}
	e := h.undo[n-1]
	if _, err := dd.Apply(e.inv); err != nil {
		return err
	}
	h.undo = h.undo[:n-1]
	h.redo = append(h.redo, e)
	h.open = ""
	return nil
}

func (h *steps) forward(dd *mutate.DataDocument) error {
	n := len(h.redo)
	if n == 0 {
		return nil
	}
	e := h.redo[n-1]
	if _, err := dd.Apply(e.redo); err != nil {
		return err
	}
	h.redo = h.redo[:n-1]
	h.undo = append(h.undo, e)
	h.open = ""
	return nil
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

type Doc struct {
	edits    uint64
	ID       ids.AssetID
	Path     string
	Type     *schema.TypeDescriptor
	reg      *schema.Registry
	lib      *data.Library
	dd       *mutate.DataDocument
	ptr      unsafe.Pointer
	origin   unsafe.Pointer
	issues   []string
	steps    steps
	saved    []byte
	disk     []byte
	conflict []byte
	rev      uint64
}

func NewDoc(env mutate.Env, lib *data.Library, id ids.AssetID, path string, raw []byte) (*Doc, error) {
	if lib == nil {
		return nil, data.ErrNoSource
	}
	dd, err := mutate.LoadData(raw, path, env)
	if err != nil {
		return nil, err
	}
	if dd.Table() {
		return nil, errs.New(errs.EditorDocument, path+" is a table, open it in the table editor")
	}
	d := &Doc{ID: id, Path: path, Type: dd.Type, reg: env.Types.Schema(), lib: lib, dd: dd, saved: dd.Bytes()}
	if err := d.load(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Doc) fields() []aetext.Field {
	return d.dd.Doc().Fields
}

func (d *Doc) compose(base ids.AssetID) (*data.Record, error) {
	return d.lib.Compose(d.ID, &data.Doc{Type: d.Type.FullName, Base: base, Fields: d.fields()})
}

func (d *Doc) load() error {
	d.edits++
	base := d.Base()
	rec, err := d.compose(base)
	if err == nil {
		d.ptr, d.origin, d.issues = rec.Ptr, rec.Origin, rec.Issues
		return nil
	}
	if base.IsZero() {
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

func (d *Doc) apply(key string, step bool, ops ...mutate.Op) error {
	res, err := d.dd.Apply(mutate.Tx{Ops: ops})
	if err != nil {
		return err
	}
	d.steps.record(res, key, step)
	return d.load()
}

func (d *Doc) Ptr() unsafe.Pointer {
	return d.ptr
}

func (d *Doc) Base() ids.AssetID {
	return d.dd.Doc().Base
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
	for _, f := range d.fields() {
		if f.Name == name {
			return true
		}
	}
	return false
}

func (d *Doc) Set(fd *schema.FieldDescriptor, v schema.Value, step bool) error {
	if d.origin != nil {
		var inherited schema.Value
		fd.Get(d.origin, &inherited)
		if prefab.Equal(inherited, v) {
			if !d.Overridden(fd.Name) {
				return nil
			}
			return d.apply(fd.Name, step, mutate.DataReset("", fd.Name))
		}
	}
	return d.apply(fd.Name, step, mutate.DataSetValue("", fd.Name, sceneio.WriteValue(d.reg, fd, v)))
}

func (d *Doc) Revert(name string) error {
	if !d.Overridden(name) {
		return nil
	}
	return d.apply("", true, mutate.DataReset("", name))
}

func (d *Doc) SetBase(id ids.AssetID) error {
	if id == d.Base() {
		return nil
	}
	if id == d.ID {
		return errs.New(errs.EditorCommand, "a data asset cannot be its own base")
	}
	if _, err := d.compose(id); err != nil {
		return err
	}
	text := ""
	if !id.IsZero() {
		text = id.String()
	}
	return d.apply("", true, mutate.DataBase(text))
}

func (d *Doc) CanUndo() bool {
	return len(d.steps.undo) > 0
}

func (d *Doc) CanRedo() bool {
	return len(d.steps.redo) > 0
}

func (d *Doc) Undo() error {
	if err := d.steps.back(d.dd); err != nil {
		return err
	}
	return d.load()
}

func (d *Doc) Redo() error {
	if err := d.steps.forward(d.dd); err != nil {
		return err
	}
	return d.load()
}

func (d *Doc) Dirty() bool {
	return !bytes.Equal(d.dd.Bytes(), d.saved)
}

func (d *Doc) MarkSaved() {
	d.saved = d.dd.Bytes()
}

func (d *Doc) Bytes() []byte {
	return d.dd.Bytes()
}
