package mutate

import (
	"bytes"
	"slices"
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/data"
	"aego/format/aetext"
	"aego/scene/sceneio"
	"aego/schema"
)

type DataDocument struct {
	Name  string
	Type  *schema.TypeDescriptor
	doc   *data.Doc
	table *data.TableDoc
	vals  *Document
}

func LoadData(src []byte, name string, env Env) (*DataDocument, error) {
	reg := env.Types.Schema()
	d := &DataDocument{Name: name, vals: &Document{env: env, reg: reg}}
	var typ string
	if bytes.HasPrefix(bytes.TrimSpace(src), []byte("table")) {
		t, err := data.ParseTable(src, name)
		if err != nil {
			return nil, err
		}
		d.table, typ = t, t.Type
	} else {
		doc, err := data.ParseDoc(src, name)
		if err != nil {
			return nil, err
		}
		d.doc, typ = doc, doc.Type
	}
	td, ok := reg.ByName(typ)
	if !ok || td.Kind != schema.TypeAsset {
		return nil, errs.New(errs.SchemaUnknownType, "unknown data type "+typ, log.F("file", name))
	}
	d.Type = td
	return d, nil
}

func (d *DataDocument) Table() bool {
	return d.table != nil
}

func (d *DataDocument) Bytes() []byte {
	if d.table != nil {
		return data.WriteTable(d.table)
	}
	return data.WriteDoc(d.doc)
}

func (d *DataDocument) Apply(tx Tx) (*Result, error) {
	return run(d, tx)
}

func (d *DataDocument) DryRun(tx Tx) (*Result, error) {
	return dryRun(d, tx)
}

func (d *DataDocument) row(id string) int {
	for i := range d.table.Rows {
		if d.table.Rows[i].ID == id {
			return i
		}
	}
	return -1
}

func (d *DataDocument) fields(row string) (*[]aetext.Field, error) {
	if d.table == nil {
		if row != "" {
			return nil, errs.New(errs.EditorCommand, "a data asset has no rows")
		}
		return &d.doc.Fields, nil
	}
	i := d.row(row)
	if i < 0 {
		return nil, errs.New(errs.EditorCommand, "no row "+strconv.Quote(row))
	}
	return &d.table.Rows[i].Fields, nil
}

func (d *DataDocument) do(op Op, res *Result) (step, error) {
	switch op.Kind {
	case KindDataSet:
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		return d.set(op, v, false)
	case KindDataReset:
		return d.set(op, aetext.Value{}, true)
	case KindDataBase:
		return d.setBase(op)
	}
	if d.table == nil && dataKinds[op.Kind] {
		return step{}, errs.New(errs.EditorCommand, op.Kind+" applies to tables")
	}
	switch op.Kind {
	case KindRowAdd:
		return d.rowAdd(op, nil)
	case KindRowRestore:
		var fs []aetext.Field
		if op.Values != nil {
			fs = op.Values.Fields
		}
		return d.rowAdd(op, fs)
	case KindRowRemove:
		return d.rowRemove(op)
	case KindRowRename:
		return d.rowRename(op)
	}
	return step{}, errs.New(errs.EditorCommand, op.Kind+" does not apply to data assets")
}

func cloneFields(fs []aetext.Field) []aetext.Field {
	out := make([]aetext.Field, len(fs))
	for i, f := range fs {
		out[i] = aetext.Field{Name: f.Name, Value: f.Value.Clone()}
	}
	return out
}

func (d *DataDocument) rowAdd(op Op, fs []aetext.Field) (step, error) {
	if op.Node == "" || d.row(op.Node) >= 0 {
		return step{}, errs.New(errs.EditorCommand, "a row needs a new, non-empty id", log.F("row", op.Node))
	}
	i := len(d.table.Rows)
	if op.HasIndex && op.Index >= 0 && op.Index < i {
		i = op.Index
	}
	d.table.Rows = slices.Insert(d.table.Rows, i, data.TableRow{ID: op.Node, Fields: cloneFields(fs)})
	redo := Op{Kind: KindRowRestore, Node: op.Node, Index: i, HasIndex: true, Values: &aetext.Node{Head: "values", Fields: cloneFields(fs)}}
	return step{inv: RowRemove(op.Node), redo: redo, change: Change{Op: op.Kind, Node: op.Node}}, nil
}

func (d *DataDocument) rowRemove(op Op) (step, error) {
	i := d.row(op.Node)
	if i < 0 {
		return step{}, errs.New(errs.EditorCommand, "no row "+strconv.Quote(op.Node))
	}
	r := d.table.Rows[i]
	inv := Op{Kind: KindRowRestore, Node: r.ID, Index: i, HasIndex: true, Values: &aetext.Node{Head: "values", Fields: cloneFields(r.Fields)}}
	d.table.Rows = slices.Delete(d.table.Rows, i, i+1)
	return step{inv: inv, redo: RowRemove(r.ID), change: Change{Op: op.Kind, Node: r.ID}}, nil
}

