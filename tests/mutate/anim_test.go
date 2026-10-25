package mutate_test

import (
	"bytes"
	"strings"
	"testing"

	"aego/format/aetext"
	"aego/scene/mutate"
)

const clipSrc = `anim version=1 length=1 loop=loop {
  frames "" fps=10 {
    sprites=[asset("ae900000000000000000000000000b00"), asset("ae900000000000000000000000000b01")]
  }
  track "" prop="SpriteRenderer.tint" kind=color {
    key t=0 v=[1, 1, 1, 1] interp=linear {}
    key t=0.5 v=[1, 0, 0, 1] {}
  }
  event t=0.25 name="Hit" payload=1 {}
}
`

const ctrlSrc = `controller version=1 {
  entry="Idle"
  param "Speed" kind=float {}
  state "Idle" {}
  state "Run" {}
  transition "Idle" to="Run" duration=0.1 {
    cond "Speed" op=gt value=0.1 {}
  }
  transition "Run" to="Idle" duration=0.1 {
    cond "Speed" op=le value=0.1 {}
  }
}
`

type textDoc interface {
	Apply(mutate.Tx) (*mutate.Result, error)
	Bytes() []byte
}

func atx(ops ...mutate.Op) mutate.Tx {
	return mutate.Tx{Label: "Test", Ops: ops}
}

