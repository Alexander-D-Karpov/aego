package agent

import (
	"errors"

	"aego/data"
	"aego/format/aetext"
	"aego/project"
	"aego/scene/mutate"
)

func dataCommands() []*Command {
	file := Arg{Name: "file", Type: "string", Doc: ".aedata or .aetable file", Required: true, Pos: true}
	row := Arg{Name: "row", Type: "string", Doc: "row id, for a table"}
	id := Arg{Name: "id", Type: "string", Doc: "row id", Required: true, Pos: true}
	csv := Arg{Name: "csv", Type: "string", Doc: "csv file, relative to the project", Required: true, Pos: true}
	field := Arg{Name: "path", Type: "string", Doc: "field, field.x or field[2]", Required: true, Pos: true}
	return []*Command{
		{Name: "data.get", Doc: "fields set in a data asset or a table row; omitted fields use the base or the type default", Args: []Arg{file, row}, Run: (*Session).dataGet},
		{Name: "data.set", Doc: "set a field", Write: true, Args: []Arg{file, field, argValue, row}, Run: func(s *Session, a Args) (*Output, error) {
			return s.dataTx(a, mutate.DataSet(a.Str("row"), a.Str("path"), a.Str("value")))
		}},
		{Name: "data.reset", Doc: "remove a field so it falls back to the base or the default", Write: true, Args: []Arg{file, field, row}, Run: func(s *Session, a Args) (*Output, error) {
			return s.dataTx(a, mutate.DataReset(a.Str("row"), a.Str("path")))
		}},
		{Name: "data.base", Doc: "set or clear the base asset of a data asset", Write: true,
			Args: []Arg{file, {Name: "asset", Type: "string", Doc: "base id or path, empty to clear", Pos: true}}, Run: func(s *Session, a Args) (*Output, error) {
				return s.dataTx(a, mutate.DataBase(a.Str("asset")))
			}},
		{Name: "data.rows", Doc: "row ids of a table", Args: []Arg{file}, Run: (*Session).dataRows},
		{Name: "data.add-row", Doc: "add a row with defaults", Write: true, Args: []Arg{file, id}, Run: func(s *Session, a Args) (*Output, error) {
			return s.dataTx(a, mutate.RowAdd(a.Str("id")))
		}},
		{Name: "data.remove-row", Doc: "remove a row", Write: true, Args: []Arg{file, id}, Run: func(s *Session, a Args) (*Output, error) {
			return s.dataTx(a, mutate.RowRemove(a.Str("id")))
		}},
		{Name: "data.rename-row", Doc: "change a row id", Write: true, Args: []Arg{file, id, {Name: "name", Type: "string", Doc: "new id", Required: true, Pos: true}}, Run: func(s *Session, a Args) (*Output, error) {
			return s.dataTx(a, mutate.RowRename(a.Str("id"), a.Str("name")))
		}},
		{Name: "data.import", Doc: "replace the rows of a table with a csv file", Write: true, Args: []Arg{file, csv}, Run: (*Session).dataImport},
		{Name: "data.export", Doc: "write the rows of a table to a csv file", Write: true, Args: []Arg{file, csv}, Run: (*Session).dataExport},
	}
}

func (s *Session) dataDoc(a Args) (string, []byte, *mutate.DataDocument, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return "", nil, nil, err
	}
	src, err := s.read(rel)
	if err != nil {
		return "", nil, nil, err
	}
	dd, err := mutate.LoadData(src, rel, s.env())
	return rel, src, dd, err
}

func (s *Session) dataTx(a Args, ops ...mutate.Op) (*Output, error) {
	rel, src, dd, err := s.dataDoc(a)
	if err != nil {
		return nil, err
	}
	tx := mutate.Tx{Label: "Data", Ops: ops}
	var res *mutate.Result
	if a.Bool("dry-run") {
		res, err = dd.DryRun(tx)
	} else {
		res, err = dd.Apply(tx)
	}
	if err != nil {
		return nil, err
	}
	out := &Output{DryRun: res.DryRun, Changes: res.Changes, Problems: res.Problems}
	if !res.DryRun {
		out.Files, err = s.save(rel, src, dd.Bytes())
	}
	return out, err
}

