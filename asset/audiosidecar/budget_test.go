package audiosidecar

import (
	"strings"
	"testing"

	"aego/format/aetext"
)

func TestWriteSizeMatchesAetextAndRejectsBeforeAllocation(t *testing.T) {
	for _, node := range []*aetext.Node{
		{Head: "empty"},
		{Head: "meta", Label: "кириллица \" \\", Attrs: []aetext.Field{
			{Name: "v", Value: aetext.Int(-42)}, {Name: "colorValue", Value: aetext.Color(0x123)},
		}, Fields: []aetext.Field{
			{Name: "ints", Value: aetext.List(aetext.Int(-9), aetext.Float32(.25), aetext.Float64(1e99))},
			{Name: "str", Value: aetext.String("a\x00b\rc\nd\te\\\"ю")},
			{Name: "refs", Value: aetext.List(aetext.AssetRef("a\"b"), aetext.NodeRef("c\\d"))},
			{Name: "object", Value: aetext.Struct(aetext.Field{Name: "x", Value: aetext.Bool(true)}, aetext.Field{Name: "y", Value: aetext.Struct()})},
			{Name: "fallback", Value: aetext.Value{}},
		}, Blocks: []*aetext.Node{{Head: "child", Fields: []aetext.Field{{Name: "v", Value: aetext.Ident("abc")}}}}},
	} {
		want := int64(len(aetext.Write(node)))
		got, err := writeSize(node, want)
		if err != nil || got != want {
			t.Fatalf("serialized size = %d, expected %d: %v", got, want, err)
		}
		if _, err := writeSize(node, want-1); err == nil {
			t.Fatal("accepted serialized output exceeding its budget")
		}
	}
	// A small input structure can expand significantly through indentation.
	root := &aetext.Node{Head: "root"}
	at := root
	for range 40 {
		next := &aetext.Node{Head: "nested"}
		at.Blocks = append(at.Blocks, next)
		at = next
	}
	at.Fields = append(at.Fields, aetext.Field{Name: "text", Value: aetext.String(strings.Repeat("a", 50))})
	if _, err := writeSize(root, 512); err == nil {
		t.Fatal("did not count indentation toward the output budget")
	}
}
