package scriptcomp

import (
	"testing"

	"aego/core/curve"
	"aego/format/aetext"
	"aego/scene"
	"aego/schema"
)

// An editor can change exported fields, including unknown fields retained
// for another module, after the timeline has already captured them.
func TestScriptKeyframeOwnsNestedPropertiesAndUnknownFields(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := Register(types); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	node := s.Create(s.Root(), "Actor")
	c := scene.Add[ScriptComponent](s, node)
	c.Attachments = []Attachment{{
		TypeName: "Actor.Script", Enabled: true,
		Unknown: []aetext.Field{{Name: "future", Value: aetext.List(aetext.List(aetext.Int(11)))}},
	}}
	object := &schema.Object{Fields: []schema.Value{schema.Curve(curve.Constant(3))}}
	c.Attachments[0].Props.Set("motion", schema.List(schema.Value{Kind: schema.KindStruct, Obj: object}))
	s.Flush()
	c, _ = scene.Get[ScriptComponent](s, node)
	key := s.Capture(nil)
	for i := 0; i < 2; i++ {
		a := &c.Attachments[0]
		a.Enabled = false
		a.Unknown[0].Value.List[0].List[0].Int = 42
		a.Props.Props[0].Value.List[0].Obj.Fields[0].Cv.Keys[0].V = 42
		s.Restore(key)
		c, _ = scene.Get[ScriptComponent](s, node)
		a = &c.Attachments[0]
		if !a.Enabled || a.Unknown[0].Value.List[0].List[0].Int != 11 || a.Props.Props[0].Value.List[0].Obj.Fields[0].Cv.Keys[0].V != 3 {
			t.Fatalf("restore %d changed the saved attachment", i+1)
		}
	}
	c.Attachments = nil
	key = s.Capture(key)
	c.Attachments = []Attachment{{TypeName: "Later"}}
	s.Restore(key)
	c, _ = scene.Get[ScriptComponent](s, node)
	if c.Attachments != nil {
		t.Fatal("reusing a snapshot retained a removed attachment")
	}
}
