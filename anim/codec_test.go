package anim

import (
	"math"
	"reflect"
	"testing"

	"aego/core/curve"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/schema"
)

func sampleClip() *Clip {
	h := float32(math.Sqrt2 / 2)
	c := &Clip{Length: 1.5, FPS: 12, Loop: LoopPingPong,
		Tracks: []Track{
			{Target: "Body/Arm", Property: "transform.rotation", Kind: schema.KindFloat32,
				Keys: []Key{{T: 0, V: Float(0)}, {T: 0.5, V: Float(1.25), Interp: curve.InterpLinear}, {T: 1.5, V: Float(-1)}}},
			{Property: "SpriteRenderer.tint", Kind: schema.KindColor, Authority: AuthSimulation, OnEnd: EndRestore,
				Keys: []Key{{T: 0, V: Lanes([4]float32{1, 1, 1, 1})}, {T: 1, V: Lanes([4]float32{1, 0, 0, 0.5}), Tangent: curve.TangentFree,
					In: [4]float32{0.125, 0.25, 0.375, 0.5}, Out: [4]float32{0.5, 0.625, 0.75, 0.875}}}},
			{Property: "Health.current", Kind: schema.KindInt32, Numeric: true,
				Keys: []Key{{T: 0, V: Int(3)}, {T: 1, V: Int(1)}}},
			{Property: "SpriteRenderer.sprite", Kind: schema.KindAssetRef,
				Keys: []Key{{T: 0, V: Asset(ids.AssetID{Hi: 1, Lo: 2}), Interp: curve.InterpStep}}},
			{Property: "Hitbox2D.active", Kind: schema.KindBool,
				Keys: []Key{{T: 0.375, V: Bool(true), Interp: curve.InterpStep}}},
			{Target: "Pivot", Property: "transform.rotation", Kind: schema.KindQuat,
				Keys: []Key{{T: 0, V: Lanes([4]float32{0, 0, 0, 1})}, {T: 1, V: Lanes([4]float32{0, 0, h, h})}}},
			{Property: RootProp, Kind: schema.KindVec2, Root: true, Authority: AuthSimulation,
				Keys: []Key{{T: 0, V: Lanes([4]float32{0, 0})}, {T: 1.5, V: Lanes([4]float32{3, 0})}}},
		},
		Events: []Event{{T: 0.375, Name: "Hit", ID: -1, Payload: 1}, {T: 1, Name: "Footstep", ID: -1, Mode: EventPresentation}},
	}
	c.Fix()
	return c
}

func TestTextRoundTrip(t *testing.T) {
	c := sampleClip()
	doc, err := aetext.Parse(aetext.Write(Encode(c)), "clip.aeanim")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, got) {
		t.Fatalf("text round trip:\n%+v\n%+v", c, got)
	}
}

func TestBinaryRoundTrip(t *testing.T) {
	c := sampleClip()
	got, err := Unmarshal(Marshal(c))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, got) {
		t.Fatalf("binary round trip:\n%+v\n%+v", c, got)
	}
}

func TestBinaryRejectsDamage(t *testing.T) {
	data := Marshal(sampleClip())
	for n := 0; n < len(data); n++ {
		if _, err := Unmarshal(data[:n]); err == nil {
			t.Fatalf("a prefix of %d bytes was accepted", n)
		}
	}
	if _, err := Unmarshal(append(data, 0)); err == nil {
		t.Fatal("trailing bytes were accepted")
	}
}

const framesSrc = `anim version=1 length=0.5 loop=loop {
  frames "Body" fps=10 {
    sprites=[asset("a0e10000000000000000000000000001"), asset("a0e10000000000000000000000000002"), asset("a0e10000000000000000000000000003")]
    flipX=[false, false, true]
  }
  track "" prop="SpriteRenderer.tint" kind=color {
    key t=0 v=#ff000080 {}
  }
  event t=0.2 name="Step" mode=presentation {}
}
`

func TestFramesAndHexColor(t *testing.T) {
	doc, err := aetext.Parse([]byte(framesSrc), "run.aeanim")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Decode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tracks) != 3 || c.Loop != LoopRepeat {
		t.Fatalf("tracks %d loop %v", len(c.Tracks), c.Loop)
	}
	sp, fl := &c.Tracks[0], &c.Tracks[1]
	if sp.Target != "Body" || sp.Property != SpriteProp || sp.Kind != schema.KindAssetRef || len(sp.Keys) != 3 {
		t.Fatalf("sprite track %+v", sp)
	}
	if !near(sp.Keys[2].T, 0.2) || sp.Keys[1].V.A.Lo != 2 || sp.Keys[0].Interp != curve.InterpStep {
		t.Fatalf("sprite keys %+v", sp.Keys)
	}
	if fl.Property != FlipProp || fl.Keys[1].V.Truth() || !fl.Keys[2].V.Truth() {
		t.Fatalf("flip track %+v", fl)
	}
	if v := c.Tracks[2].Keys[0].V.L; v[0] != 1 || v[1] != 0 || !near(v[3], 128.0/255) {
		t.Fatalf("hex color %v", v)
	}
	if len(c.Events) != 1 || c.Events[0].Mode != EventPresentation || c.Events[0].ID != -1 {
		t.Fatalf("events %+v", c.Events)
	}
}

func TestDecodeErrors(t *testing.T) {
	for name, src := range map[string]string{
		"no length":  `anim version=1 {}`,
		"bad kind":   `anim version=1 length=1 { track "" prop="A.b" kind=string { key t=0 v="x" {} } }`,
		"bad value":  `anim version=1 length=1 { track "" prop="A.b" kind=vec2 { key t=0 v=1 {} } }`,
		"bad loop":   `anim version=1 length=1 loop=forever {}`,
		"bad block":  `anim version=1 length=1 { curve "" {} }`,
		"free no in": `anim version=1 length=1 { track "" prop="A.b" kind=float32 { key t=0 v=1 tangent=free {} } }`,
		"newer":      `anim version=9 length=1 {}`,
	} {
		doc, err := aetext.Parse([]byte(src), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := Decode(doc); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}
