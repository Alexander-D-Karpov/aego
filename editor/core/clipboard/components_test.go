package clipboard

import (
	"testing"

	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
)

func TestComponentCopyPasteAddsAndUndoes(t *testing.T) {
	ctx, d := setup(t)
	s := d.Scene()
	a := s.Find(s.Root(), "A")
	b := s.Find(s.Root(), "A/B")
	td, ok := ctx.Types.Schema().ByName("test.Link")
	if !ok {
		t.Fatal("Link not registered")
	}
	if err := CopyComponent(ctx, s, s.ID(a), td.ID); err != nil {
		t.Fatal(err)
	}
	if Peek(ctx) != KindComponent {
		t.Fatalf("peek = %v", Peek(ctx))
	}
	ctx.Select(s.ID(b))
	if err := Paste(ctx); err != nil {
		t.Fatal(err)
	}
	link, ok := scene.Get[Link](s, b)
	if !ok {
		t.Fatal("component was not added")
	}
	if got, ok := link.Target.Resolve(s); !ok || got != b {
		t.Fatal("pasted values lost")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, ok := scene.Get[Link](s, b); ok {
		t.Fatal("paste must be one undo step")
	}
}

func TestReadValueFromText(t *testing.T) {
	ctx, _ := setup(t)
	ctx.Clipboard.SetText("1.5, -2")
	v, ok := ReadValue(ctx, &schema.FieldDescriptor{Kind: schema.KindVec2})
	if !ok || v.V != (mathx.Vec2{X: 1.5, Y: -2}) {
		t.Fatalf("vec2 = %+v %v", v, ok)
	}
	ctx.Clipboard.SetText("abc")
	if _, ok := ReadValue(ctx, &schema.FieldDescriptor{Kind: schema.KindFloat32}); ok {
		t.Fatal("text must not paste into a number")
	}
	ctx.Clipboard.SetText("#ff0000")
	if v, ok := ReadValue(ctx, &schema.FieldDescriptor{Kind: schema.KindColor}); !ok || v.C.R < 0.99 {
		t.Fatalf("color = %+v %v", v, ok)
	}
}

func TestPasteWithNothingUsefulIsNotAnError(t *testing.T) {
	ctx, _ := setup(t)
	ctx.Clipboard.SetText("")
	if Peek(ctx) != KindEmpty {
		t.Fatal("empty clipboard misdetected")
	}
	if err := Paste(ctx); err != nil {
		t.Fatalf("paste returned %v", err)
	}
	ctx.Clipboard.SetText("hello")
	if err := Paste(ctx); err != nil {
		t.Fatalf("plain text paste returned %v", err)
	}
}
