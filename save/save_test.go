package save

import (
	"testing"
	"time"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/format/aetext"
)

func TestPersistentID(t *testing.T) {
	id := PersistentID(42)
	if !IsPersistent(id) || Counter(id) != 42 {
		t.Fatalf("id %v", id)
	}
	if IsPersistent(ids.NodeID{Hi: 1, Lo: 42}) || Counter(ids.NodeID{Hi: 1, Lo: 42}) != 0 {
		t.Fatal("scene ids are not persistent")
	}
}

func TestDocRoundTrip(t *testing.T) {
	scene := ids.AssetID{Hi: 7, Lo: 9}
	node := &aetext.Node{Head: "node", Label: ids.NodeID{Hi: 3, Lo: 4}.String()}
	w := NewWriter(node, nil)
	w.Int("coins", 12)
	w.Vec2("pos", mathx.Vec2{X: 1.5, Y: -2})
	w.String("name", "hero")
	d := &Doc{
		Meta:      Meta{Name: "slot", Game: "g", Engine: "e", Scene: scene, Date: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Playtime: 12.5, Objects: 3, Counter: 17},
		Nodes:     []*aetext.Node{node},
		Spawned:   []*aetext.Node{{Head: "spawned", Label: PersistentID(17).String()}},
		Destroyed: []ids.NodeID{{Hi: 5, Lo: 6}},
		Global:    []aetext.Field{{Name: "level", Value: aetext.Int(2)}},
		Sections:  []Section{{ID: "mod.state", Version: 3, Node: &aetext.Node{Fields: []aetext.Field{{Name: "x", Value: aetext.Int(1)}}}}},
	}
	got, err := Decode(Encode(d), "t.aesave")
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta != d.Meta {
		t.Fatalf("meta %+v", got.Meta)
	}
	if len(got.Nodes) != 1 || len(got.Spawned) != 1 || len(got.Destroyed) != 1 || got.Destroyed[0] != d.Destroyed[0] {
		t.Fatalf("doc %+v", got)
	}
	r := NewReader(got.Nodes[0], nil)
	if r.Int("coins", 0) != 12 || r.Vec2("pos", mathx.Vec2{}) != (mathx.Vec2{X: 1.5, Y: -2}) || r.String("name", "") != "hero" {
		t.Fatal("node fields lost")
	}
	if len(got.Global) != 1 || got.Global[0].Value.AsInt() != 2 {
		t.Fatalf("global %+v", got.Global)
	}
	s, ok := got.Section("mod.state")
	if !ok || s.Version != 3 || NewReader(s.Node, nil).Int("x", 0) != 1 {
		t.Fatalf("section %+v", s)
	}
}

func TestDecodeRejectsNewer(t *testing.T) {
	if _, err := Decode([]byte("save version=99 { meta { } }"), "t"); err == nil {
		t.Fatal("newer save accepted")
	}
	if _, err := Decode([]byte("scene version=1 { }"), "t"); err == nil {
		t.Fatal("non-save accepted")
	}
}

func TestStore(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Path("../x"); err == nil {
		t.Fatal("bad slot accepted")
	}
	d := &Doc{Meta: Meta{Name: "one"}}
	if err := s.Write("slot_1", Encode(d), []byte("png")); err != nil {
		t.Fatal(err)
	}
	if !s.Exists("slot_1") {
		t.Fatal("slot missing")
	}
	got, err := s.Load("slot_1")
	if err != nil || got.Meta.Name != "one" {
		t.Fatalf("load %v %+v", err, got)
	}
	list := s.List()
	if len(list) != 1 || list[0].Slot != "slot_1" || list[0].Preview == "" || list[0].Err != nil {
		t.Fatalf("list %+v", list)
	}
	if err := s.Delete("slot_1"); err != nil || s.Exists("slot_1") || len(s.List()) != 0 {
		t.Fatalf("delete %v", err)
	}
}