func (d *DataDocument) rowRename(op Op) (step, error) {
	i := d.row(op.Node)
	if i < 0 {
		return step{}, errs.New(errs.EditorCommand, "no row "+strconv.Quote(op.Node))
	}
	if op.Name == "" || d.row(op.Name) >= 0 {
		return step{}, errs.New(errs.EditorCommand, "the new row id must be new and non-empty", log.F("row", op.Name))
	}
	d.table.Rows[i].ID = op.Name
	return step{inv: RowRename(op.Name, op.Node), redo: RowRename(op.Node, op.Name),
		change: Change{Op: op.Kind, Node: op.Node, Before: op.Node, After: op.Name}}, nil
}

func (d *DataDocument) fresh(fd *schema.FieldDescriptor) schema.Value {
	if d.Type.New == nil {
		return schema.Value{Kind: fd.Kind}
	}
	var v schema.Value
	fd.Get(data.Fresh(d.Type), &v)
	return v
}

func (d *DataDocument) set(op Op, raw aetext.Value, reset bool) (step, error) {
	list, err := d.fields(op.Node)
	if err != nil {
		return step{}, err
	}
	parts := strings.Split(op.Path, ".")
	if len(parts) > 2 {
		return step{}, errs.New(errs.EditorProps, "nested struct fields are not addressable yet", log.F("path", op.Path))
	}
	name, index, err := splitIndex(parts[0])
	if err != nil {
		return step{}, err
	}
	fd, ok := field(d.Type, name)
	if !ok {
		return step{}, errs.New(errs.EditorProps, d.Type.FullName+" has no field "+name)
	}
	lane := ""
	if len(parts) == 2 {
		lane = strings.ToLower(parts[1])
	}
	at := slices.IndexFunc(*list, func(f aetext.Field) bool { return f.Name == fd.Name })
	var cur schema.Value
	if at >= 0 {
		if cur, err = d.vals.read(fd, (*list)[at].Value); err != nil {
			return step{}, err
		}
	} else {
		cur = d.fresh(fd)
	}
	ch := Change{Op: op.Kind, Node: op.Node, Path: fd.Name, Before: d.vals.text(fd, cur)}
	inv := DataReset(op.Node, fd.Name)
	if at >= 0 {
		inv = Op{Kind: KindDataSet, Node: op.Node, Path: fd.Name, Value: (*list)[at].Value.Clone(), HasValue: true}
	}
	if reset {
		if at < 0 {
			return step{}, ErrNothing
		}
		*list = slices.Delete(*list, at, at+1)
		ch.After = ""
		return step{inv: inv, redo: DataReset(op.Node, fd.Name), change: ch}, nil
	}
	nv, err := d.vals.next(fd, cur, raw, lane, index, op.Path)
	if err != nil {
		return step{}, err
	}
	if err := d.vals.validate(fd, nv, d.Type.FullName+"."+fd.Name, false); err != nil {
		return step{}, err
	}
	out := sceneio.WriteValue(d.vals.reg, fd, nv)
	if at >= 0 {
		(*list)[at].Value = out
	} else {
		*list = append(*list, aetext.Field{Name: fd.Name, Value: out})
	}
	ch.After = d.vals.text(fd, nv)
	return step{inv: inv, redo: Op{Kind: KindDataSet, Node: op.Node, Path: fd.Name, Value: out.Clone(), HasValue: true}, change: ch}, nil
}

func (d *DataDocument) Doc() *data.Doc {
	return d.doc
}

func (d *DataDocument) TableDoc() *data.TableDoc {
	return d.table
}

func baseText(id ids.AssetID) string {
	if id.IsZero() {
		return ""
	}
	return id.String()
}

func (d *DataDocument) setBase(op Op) (step, error) {
	if d.table != nil {
		return step{}, errs.New(errs.EditorCommand, "a table has no base")
	}
	var id ids.AssetID
	if op.Asset != "" {
		var err error
		if id, err = d.vals.assetID(op.Asset); err != nil {
			return step{}, err
		}
		if d.vals.env.Assets != nil && !d.vals.env.Assets(id) {
			return step{}, errNoAsset(op.Asset)
		}
	}
	old := d.doc.Base
	d.doc.Base = id
	ch := Change{Op: op.Kind, Path: "base", Before: baseText(old), After: baseText(id)}
	return step{inv: DataBase(baseText(old)), redo: DataBase(baseText(id)), change: ch}, nil
}
