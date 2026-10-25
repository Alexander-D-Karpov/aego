package animctrl

import (
	"strings"
	"testing"

	"aego/anim"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
)

func run(src string) (*pipeline.Result, error) {
	return New().Import(&pipeline.ImportContext{ID: ids.AssetID{Hi: 9, Lo: 9}, Path: "assets/player.aectrl", Source: []byte(src)})
}

func TestImport(t *testing.T) {
	res, err := run(`controller version=1 {
  entry="Idle"
  param "Speed" kind=float {}
  state "Idle" clip=asset("a0e10000000000000000000000000012") {}
  state "Run" clip=asset("a0e10000000000000000000000000011") {}
  state "Pose" clip=asset("a0e10000000000000000000000000012") {}
  transition "Idle" to="Run" duration=0.125 {
    cond "Speed" op=gt value=0.1 {}
  }
}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != asset.ArtCtrl || len(res.Deps) != 2 || res.Deps[0].Lo != 0x11 || res.Deps[1].Lo != 0x12 {
		t.Fatalf("result %+v", res)
	}
	g, err := anim.UnmarshalGraph(res.Artifacts[0].Data)
	if err != nil || len(g.States) != 3 || len(g.Transitions) != 1 {
		t.Fatalf("graph %+v %v", g, err)
	}
}

func TestImportCodes(t *testing.T) {
	for code, src := range map[string]string{
		CodeParam: `controller version=1 { param "Grounded" kind=bool {} state "A" {} state "B" {} transition "A" to="B" { cond "Grounded" op=gt value=0 {} } }`,
		CodeState: `controller version=1 { state "A" {} transition "A" to="Ghost" {} }`,
		CodeSelf:  `controller version=1 { param "P" kind=trigger {} state "A" {} transition "A" to="A" { cond "P" {} } }`,
		CodeLoop:  `controller version=1 { state "A" {} state "B" {} transition "A" to="B" {} transition "B" to="A" {} }`,
		CodeText:  `controller version=1 { state "A" speedd=2 {} }`,
	} {
		_, err := run(src)
		if err == nil || !strings.Contains(err.Error(), code) {
			t.Fatalf("%s: got %v", code, err)
		}
	}
}
