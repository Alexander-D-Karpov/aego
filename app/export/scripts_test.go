package export

import (
	"strings"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/asset/loader"
	"aego/engine"
	"aego/engine/problems"
	scriptsystems "aego/modules/scripting/systems"
	"aego/project"
	"aego/scene/sceneio"
	"aego/script/bind"
	scriptruntime "aego/script/runtime"
)

func TestExportPublishesCompiledProjectScriptsAndKeepsUnknownDiagnostics(t *testing.T) {
	p, err := project.Create(t.TempDir(), "game", false)
	if err != nil {
		t.Fatal(err)
	}
	writeMaterialExportSource(t, p.Root, "scripts/mover.aes", `package game
@former("game.OldMover")
script Mover {
    @export var Speed float = 2
    func Ready() {
        p := node.Position()
        p.X = Speed
        node.SetPosition(p)
    }
}
`)
	writeMaterialExportSource(t, p.Root, "scenes/main.aescene", `scene version=1 {
  node "0123456789abcdef0123456789abcdef" name="Known" {
    component "engine.Script" version=1 {
      script "game.Mover" { Speed=7 }
      script "missing.NotCompiled" { Keep=true }
    }
  }
  node "0123456789abcdef0123456789abcdee" name="Former" {
    component "engine.Script" version=1 {
      script "game.OldMover" { Speed=9 }
    }
  }
}
`)
	s, closeSnapshot, err := Fresh(p)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSnapshot()
	s.idx.Each(func(e *asset.Entry) {
		if e.Source == "scenes/main.aescene" {
			p.MainScene = e.ID
		}
	})
	ds := Check(p, s, "release", false)
	if Blocking(ds) != 0 {
		t.Fatalf("compiled project failed export validation: %+v", ds)
	}
	unknown := 0
	for _, d := range ds {
		if d.Code != CodeUnknown {
			continue
		}
		unknown++
		if d.Severity != problems.Warning || !strings.Contains(d.Message, "missing.NotCompiled") || d.Subject.File != "scenes/main.aescene" {
			t.Fatalf("compiled/Former script reported as unknown: %+v", d)
		}
	}
	if unknown != 1 {
		t.Fatalf("unknown script warning was lost: %+v", ds)
	}

	// Execute those same frozen bytes through the player's Link -> scene Load
	// -> Instantiate order. Schema-only acceptance must not hide an inert script.
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	bindings := bind.New()
	if _, err := builtin.Runtime().Load(types, bindings, assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	if err := scriptsystems.Freeze(bindings, types.Schema()); err != nil {
		t.Fatal(err)
	}
	l := loader.New(nil, nil, nil)
	if err := l.Mount(s.idx, s.files); err != nil {
		t.Fatal(err)
	}
	blobs, err := l.Scripts()
	if err != nil {
		t.Fatal(err)
	}
	mods, err := scriptruntime.DecodeAll(blobs)
	if err != nil {
		t.Fatal(err)
	}
	rt := scriptruntime.New(scriptruntime.Options{Bindings: bindings, Schema: types.Schema()})
	defer rt.Close()
	if err := rt.Link(mods); err != nil {
		t.Fatal(err)
	}
	res := &resolver{s: s}
	source, ok := res.blob(p.MainScene, asset.ArtScene)
	if !ok {
		t.Fatal("main scene artifact missing")
	}
	sc, _, err := sceneio.Load(source, "scenes/main.aescene", types, res, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Instantiate(sc); err != nil {
		t.Fatal(err)
	}
	rt.Update(&engine.Context{World: engine.NewWorld("export-test"), Dt: .1, Frame: 1})
	if len(rt.Faults()) != 0 || rt.Stats().Instances != 2 {
		t.Fatalf("compiled scripts did not execute: stats=%+v faults=%+v", rt.Stats(), rt.Faults())
	}
	nodes := sc.PreOrder()
	if len(nodes) != 3 || sc.Local(nodes[1]).Position.X != 7 || sc.Local(nodes[2]).Position.X != 9 {
		t.Fatal("Ready did not apply compiled fields through known and Former script attachments")
	}
}

func TestExportRejectsMalformedCompiledScriptSchema(t *testing.T) {
	s, files, _ := preparedMaterialSnapshot(t, "", "")
	files["invalid.aesb"] = []byte("invalid compiled script")
	e := asset.Entry{ID: asset.BuiltinShaderUnlit, Kind: asset.ArtScript, Source: "scripts:fixture", Artifacts: []asset.Artifact{{Kind: asset.ArtScript, Name: "game", Path: "cache://invalid.aesb"}}}
	s.idx.Put(e)
	if ds := validate(s, false); !hasExportError(ds, CodeScripts, "cannot load the compiled script schemas") {
		t.Fatalf("invalid compiled schema was accepted: %+v", ds)
	}
}
