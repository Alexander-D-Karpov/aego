package state

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
)

func TestRoundTrip(t *testing.T) {
	a, b := ids.NewNodeID(), ids.NewNodeID()
	s := Default()
	s.Active = "scenes/main.aescene"
	s.Tool = 2
	s.Snap = true
	s.WindowW, s.WindowH = 1600, 900
	s.Docs = []Doc{{
		Path:      "scenes/main.aescene",
		Camera:    document.Camera{Position: mathx.Vec2{X: 3, Y: -2}, Zoom: 48, Rotation: 0.5},
		Expanded:  []ids.NodeID{a},
		Selection: []ids.NodeID{a, b},
	}}
	data := Encode(s)
	back, err := Decode(data, "editor.aetext")
	if err != nil {
		t.Fatal(err)
	}
	if string(Encode(back)) != string(data) {
		t.Fatalf("round trip differs:\n%s\n---\n%s", data, Encode(back))
	}
	if back.Docs[0].Camera.Zoom != 48 || len(back.Docs[0].Selection) != 2 || back.Tool != 2 {
		t.Fatalf("state = %+v", back)
	}
}

func TestLoadMissingIsDefault(t *testing.T) {
	s, err := Load(t.TempDir())
	if err != nil || !s.Grid || s.SnapMove != 0.5 {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}
