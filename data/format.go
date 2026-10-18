package data

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
)

const FormatVersion = 1

type Doc struct {
	Type   string
	Base   ids.AssetID
	Fields []aetext.Field
}

type TableRow struct {
	ID     string
	Fields []aetext.Field
}

type TableDoc struct {
	Type string
	Rows []TableRow
}

func parseHead(src []byte, name, head string) (*aetext.Node, string, error) {
	n, err := aetext.Parse(src, name)
	if err != nil {
		return nil, "", err
	}
	if n.Head != head {
		return nil, "", errs.New(errs.AssetBadArtifact, "not a "+head+" document", log.F("path", name), log.F("head", n.Head))
	}
	if v, ok := n.Attr("version"); ok && int(v.AsInt()) > FormatVersion {
		return nil, "", errs.New(errs.AssetBadArtifact, head+" document was written by a newer engine", log.F("path", name))
	}
	v, ok := n.Attr("type")
	if !ok || v.Str == "" {
		return nil, "", errs.New(errs.AssetBadArtifact, head+" document has no type", log.F("path", name))
	}
	return n, v.Str, nil
}

func ParseDoc(src []byte, name string) (*Doc, error) {
	n, typ, err := parseHead(src, name, "data")
	if err != nil {
		return nil, err
	}
	d := &Doc{Type: typ, Fields: n.Fields}
	if v, ok := n.Attr("base"); ok && v.Str != "" {
		id, err := ids.ParseAssetID(v.Str)
		if err != nil {
			return nil, errs.Wrap(err, errs.AssetBadArtifact, "malformed base reference").With(log.F("path", name))
		}
		d.Base = id
	}
	return d, nil
}

func header(head, typ string) *aetext.Node {
	n := &aetext.Node{Head: head}
	n.Attrs = append(n.Attrs,
		aetext.Field{Name: "version", Value: aetext.Int(FormatVersion)},
		aetext.Field{Name: "type", Value: aetext.String(typ)},
	)
	return n
}

func WriteDoc(d *Doc) []byte {
	n := header("data", d.Type)
	if !d.Base.IsZero() {
		n.Attrs = append(n.Attrs, aetext.Field{Name: "base", Value: aetext.AssetRef(d.Base.String())})
	}
	n.Fields = d.Fields
	return aetext.Write(n)
}

func ParseTable(src []byte, name string) (*TableDoc, error) {
	n, typ, err := parseHead(src, name, "table")
	if err != nil {
		return nil, err
	}
	t := &TableDoc{Type: typ}
	n.EachBlock("row", func(b *aetext.Node) {
		t.Rows = append(t.Rows, TableRow{ID: b.Label, Fields: b.Fields})
	})
	return t, nil
}

func WriteTable(t *TableDoc) []byte {
	n := header("table", t.Type)
	for _, r := range t.Rows {
		n.Blocks = append(n.Blocks, &aetext.Node{Head: "row", Label: r.ID, Fields: r.Fields})
	}
	return aetext.Write(n)
}
