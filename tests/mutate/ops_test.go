package mutate_test

import (
	"bytes"
	"testing"

	"aego/core/ids"
	"aego/data"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
)

func TestRedoRecreatesTheSameScene(t *testing.T) {
	e := env(t)
	for _, c := range roundTrips {
		d, before := load(t, e)
		res, err := d.Apply(c.tx)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		after := d.Bytes()
		if _, err := d.Apply(res.Inverse); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !bytes.Equal(d.Bytes(), before) {
			t.Fatalf("%s: undo", c.name)
		}
		if _, err := d.Apply(res.Redo); err != nil {
			t.Fatalf("%s: redo: %v", c.name, err)
		}
		if !bytes.Equal(d.Bytes(), after) {
			t.Fatalf("%s: redo differs from the first apply", c.name)
		}
	}
}

func TestMoveKeepWorldAndInsert(t *testing.T) {
	e := env(t)
	d, before := load(t, e)
	h := d.Scene.Find(d.Scene.Root(), "PlatformLow")
	frag := string(sceneio.SaveSubtree(d.Scene, h, false))
	res, err := d.Apply(tx(
		mutate.Move("PlatformLow", "Player").KeepingWorld().At(0),
		mutate.RemoveNode("Player/PlatformLow"),
		mutate.Insert("Root", frag).At(1),
	))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(d.Bytes(), before) {
		t.Fatal("nothing changed")
	}
	if _, err := d.Apply(res.Inverse); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Bytes(), before) {
		t.Fatalf("inverse:\n%s", d.Bytes())
	}
}

func firstData(t *testing.T, e mutate.Env) (*schema.TypeDescriptor, *schema.FieldDescriptor) {
	t.Helper()
	for _, td := range e.Types.Schema().DataTypes() {
		for i := range td.Fields {
			fd := &td.Fields[i]
			if fd.Flags.Has(schema.FlagSerialize) && !fd.Flags.Has(schema.FlagReadOnly) && (!fd.HasMax || fd.Max >= 5) &&
				(fd.Kind == schema.KindFloat32 || fd.Kind == schema.KindInt32 || fd.Kind == schema.KindInt64) {
				return td, fd
			}
		}
	}
	t.Skip("no data type with a numeric field is registered")
	return nil, nil
}

func TestDataDocuments(t *testing.T) {
	e := env(t)
	td, fd := firstData(t, e)
	doc, err := mutate.LoadData(data.WriteDoc(&data.Doc{Type: td.FullName}), "a.aedata", e)
	if err != nil {
		t.Fatal(err)
	}
	before := doc.Bytes()
	res, err := doc.Apply(tx(mutate.DataSet("", fd.Name, "3")))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(doc.Bytes(), []byte(fd.Name+"=3")) {
		t.Fatalf("no override:\n%s", doc.Bytes())
	}
	if _, err := doc.Apply(res.Inverse); err != nil || !bytes.Equal(doc.Bytes(), before) {
		t.Fatalf("data inverse: %v\n%s", err, doc.Bytes())
	}
	tab, err := mutate.LoadData(data.WriteTable(&data.TableDoc{Type: td.FullName}), "t.aetable", e)
	if err != nil {
		t.Fatal(err)
	}
	empty := tab.Bytes()
	res, err = tab.Apply(tx(
		mutate.RowAdd("a"),
		mutate.RowAdd("b"),
		mutate.DataSet("a", fd.Name, "5"),
		mutate.RowRename("a", "c"),
		mutate.RowRemove("b"),
	))
	if err != nil {
		t.Fatal(err)
	}
	full := tab.Bytes()
	if _, err := tab.Apply(res.Inverse); err != nil || !bytes.Equal(tab.Bytes(), empty) {
		t.Fatalf("table inverse: %v\n%s", err, tab.Bytes())
	}
	if _, err := tab.Apply(res.Redo); err != nil || !bytes.Equal(tab.Bytes(), full) {
		t.Fatalf("table redo: %v", err)
	}
	if _, err := tab.Apply(tx(mutate.RowAdd("c"))); err == nil {
		t.Fatal("a duplicate row id was accepted")
	}
}

func TestDataBase(t *testing.T) {
	e := env(t)
	td, _ := firstData(t, e)
	base := ids.AssetID{Hi: 0xda7a, Lo: 1}
	e.Assets = func(id ids.AssetID) bool { return id == base }
	doc, err := mutate.LoadData(data.WriteDoc(&data.Doc{Type: td.FullName}), "a.aedata", e)
	if err != nil {
		t.Fatal(err)
	}
	before := doc.Bytes()
	res, err := doc.Apply(tx(mutate.DataBase(base.String())))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Doc().Base != base {
		t.Fatalf("base %s", doc.Doc().Base)
	}
	after := doc.Bytes()
	if _, err := doc.Apply(res.Inverse); err != nil || !bytes.Equal(doc.Bytes(), before) {
		t.Fatalf("inverse: %v\n%s", err, doc.Bytes())
	}
	if _, err := doc.Apply(res.Redo); err != nil || !bytes.Equal(doc.Bytes(), after) {
		t.Fatalf("redo: %v", err)
	}
	if _, err := doc.Apply(tx(mutate.DataBase(ids.AssetID{Hi: 9, Lo: 9}.String()))); err == nil {
		t.Fatal("a base outside the project was accepted")
	}
	tab, err := mutate.LoadData(data.WriteTable(&data.TableDoc{Type: td.FullName}), "t.aetable", e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tab.Apply(tx(mutate.DataBase(base.String()))); err == nil {
		t.Fatal("a table accepted a base")
	}
	src, err := tx(mutate.DataBase(base.String())).Encode()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mutate.ParseTx(src, "base.aetext")
	if err != nil || len(parsed.Ops) != 1 || parsed.Ops[0].Asset != base.String() {
		t.Fatalf("parsed %+v: %v\n%s", parsed, err, src)
	}
}
