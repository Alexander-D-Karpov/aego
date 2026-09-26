package sceneio

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/format/aetext"
)

type Migration func(*aetext.Node) error

var migrations = map[int]Migration{
	0: migrateV0ToV1,
}

func Migrate(doc *aetext.Node, from, to int) (*aetext.Node, error) {
	if from > to {
		return nil, errs.New(errs.SceneIOMigrate, "cannot migrate backwards",
			log.F("from", from), log.F("to", to))
	}
	for v := from; v < to; v++ {
		fn, ok := migrations[v]
		if !ok {
			return nil, errs.New(errs.SceneIOMigrate, "no migration for this version",
				log.F("from", v), log.F("to", v+1))
		}
		if err := fn(doc); err != nil {
			return nil, err
		}
	}
	doc.Attrs = setAttr(doc.Attrs, "version", aetext.Int(int64(to)))
	return doc, nil
}

func setAttr(attrs []aetext.Field, name string, v aetext.Value) []aetext.Field {
	for i := range attrs {
		if attrs[i].Name == name {
			attrs[i].Value = v
			return attrs
		}
	}
	return append(attrs, aetext.Field{Name: name, Value: v})
}

func migrateV0ToV1(n *aetext.Node) error {
	if n.Head == "transform" {
		if v, ok := n.Field("pos"); ok {
			n.RemoveField("pos")
			n.SetField("position", v)
		}
	}
	for _, b := range n.Blocks {
		if err := migrateV0ToV1(b); err != nil {
			return err
		}
	}
	return nil
}
