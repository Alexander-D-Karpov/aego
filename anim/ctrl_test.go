package anim

import (
	"reflect"
	"slices"
	"testing"

	"aego/core/curve"
	"aego/format/aetext"
)

const ctrlSrc = `controller version=1 {
  entry="Idle"
  param "Speed" kind=float {}
  param "Grounded" kind=bool default=true {}
  param "Attack" kind=trigger {}
  param "Combo" kind=int default=2 {}
  state "Idle" clip=asset("a0e10000000000000000000000000011") {}
  state "Run" clip=asset("a0e10000000000000000000000000012") speed=1.5 loop=loop {}
  state "Hit" clip=asset("a0e10000000000000000000000000013") loop=once events=both {}
  transition "Idle" to="Run" duration=0.125 {
    cond "Speed" op=gt value=0.1 {}
    cond "Grounded" {}
  }
  transition "Run" to="Idle" duration=0.125 {
    cond "Speed" op=le value=0.1 {}
  }
  transition "any" to="Hit" interrupt=true {
    cond "Attack" {}
  }
  transition "Hit" to="Idle" exitTime=1 duration=0.0625 {
    cond "Combo" op=ge value=2 {}
  }
}
`

func decodeCtrl(t *testing.T, src string) (*Graph, []GraphProblem) {
	t.Helper()
	doc, err := aetext.Parse([]byte(src), "test.aectrl")
	if err != nil {
		t.Fatal(err)
	}
	g, probs, err := DecodeGraph(doc)
	if err != nil {
		t.Fatal(err)
	}
	return g, probs
}

func TestDecodeGraph(t *testing.T) {
	g, probs := decodeCtrl(t, ctrlSrc)
	if len(probs) != 0 || len(g.Check(nil)) != 0 {
		t.Fatalf("problems %+v %+v", probs, g.Check(nil))
	}
	if g.Ease != curve.InOutQuad || g.Entry != 0 || len(g.Params) != 4 || len(g.States) != 3 || len(g.Transitions) != 4 {
		t.Fatalf("graph %+v", g)
	}
	if g.Params[1].Default != 1 || g.Params[3].Default != 2 || g.Params[2].Kind != ParamTrigger {
		t.Fatalf("params %+v", g.Params)
	}
	if run := g.States[1]; run.Speed != 1.5 || !run.HasLoop || run.Loop != LoopRepeat || run.Clip.Lo != 0x12 {
		t.Fatalf("run %+v", run)
	}
	if g.States[2].Events != EventsBoth || g.States[0].HasLoop {
		t.Fatalf("states %+v", g.States)
	}
	if tr := g.Transitions[2]; tr.From != AnyState || tr.To != 2 || !tr.Interrupt || tr.Conds[0].Op != OpSet {
		t.Fatalf("any %+v", tr)
	}
	if tr := g.Transitions[3]; !tr.HasExit || tr.ExitTime != 1 || tr.Conds[0].Op != OpGe || tr.Conds[0].Value != 2 {
		t.Fatalf("exit %+v", tr)
	}
}

func TestGraphTextRoundTrip(t *testing.T) {
	g, _ := decodeCtrl(t, ctrlSrc)
	again, probs := decodeCtrl(t, string(aetext.Write(EncodeGraph(g))))
	if len(probs) != 0 || !reflect.DeepEqual(g, again) {
		t.Fatalf("round trip:\n%+v\n%+v", g, again)
	}
}

func TestGraphBinaryRoundTrip(t *testing.T) {
	g, _ := decodeCtrl(t, ctrlSrc)
	g.Layout()
	data := MarshalGraph(g)
	again, err := UnmarshalGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	g.EntryPos, g.AnyPos, g.HasEntryPos, g.HasAnyPos = [2]float32{}, [2]float32{}, false, false
	if !reflect.DeepEqual(g, again) {
		t.Fatalf("round trip:\n%+v\n%+v", g, again)
	}
	for n := 0; n < len(data); n++ {
		if _, err := UnmarshalGraph(data[:n]); err == nil {
			t.Fatalf("a prefix of %d bytes was accepted", n)
		}
	}
	if _, err := UnmarshalGraph(append(data, 0)); err == nil {
		t.Fatal("trailing bytes were accepted")
	}
}

func TestGraphNamesThatDoNotExist(t *testing.T) {
	_, probs := decodeCtrl(t, `controller version=1 {
  entry="Nope"
  param "Speed" kind=float {}
  state "Idle" {}
  transition "Idle" to="Ghost" {
    cond "Missing" {}
  }
  transition "Phantom" to="Idle" exitTime=1 {}
}
`)
	var names []string
	for _, p := range probs {
		names = append(names, p.Name)
	}
	for _, want := range []string{"Nope", "Ghost", "Missing", "Phantom"} {
		if !slices.Contains(names, want) {
			t.Fatalf("%s is not reported: %+v", want, probs)
		}
	}
}

func TestGraphDecodeErrors(t *testing.T) {
	for name, src := range map[string]string{
		"reserved state":  `controller version=1 { state "any" {} }`,
		"trigger default": `controller version=1 { param "A" kind=trigger default=true {} }`,
		"int default":     `controller version=1 { param "A" kind=int default=1.5 {} }`,
		"no kind":         `controller version=1 { param "A" {} }`,
		"no value":        `controller version=1 { param "A" kind=float {} state "S" {} transition "S" to="S" exitTime=1 { cond "A" op=gt {} } }`,
		"twice":           `controller version=1 { state "S" {} state "S" {} }`,
		"no to":           `controller version=1 { state "S" {} transition "S" {} }`,
		"newer":           `controller version=9 {}`,
	} {
		doc, err := aetext.Parse([]byte(src), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, _, err := DecodeGraph(doc); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}