func values(fs []aetext.Field) map[string]string {
	out := make(map[string]string, len(fs))
	for _, f := range fs {
		out[f.Name] = mutate.FormatValue(f.Value)
	}
	return out
}

func (s *Session) dataGet(a Args) (*Output, error) {
	_, _, dd, err := s.dataDoc(a)
	if err != nil {
		return nil, err
	}
	v := map[string]any{"type": dd.Type.FullName}
	if !dd.Table() {
		d := dd.Doc()
		if !d.Base.IsZero() {
			v["base"] = s.label(d.Base)
		}
		v["fields"] = values(d.Fields)
		return &Output{Value: v}, nil
	}
	if !a.Has("row") {
		return nil, errors.New("this is a table: pass --row; data rows lists them")
	}
	for _, r := range dd.TableDoc().Rows {
		if r.ID == a.Str("row") {
			v["row"], v["fields"] = r.ID, values(r.Fields)
			return &Output{Value: v}, nil
		}
	}
	return nil, errors.New("no row " + a.Str("row"))
}

func (s *Session) dataRows(a Args) (*Output, error) {
	_, _, dd, err := s.dataDoc(a)
	if err != nil {
		return nil, err
	}
	if !dd.Table() {
		return nil, errors.New("not a table")
	}
	var list []string
	for _, r := range dd.TableDoc().Rows {
		list = append(list, r.ID)
	}
	return &Output{Value: list}, nil
}

func tableCopy(t *data.TableDoc) *data.TableDoc {
	out := &data.TableDoc{Type: t.Type, Rows: make([]data.TableRow, len(t.Rows))}
	for i, r := range t.Rows {
		fs := make([]aetext.Field, len(r.Fields))
		for k, f := range r.Fields {
			fs[k] = aetext.Field{Name: f.Name, Value: f.Value.Clone()}
		}
		out.Rows[i] = data.TableRow{ID: r.ID, Fields: fs}
	}
	return out
}

func (s *Session) dataImport(a Args) (*Output, error) {
	_, _, dd, err := s.dataDoc(a)
	if err != nil {
		return nil, err
	}
	if !dd.Table() {
		return nil, errors.New("not a table")
	}
	csvRel, err := s.rel(a.Str("csv"))
	if err != nil {
		return nil, err
	}
	src, err := s.read(csvRel)
	if err != nil {
		return nil, err
	}
	next, err := data.ImportCSV(s.Types.Schema(), tableCopy(dd.TableDoc()), src)
	if err != nil {
		return nil, err
	}
	var ops []mutate.Op
	for _, r := range dd.TableDoc().Rows {
		ops = append(ops, mutate.RowRemove(r.ID))
	}
	for _, r := range next.Rows {
		ops = append(ops, mutate.RowRestore(r.ID, r.Fields))
	}
	return s.dataTx(a, ops...)
}

func (s *Session) dataExport(a Args) (*Output, error) {
	_, _, dd, err := s.dataDoc(a)
	if err != nil {
		return nil, err
	}
	if !dd.Table() {
		return nil, errors.New("not a table")
	}
	csvRel, err := s.rel(a.Str("csv"))
	if err != nil {
		return nil, err
	}
	b, err := data.ExportCSV(s.Types.Schema(), dd.TableDoc())
	if err != nil {
		return nil, err
	}
	if a.Bool("dry-run") {
		return &Output{DryRun: true, Notes: []string{"would write " + csvRel}}, nil
	}
	if err := project.WriteAtomic(s.Pipeline.Abs(csvRel), b); err != nil {
		return nil, err
	}
	return &Output{Files: []string{csvRel}}, nil
}
