package anim

import (
	"testing"

	"aego/format/aetext"
)

const layoutSrc = `controller version=1 {
  entry="Idle"
  param "Speed" kind=float {}
  state "Idle" {}
  state "Run" {}
  state "Jump" pos=[500, -40] {}
  state "Lost" {}
  transition "Idle" to="Run" duration=0.1 {
    cond "Speed" op=gt value=0.1 {}
  }
  transition "Run" to="Idle" duration=0.1 {
    cond "Speed" op=le value=0.1 {}
  }
  transition "any" to="Jump" {
    cond "Speed" op=lt value=0 {}
  }
}
`

func layoutGraph(t *testing.T) *Graph {
	t.Helper()
	doc, err := aetext.Parse([]byte(layoutSrc), "l.aectrl")
	if err != nil {
		t.Fatal(err)
	}
	g, probs, err := DecodeGraph(doc)
	if err != nil || len(probs) > 0 {
		t.Fatalf("%v %+v", err, probs)
	}
	return g
}

func TestLayoutPlacesEveryStateApart(t *testing.T) {
	g := layoutGraph(t)
	g.Layout()
	seen := map[[2]float32]string{}
	for _, s := range g.States {
		if !s.HasPos {
			t.Fatalf("%s has no position", s.Name)
		}
		if o, dup := seen[s.Pos]; dup {
			t.Fatalf("%s and %s share %v", s.Name, o, s.Pos)
		}
		seen[s.Pos] = s.Name
	}
	idle, run, jump := g.States[0].Pos, g.States[1].Pos, g.States[2].Pos
	if idle[0] != 0 || run[0] != LayoutColumn {
		t.Fatalf("entry %v, next %v: the entry is the first column", idle, run)
	}
	if jump != [2]float32{500, -40} {
		t.Fatalf("a written position moved to %v", jump)
	}
}

func TestPosRoundTrips(t *testing.T) {
	g := layoutGraph(t)
	g.Layout()
	text := aetext.Write(EncodeGraph(g))
	doc, err := aetext.Parse(text, "l.aectrl")
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := DecodeGraph(doc)
	if err != nil {
		t.Fatal(err)
	}
	bin, err := UnmarshalGraph(MarshalGraph(back))
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.States {
		if back.States[i].Pos != g.States[i].Pos || bin.States[i].Pos != g.States[i].Pos {
			t.Fatalf("%s: text %v binary %v, want %v", g.States[i].Name, back.States[i].Pos, bin.States[i].Pos, g.States[i].Pos)
		}
	}
	if _, err := UnmarshalGraph(MarshalGraph(back)[:len(MarshalGraph(back))-4]); err == nil {
		t.Fatal("a truncated state was accepted")
	}
}
