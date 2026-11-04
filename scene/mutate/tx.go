package mutate

import (
	"strconv"

	"aego/core/errs"
	"aego/core/log"
	"aego/format/aetext"
)

const (
	TxHead    = "tx"
	TxVersion = 1
	opHead    = "op"
)

type Tx struct {
	Label string
	Ops   []Op
}

func (tx Tx) Empty() bool {
	return len(tx.Ops) == 0
}

func (o Op) encode() (*aetext.Node, error) {
	n := &aetext.Node{Head: opHead, Label: o.Kind}
	str := func(name, v string) {
		if v != "" {
			n.Fields = append(n.Fields, aetext.Field{Name: name, Value: aetext.String(v)})
		}
	}
	flag := func(name string, on bool) {
		if on {
			n.Fields = append(n.Fields, aetext.Field{Name: name, Value: aetext.Int(1)})
		}
	}
	str("node", o.Node)
	str("parent", o.Parent)
	str("name", o.Name)
	str("id", o.ID)
	str("space", o.Space)
	str("comp", o.Comp)
	str("path", o.Path)
	str("prefab", o.Prefab)
	str("level", o.Level)
	str("key", o.Key)
	str("owner", o.Owner)
	str("field", o.Field)
	str("asset", o.Asset)
	if o.Kind == KindSet || o.Kind == KindDataSet || o.Kind == KindMaterialSet || o.hasValue() {
		v, err := o.value()
		if err != nil {
			return nil, err
		}
		n.Fields = append(n.Fields, aetext.Field{Name: "value", Value: v})
	}
	if o.HasIndex {
		n.Fields = append(n.Fields, aetext.Field{Name: "index", Value: aetext.Int(int64(o.Index))})
	}
	flag("raw", o.Raw)
	flag("live", o.Live)
	flag("keepWorld", o.KeepWorld)
	flag("deep", o.Deep)
	str("fragment", o.Fragment)
	if o.Values != nil {
		n.Blocks = append(n.Blocks, o.Values)
	}
	return n, nil
}

func (tx Tx) Encode() ([]byte, error) {
	root := &aetext.Node{Head: TxHead, Attrs: []aetext.Field{{Name: "version", Value: aetext.Int(TxVersion)}}}
	if tx.Label != "" {
		root.Fields = append(root.Fields, aetext.Field{Name: "label", Value: aetext.String(tx.Label)})
	}
	for _, o := range tx.Ops {
		n, err := o.encode()
		if err != nil {
			return nil, err
		}
		root.Blocks = append(root.Blocks, n)
	}
	return aetext.Write(root), nil
}

func ParseTx(src []byte, name string) (Tx, error) {
	doc, err := aetext.Parse(src, name)
	if err != nil {
		return Tx{}, err
	}
	if doc.Head != TxHead {
		return Tx{}, errs.New(errs.SceneIOParse, "not a transaction file", log.F("file", name), log.F("head", doc.Head))
	}
	if v, ok := doc.Attr("version"); ok && int(v.AsInt()) > TxVersion {
		return Tx{}, errs.New(errs.SceneIOParse, "the transaction was written by a newer engine", log.F("file", name))
	}
	tx := Tx{}
	if v, ok := doc.Field("label"); ok {
		tx.Label = v.Str
	}
	var bad error
	i := 0
	doc.EachBlock(opHead, func(n *aetext.Node) {
		i++
		if bad != nil {
			return
		}
		if !knownKind(n.Label) {
			bad = errs.New(errs.SceneIOParse, "unknown operation "+strconv.Quote(n.Label), log.F("file", name), log.F("op", i))
			return
		}
		o := Op{Kind: n.Label}
		get := func(f string) string {
			if v, ok := n.Field(f); ok {
				return v.Str
			}
			return ""
		}
		on := func(f string) bool {
			v, ok := n.Field(f)
			return ok && v.AsInt() == 1
		}
		o.Node, o.Parent, o.Name, o.ID = get("node"), get("parent"), get("name"), get("id")
		o.Space, o.Comp, o.Path, o.Fragment = get("space"), get("comp"), get("path"), get("fragment")
		o.Prefab, o.Level, o.Key, o.Owner, o.Field = get("prefab"), get("level"), get("key"), get("owner"), get("field")
		o.Asset = get("asset")
		if v, ok := n.Field("value"); ok {
			o.Value, o.HasValue = v, true
		}
		if v, ok := n.Field("index"); ok {
			o.Index, o.HasIndex = int(v.AsInt()), true
		}
		o.Raw, o.Live, o.KeepWorld, o.Deep = on("raw"), on("live"), on("keepWorld"), on("deep")
		o.Values = n.Block("values", "")
		if (o.Kind == KindSet || o.Kind == KindDataSet || o.Kind == KindMaterialSet || o.Kind == KindMaterialVariants) && !o.HasValue {
			bad = errs.New(errs.SceneIOParse, o.Kind+" needs a value", log.F("file", name), log.F("op", i))
			return
		}
		tx.Ops = append(tx.Ops, o)
	})
	return tx, bad
}
