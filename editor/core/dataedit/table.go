package dataedit

import (
	"slices"
	"strconv"
	"strings"
	"unsafe"

	"aego/core/errs"
	"aego/core/ids"
	"aego/data"
	"aego/format/aetext"
	"aego/schema"
)

type Row struct {
	ID    string
	Ptr   unsafe.Pointer
	extra []aetext.Field
}

type tableState struct {
	doc *data.TableDoc
	seq uint64
}

type Table struct {
	ID     ids.AssetID
	Path   string
	Type   *schema.TypeDescriptor
	Rows   []Row
	Issues []string
	reg    *schema.Registry
	def    unsafe.Pointer
	undo   []tableState
	redo   []tableState
	seq    uint64
	next   uint64
	saved  uint64
	open   string
	disk   []byte
	rev    uint64
}

func NewTable(reg *schema.Registry, id ids.AssetID, path string, doc *data.TableDoc) (*Table, error) {
	td, err := dataType(reg, doc.Type, path)
	if err != nil {
		return nil, err
	}
	t := &Table{ID: id, Path: path, Type: td, reg: reg, def: data.Fresh(td)}
	t.load(doc)
	return t, nil
}

func (t *Table) load(doc *data.TableDoc) {
	t.Rows, t.Issues = nil, nil
	seen := map[string]bool{}
	for _, r := range doc.Rows {
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
		var extra []aetext.Field
		for _, f := range r.Fields {
			if _, ok := t.Type.Field(f.Name); !ok {
				extra = append(extra, f)
			}
		}
		t.Rows = append(t.Rows, Row{ID: r.ID, Ptr: p, extra: extra})
	}
}

func (t *Table) Doc() *data.TableDoc {
	out := &data.TableDoc{Type: t.Type.FullName}
	for _, r := range t.Rows {
		fs := data.Fields(t.reg, t.Type, r.Ptr, t.def)
		out.Rows = append(out.Rows, data.TableRow{ID: r.ID, Fields: append(fs, cloneFields(r.extra)...)})
	}
	return out
}

func (t *Table) push() {
	t.undo = append(t.undo, tableState{doc: t.Doc(), seq: t.seq})
	if len(t.undo) > maxUndo {
		t.undo = append(t.undo[:0], t.undo[1:]...)
	}
	t.redo = t.redo[:0]
}

func (t *Table) bump() {
	t.next++
	t.seq = t.next
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

func (t *Table) unique(base string) string {
	if t.Index(base) < 0 {
		return base
	}
	for n := 2; ; n++ {
		if id := base + strconv.Itoa(n); t.Index(id) < 0 {
			return id
		}
	}
}

func (t *Table) Set(rows []int, fd *schema.FieldDescriptor, v schema.Value, step bool) error {
	if len(rows) == 0 {
		return nil
	}
	if step || t.open != fd.Name {
		t.push()
		t.open = fd.Name
	}
	for _, i := range rows {
		if err := fd.Set(t.Rows[i].Ptr, v); err != nil {
			return err
		}
	}
	t.bump()
	return nil
}

func (t *Table) Rename(i int, id string) error {
	id = strings.TrimSpace(id)
	switch {
	case id == "":
		return errs.New(errs.EditorCommand, "a row id cannot be empty")
	case id == t.Rows[i].ID:
		return nil
	case t.Index(id) >= 0:
		return errs.New(errs.EditorCommand, "the table already has a row "+id)
	}
	t.push()
	t.open = ""
	t.Rows[i].ID = id
	t.bump()
	return nil
}

func (t *Table) AddRow() int {
	t.push()
	t.open = ""
	t.Rows = append(t.Rows, Row{ID: t.unique("row"), Ptr: data.Fresh(t.Type)})
	t.bump()
	return len(t.Rows) - 1
}

func (t *Table) Duplicate(rows []int) []int {
	if len(rows) == 0 {
		return nil
	}
	t.push()
	t.open = ""
	var made []int
	for _, i := range rows {
		src := t.Rows[i]
		p := data.Fresh(t.Type)
		t.Type.Copy(p, src.Ptr)
		t.Rows = append(t.Rows, Row{ID: t.unique(src.ID + "_copy"), Ptr: p, extra: cloneFields(src.extra)})
		made = append(made, len(t.Rows)-1)
	}
	t.bump()
	return made
}

func (t *Table) Delete(rows []int) {
	if len(rows) == 0 {
		return
	}
	t.push()
	t.open = ""
	out := make([]Row, 0, len(t.Rows))
	for i, r := range t.Rows {
		if !slices.Contains(rows, i) {
			out = append(out, r)
		}
	}
	t.Rows = out
	t.bump()
}

func (t *Table) ExportCSV() ([]byte, error) {
	return data.ExportCSV(t.reg, t.Doc())
}

func (t *Table) ImportCSV(src []byte) error {
	doc, err := data.ImportCSV(t.reg, t.Doc(), src)
	if err != nil {
		return err
	}
	t.push()
	t.open = ""
	t.load(doc)
	t.bump()
	return nil
}

func (t *Table) CanUndo() bool {
	return len(t.undo) > 0
}

func (t *Table) CanRedo() bool {
	return len(t.redo) > 0
}

func (t *Table) Undo() {
	n := len(t.undo)
	if n == 0 {
		return
	}
	t.redo = append(t.redo, tableState{doc: t.Doc(), seq: t.seq})
	s := t.undo[n-1]
	t.undo = t.undo[:n-1]
	t.load(s.doc)
	t.seq, t.open = s.seq, ""
}

func (t *Table) Redo() {
	n := len(t.redo)
	if n == 0 {
		return
	}
	t.undo = append(t.undo, tableState{doc: t.Doc(), seq: t.seq})
	s := t.redo[n-1]
	t.redo = t.redo[:n-1]
	t.load(s.doc)
	t.seq, t.open = s.seq, ""
}

func (t *Table) Dirty() bool {
	return t.seq != t.saved
}

func (t *Table) MarkSaved() {
	t.saved = t.seq
}

func (t *Table) Bytes() []byte {
	return data.WriteTable(t.Doc())
}