func val(t *testing.T, s string) aetext.Value {
	t.Helper()
	v, err := mutate.ParseValue(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func clipDoc(t *testing.T) textDoc {
	t.Helper()
	d, err := mutate.LoadAnim([]byte(clipSrc), "c.aeanim")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func ctrlDoc(t *testing.T) textDoc {
	t.Helper()
	d, err := mutate.LoadCtrl([]byte(ctrlSrc), "c.aectrl")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func checkRoundTrip(t *testing.T, name string, load func(*testing.T) textDoc, tx mutate.Tx) {
	t.Helper()
	d := load(t)
	before := d.Bytes()
	res, err := d.Apply(tx)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	for _, p := range res.Problems {
		if p.Error {
			t.Fatalf("%s: %s", name, p.Message)
		}
	}
	after := d.Bytes()
	if bytes.Equal(after, before) {
		t.Fatalf("%s changed nothing", name)
	}
	if _, err := d.Apply(res.Inverse); err != nil || !bytes.Equal(d.Bytes(), before) {
		t.Fatalf("%s: undo %v\n%s", name, err, d.Bytes())
	}
	if _, err := d.Apply(res.Redo); err != nil || !bytes.Equal(d.Bytes(), after) {
		t.Fatalf("%s: redo %v\n%s", name, err, d.Bytes())
	}
	src, err := res.Redo.Encode()
	if err != nil {
		t.Fatalf("%s: encode %v", name, err)
	}
	parsed, err := mutate.ParseTx(src, name+".aetext")
	if err != nil {
		t.Fatalf("%s: parse %v\n%s", name, err, src)
	}
	fresh := load(t)
	if _, err := fresh.Apply(parsed); err != nil || !bytes.Equal(fresh.Bytes(), after) {
		t.Fatalf("%s: the encoded redo gives another file: %v\n%s", name, err, fresh.Bytes())
	}
}

func TestAnimOpsRoundTrip(t *testing.T) {
	step := mutate.AnimKeySet("", "SpriteRenderer.tint", "0.75", val(t, "[0, 0, 1, 1]"))
	step.Values = &aetext.Node{Head: "values", Attrs: []aetext.Field{{Name: "interp", Value: aetext.Ident("step")}}}
	scale := mutate.AnimKeySet("", "transform.scale", "0.2", val(t, "[1.5, 1.5]"))
	scale.Comp = "vec2"
	ev := mutate.AnimEventAdd("0.6", "Footstep")
	ev.Level = "presentation"
	for _, c := range []struct {
		name string
		tx   mutate.Tx
	}{
		{"set existing key", atx(mutate.AnimKeySet("", "SpriteRenderer.tint", "0.5", val(t, "[0, 1, 0, 1]")))},
		{"add key with interp", atx(step)},
		{"add track", atx(scale)},
		{"remove key", atx(mutate.AnimKeyRemove("", "SpriteRenderer.tint", "0"))},
		{"remove last key drops the track", atx(scale, mutate.AnimKeyRemove("", "transform.scale", "0.2"), mutate.AnimKeyRemove("", "SpriteRenderer.tint", "0"))},
		{"move key", atx(mutate.AnimKeyMove("", "SpriteRenderer.tint", "0.5", "0.8"))},
		{"remove track", atx(mutate.AnimTrackRemove("", "SpriteRenderer.tint"))},
		{"add event", atx(ev)},
		{"remove event", atx(mutate.AnimEventRemove(0))},
		{"move event", atx(mutate.AnimEventMove(0, "0.6"))},
		{"clip attrs", atx(mutate.AnimClipSet("length", val(t, "2")), mutate.AnimClipSet("loop", val(t, "once")), mutate.AnimClipSet("fps", val(t, "24")))},
		{"clear loop", atx(mutate.AnimClipClear("loop"))},
		{"expand frames", atx(mutate.AnimFramesExpand(""))},
	} {
		checkRoundTrip(t, c.name, clipDoc, c.tx)
	}
}

func TestAnimKeepsFramesAndRefusesFrameKeys(t *testing.T) {
	d, _ := mutate.LoadAnim([]byte(clipSrc), "c.aeanim")
	frames := string(aetext.Write(d.Root().Block("frames", "")))
	if _, err := d.Apply(atx(mutate.AnimKeySet("", "SpriteRenderer.tint", "0.3", val(t, "[1, 1, 0, 1]")))); err != nil {
		t.Fatal(err)
	}
	if got := string(aetext.Write(d.Root().Block("frames", ""))); got != frames {
		t.Fatalf("an edit of another track rewrote the frames block:\n%s", got)
	}
	before := d.Bytes()
	_, err := d.Apply(atx(mutate.AnimKeySet("", "SpriteRenderer.sprite", "0", val(t, `asset("ae900000000000000000000000000b02")`))))
	if err == nil || !strings.Contains(err.Error(), "frames") {
		t.Fatalf("a key on a frames track: %v", err)
	}
	if _, err := d.Apply(atx(mutate.AnimKeyMove("", "SpriteRenderer.tint", "0.3", "0.5"))); err == nil {
		t.Fatal("a key moved onto another key")
	}
	if _, err := d.Apply(atx(mutate.AnimClipClear("length"))); err == nil {
		t.Fatal("the length was cleared")
	}
	if !bytes.Equal(d.Bytes(), before) {
		t.Fatalf("failed operations left changes:\n%s", d.Bytes())
	}
	res, err := d.Apply(atx(mutate.AnimKeySet("", "SpriteRenderer.tint", "5", val(t, "[1, 1, 1, 1]"))))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 || !res.Problems[0].Error || !strings.Contains(res.Problems[0].Message, "ANM002") {
		t.Fatalf("a key past the end: %+v", res.Problems)
	}
}

func TestCtrlOpsRoundTrip(t *testing.T) {
	jump := &aetext.Node{Attrs: []aetext.Field{{Name: "pos", Value: val(t, "[440, 0]")}}}
	toJump := &aetext.Node{
		Attrs:  []aetext.Field{{Name: "to", Value: aetext.String("Jump")}, {Name: "interrupt", Value: aetext.Bool(true)}},
		Blocks: []*aetext.Node{{Head: "cond", Label: "Grounded", Attrs: []aetext.Field{{Name: "op", Value: aetext.Ident("eq")}, {Name: "value", Value: aetext.Bool(false)}}}},
	}
	grounded := &aetext.Node{Attrs: []aetext.Field{{Name: "kind", Value: aetext.Ident("bool")}, {Name: "default", Value: aetext.Bool(true)}}}
	for _, c := range []struct {
		name string
		tx   mutate.Tx
	}{
		{"add state", atx(mutate.CtrlPut("state", "Jump", jump))},
		{"move a state", atx(mutate.CtrlAttr("state", "Idle", "pos", val(t, "[10, 20]")))},
		{"add param and transition", atx(mutate.CtrlPut("param", "Grounded", grounded), mutate.CtrlPut("state", "Jump", jump), mutate.CtrlPut("transition", "any", toJump))},
		{"transition duration", atx(mutate.CtrlAttr("transition", "", "duration", val(t, "0.2")).At(0))},
		{"remove transition", atx(mutate.CtrlRemove("transition", "").At(1))},
		{"entry", atx(mutate.CtrlRoot("entry", aetext.String("Run")))},
		{"remove a state with its transitions", atx(mutate.CtrlRemove("transition", "").At(1), mutate.CtrlRemove("transition", "").At(0), mutate.CtrlRemove("state", "Run"))},
	} {
		checkRoundTrip(t, c.name, ctrlDoc, c.tx)
	}
}

func TestCtrlProblemsDoNotRollBack(t *testing.T) {
	d, _ := mutate.LoadCtrl([]byte(ctrlSrc), "c.aectrl")
	res, err := d.Apply(atx(mutate.CtrlRemove("state", "Run")))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 || !strings.Contains(res.Problems[0].Message, "ANC002") {
		t.Fatalf("a state still used by transitions: %+v", res.Problems)
	}
	if _, err := d.Apply(atx(mutate.CtrlAttr("state", "Nope", "speed", val(t, "2")))); err == nil {
		t.Fatal("an unknown state was edited")
	}
}

func TestCtrlMoveSwapsTransitions(t *testing.T) {
	src := "controller version=1 {\n  entry=\"A\"\n  state \"A\" {}\n  state \"B\" {}\n  transition \"A\" to=\"B\" {}\n  transition \"B\" to=\"A\" {}\n}\n"
	d, err := mutate.LoadCtrl([]byte(src), "t.aectrl")
	if err != nil {
		t.Fatal(err)
	}
	res, err := d.Apply(mutate.Tx{Ops: []mutate.Op{mutate.CtrlMove(1, -1)}})
	if err != nil {
		t.Fatal(err)
	}
	g, _ := d.Graph()
	if g.Transitions[0].From != 1 || g.Transitions[1].From != 0 {
		t.Fatalf("transitions not swapped: %+v", g.Transitions)
	}
	if _, err := d.Apply(res.Inverse); err != nil {
		t.Fatal(err)
	}
	if string(d.Bytes()) != src {
		t.Fatalf("the inverse must restore the bytes:\n%s", d.Bytes())
	}
}
