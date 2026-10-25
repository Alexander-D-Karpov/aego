package animclip

import (
	"strings"
	"testing"

	"aego/anim"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
)

func run(src string) (*pipeline.Result, error) {
	return New().Import(&pipeline.ImportContext{ID: ids.AssetID{Hi: 9, Lo: 9}, Path: "assets/run.aeanim", Source: []byte(src)})
}

func TestImportFrames(t *testing.T) {
	res, err := run(`anim version=1 length=0.4 loop=loop {
  frames "" fps=10 {
    sprites=[asset("a0e10000000000000000000000000002"), asset("a0e10000000000000000000000000001"), asset("a0e10000000000000000000000000002")]
  }
  event t=0.1 name="Footstep" mode=presentation {}
}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != asset.ArtAnim || len(res.Artifacts) != 1 || res.Artifacts[0].Kind != asset.ArtAnim {
		t.Fatalf("result %+v", res)
	}
	if len(res.Deps) != 2 || res.Deps[0].Lo != 1 || res.Deps[1].Lo != 2 {
		t.Fatalf("deps %v", res.Deps)
	}
	c, err := anim.Unmarshal(res.Artifacts[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tracks) != 1 || len(c.Tracks[0].Keys) != 3 || len(c.Events) != 1 {
		t.Fatalf("clip %+v", c)
	}
}

func TestImportErrors(t *testing.T) {
	for code, src := range map[string]string{
		CodeTrack: `anim version=1 length=1 { track "" prop="A.b" kind=string {} }`,
		CodeRange: `anim version=1 length=1 { track "" prop="A.b" kind=float32 { key t=2 v=1 {} } }`,
	} {
		_, err := run(src)
		if err == nil || !strings.Contains(err.Error(), code) {
			t.Fatalf("%s: got %v", code, err)
		}
	}
	if _, err := run(`anim length=`); err == nil {
		t.Fatal("broken text was accepted")
	}
}

func TestUnknownNameIsANM001(t *testing.T) {
	_, err := run("anim version=1 length=1 {\n  frames \"\" {\n    sprites=[asset(\"ae900000000000000000000000000b01\")]\n    start=0.1\n  }\n}\n")
	if err == nil || !strings.Contains(err.Error(), CodeTrack) || !strings.Contains(err.Error(), "start is an attribute of frames") {
		t.Fatalf("got %v", err)
	}
}
