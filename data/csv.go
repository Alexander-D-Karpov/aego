package data

import (
	"bytes"
	"encoding/csv"
	"slices"
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/schema"
)

func csvField(fd *schema.FieldDescriptor) bool {
	if !fd.Flags.Has(schema.FlagSerialize) {
		return false
	}
	switch fd.Kind {
	case schema.KindBool, schema.KindInt32, schema.KindInt64, schema.KindFloat32, schema.KindFloat64,
		schema.KindString, schema.KindEnum, schema.KindAssetRef:
		return true
	}
	return false
}

func tableType(reg *schema.Registry, name string) (*schema.TypeDescriptor, error) {
	td, ok := reg.ByName(name)
	if !ok || td.Kind != schema.TypeAsset {
		return nil, errs.New(errs.SchemaUnknownType, "unknown data type "+name)
	}
	return td, nil
}

func cellText(v aetext.Value) string {
	switch v.Kind {
	case aetext.VInt:
		return strconv.FormatInt(v.Int, 10)
	case aetext.VFloat:
		bits := 64
		if v.Single {
			bits = 32
		}
		return strconv.FormatFloat(v.Float, 'g', -1, bits)
	case aetext.VBool:
		return strconv.FormatBool(v.Bool)
	case aetext.VString, aetext.VAsset:
		return v.Str
	case aetext.VIdent:
		return v.Ident
	}
	return ""
}

func ExportCSV(reg *schema.Registry, t *TableDoc) ([]byte, error) {
	td, err := tableType(reg, t.Type)
	if err != nil {
		return nil, err
	}
	header := []string{"id"}
	for i := range td.Fields {
		if fd := &td.Fields[i]; csvField(fd) {
			header = append(header, fd.Name)
		}
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if err := w.Write(header); err != nil {
		return nil, err
	}
	rec := make([]string, len(header))
	for _, r := range t.Rows {
		clear(rec)
		rec[0] = r.ID
		for _, f := range r.Fields {
			if j := slices.Index(header, f.Name); j > 0 {
				rec[j] = cellText(f.Value)
			}
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

func parseCell(fd *schema.FieldDescriptor, s string) (aetext.Value, error) {
	switch fd.Kind {
	case schema.KindBool:
		v, err := strconv.ParseBool(s)
		return aetext.Bool(v), err
	case schema.KindInt32, schema.KindInt64:
		v, err := strconv.ParseInt(s, 10, 64)
		return aetext.Int(v), err
	case schema.KindFloat32:
		v, err := strconv.ParseFloat(s, 32)
		return aetext.Float32(float32(v)), err
	case schema.KindFloat64:
		v, err := strconv.ParseFloat(s, 64)
		return aetext.Float64(v), err
	case schema.KindString:
		return aetext.String(s), nil
	case schema.KindEnum:
		if _, ok := fd.EnumValueOf(s); ok {
			return aetext.Ident(s), nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		return aetext.Int(v), err
	case schema.KindAssetRef:
		id, err := ids.ParseAssetID(s)
		return aetext.AssetRef(id.String()), err
	}
	return aetext.Value{}, errs.New(errs.SchemaBadField, "field "+fd.Name+" cannot be a CSV column")
}

func order(td *schema.TypeDescriptor, fs []aetext.Field) {
	pos := func(name string) int {
		for i := range td.Fields {
			if td.Fields[i].Name == name {
				return i
			}
		}
		return len(td.Fields)
	}
	slices.SortStableFunc(fs, func(a, b aetext.Field) int { return pos(a.Name) - pos(b.Name) })
}

func ImportCSV(reg *schema.Registry, into *TableDoc, src []byte) (*TableDoc, error) {
	td, err := tableType(reg, into.Type)
	if err != nil {
		return nil, err
	}
	records, err := csv.NewReader(bytes.NewReader(src)).ReadAll()
	if err != nil {
		return nil, errs.Wrap(err, errs.AssetImportFailed, "malformed CSV")
	}
	if len(records) == 0 || strings.TrimSpace(records[0][0]) != "id" {
		return nil, errs.New(errs.AssetImportFailed, "the first CSV column must be id")
	}
	header := records[0]
	cols := make([]*schema.FieldDescriptor, len(header))
	for j := 1; j < len(header); j++ {
		fd, ok := td.Field(strings.TrimSpace(header[j]))
		if !ok || !csvField(fd) {
			return nil, errs.New(errs.AssetImportFailed, "column "+header[j]+" is not a CSV field of "+td.FullName)
		}
		cols[j] = fd
	}
	inCols := func(name string) bool {
		for _, fd := range cols[1:] {
			if fd.Name == name {
				return true
			}
		}
		return false
	}
	old := map[string]*TableRow{}
	for i := range into.Rows {
		old[into.Rows[i].ID] = &into.Rows[i]
	}
	out := &TableDoc{Type: into.Type}
	seen := map[string]bool{}
	for line, rec := range records[1:] {
		id := strings.TrimSpace(rec[0])
		if id == "" || seen[id] {
			return nil, errs.New(errs.AssetImportFailed, "CSV line "+strconv.Itoa(line+2)+" has an empty or repeated id", log.F("id", id))
		}
		seen[id] = true
		row := TableRow{ID: id}
		if prev := old[id]; prev != nil {
			for _, f := range prev.Fields {
				if !inCols(f.Name) {
					row.Fields = append(row.Fields, f)
				}
			}
		}
		for j := 1; j < len(rec); j++ {
			if rec[j] == "" {
				continue
			}
			v, err := parseCell(cols[j], rec[j])
			if err != nil {
				return nil, errs.Wrap(err, errs.AssetImportFailed, "row "+id+", column "+cols[j].Name)
			}
			row.Fields = append(row.Fields, aetext.Field{Name: cols[j].Name, Value: v})
		}
		order(td, row.Fields)
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}
