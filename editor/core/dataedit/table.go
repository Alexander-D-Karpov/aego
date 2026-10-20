package dataedit

import (
	"bytes"
	"strconv"
	"strings"
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

type Row struct {
	ID  string
	Ptr unsafe.Pointer
}

type Table struct {
	ID     ids.AssetID
	Path   string
	Type   *schema.TypeDescriptor
	Rows   []Row
	Issues []string
	reg    *schema.Registry
	def    unsafe.Pointer
	dd     *mutate.DataDocument
	steps  steps
	saved  []byte
	disk   []byte
	rev    uint64
}

func NewTable(env mutate.Env, id ids.AssetID, path string, raw []byte) (*Table, error) {
	dd, err := mutate.LoadData(raw, path, env)
	if err != nil {
		return nil, err
	}
	if !dd.Table() {
		return nil, errs.New(errs.EditorDocument, path+" is not a table")
	}
	t := &Table{ID: id, Path: path, Type: dd.Type, reg: env.Types.Schema(), def: data.Fresh(dd.Type), dd: dd, saved: dd.Bytes()}
	t.load()
	return t, nil
}

func (t *Table) load() {
	t.Rows, t.Issues = nil, nil
	seen := map[string]bool{}
	for _, r := range t.dd.TableDoc().Rows {
		switch {
		case r.ID == "":
			t.Issues = append(t.Issues, "a row without an id is ignored")
			continue
		case seen[r.ID]:
			t.Issues = append(t.Issues, "row "+r.ID+" is declared twice, the second is ignored")
			continue
		}
		seen[r.ID] = true
		p, issues := data.NewValue(t.reg, t.Type, r.Fields)
		for _, s := range issues {
			t.Issues = append(t.Issues, "row "+r.ID+": "+s)
		}
		t.Rows = append(t.Rows, Row{ID: r.ID, Ptr: p})
	}
}

func (t *Table) apply(key string, step bool, ops ...mutate.Op) error {
	res, err := t.dd.Apply(mutate.Tx{Ops: ops})
	if err != nil {
		return err
	}
	t.steps.record(res, key, step)
	t.load()
	return nil
}

func (t *Table) Doc() *data.TableDoc {
	src := t.dd.TableDoc()
	out := &data.TableDoc{Type: src.Type, Rows: make([]data.TableRow, len(src.Rows))}
	for i, r := range src.Rows {
		out.Rows[i] = data.TableRow{ID: r.ID, Fields: cloneFields(r.Fields)}
	}
	return out
}

func (t *Table) fieldsOf(id string) []aetext.Field {
	for _, r := range t.dd.TableDoc().Rows {
		if r.ID == id {
			return r.Fields
		}
	}
	return nil
}

func (t *Table) has(id, name string) bool {
	for _, f := range t.fieldsOf(id) {
		if f.Name == name {
			return true
		}
	}
	return false
}

func (t *Table) Value(i int, fd *schema.FieldDescriptor) schema.Value {
	var v schema.Value
	fd.Get(t.Rows[i].Ptr, &v)
	return v
}

func (t *Table) Index(id string) int {
	for i := range t.Rows {
		if t.Rows[i].ID == id {
			return i
		}
	}
	return -1
}

func (t *Table) unique(base string, taken map[string]bool) string {
	free := func(id string) bool { return t.Index(id) < 0 && !taken[id] }
	id := base
	for n := 2; !free(id); n++ {
		id = base + strconv.Itoa(n)
	}
	taken[id] = true
	return id
}

func (t *Table) Set(rows []int, fd *schema.FieldDescriptor, v schema.Value, step bool) error {
	if len(rows) == 0 {
		return nil
	}
	var dv schema.Value
	fd.Get(t.def, &dv)
	same := prefab.Equal(dv, v)
	keys := []string{fd.Name}
	var ops []mutate.Op
	for _, i := range rows {
		id := t.Rows[i].ID
		keys = append(keys, id)
		switch {
		case !same:
			ops = append(ops, mutate.DataSetValue(id, fd.Name, sceneio.WriteValue(t.reg, fd, v)))
		case t.has(id, fd.Name):
			ops = append(ops, mutate.DataReset(id, fd.Name))
		}
	}
	if len(ops) == 0 {
		return nil
	}
	return t.apply(strings.Join(keys, "\x00"), step, ops...)
}

func (t *Table) Rename(i int, id string) error {
	id = strings.TrimSpace(id)
	old := t.Rows[i].ID
	switch {
	case id == "":
		return errs.New(errs.EditorCommand, "a row id cannot be empty")
	case id == old:
		return nil
	case t.Index(id) >= 0:
		return errs.New(errs.EditorCommand, "the table already has a row "+id)
	}
	return t.apply("", true, mutate.RowRename(old, id))
}

func (t *Table) AddRow() (int, error) {
	id := t.unique("row", map[string]bool{})
	if err := t.apply("", true, mutate.RowAdd(id)); err != nil {
		return -1, err
	}
	return t.Index(id), nil
}

func (t *Table) Duplicate(rows []int) ([]int, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	taken := map[string]bool{}
	made := make([]string, 0, len(rows))
	ops := make([]mutate.Op, 0, len(rows))
	for _, i := range rows {
		src := t.Rows[i].ID
		id := t.unique(src+"_copy", taken)
		made = append(made, id)
		ops = append(ops, mutate.RowRestore(id, t.fieldsOf(src)))
	}
	if err := t.apply("", true, ops...); err != nil {
		return nil, err
	}
	out := make([]int, len(made))
	for k, id := range made {
		out[k] = t.Index(id)
	}
	return out, nil
}

func (t *Table) Delete(rows []int) error {
	if len(rows) == 0 {
		return nil
	}
	ops := make([]mutate.Op, 0, len(rows))
	for _, i := range rows {
		ops = append(ops, mutate.RowRemove(t.Rows[i].ID))
	}
	return t.apply("", true, ops...)
}

func (t *Table) ExportCSV() ([]byte, error) {
	return data.ExportCSV(t.reg, t.Doc())
}

func (t *Table) ImportCSV(src []byte) error {
	doc, err := data.ImportCSV(t.reg, t.Doc(), src)
	if err != nil {
		return err
	}
	var ops []mutate.Op
	for _, r := range t.dd.TableDoc().Rows {
		ops = append(ops, mutate.RowRemove(r.ID))
	}
	for _, r := range doc.Rows {
		ops = append(ops, mutate.RowRestore(r.ID, r.Fields))
	}
	return t.apply("", true, ops...)
}

func (t *Table) CanUndo() bool {
	return len(t.steps.undo) > 0
}

func (t *Table) CanRedo() bool {
	return len(t.steps.redo) > 0
}

func (t *Table) Undo() error {
	if err := t.steps.back(t.dd); err != nil {
		return err
	}
	t.load()
	return nil
}

func (t *Table) Redo() error {
	if err := t.steps.forward(t.dd); err != nil {
		return err
	}
	t.load()
	return nil
}

func (t *Table) Dirty() bool {
	return !bytes.Equal(t.dd.Bytes(), t.saved)
}

func (t *Table) MarkSaved() {
	t.saved = t.dd.Bytes()
}

func (t *Table) Bytes() []byte {
	return t.dd.Bytes()
}
