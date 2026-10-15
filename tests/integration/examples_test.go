package integration

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/engine/module"
	"aego/project"
	"aego/scene/sceneio"
	"aego/script/bind"
	"aego/sdk"
)

var updateExamples = flag.Bool("update-examples", false, "rewrite example project and scene files in canonical form")

func TestExamplesCanonical(t *testing.T) {
	for _, tc := range []struct {
		dir  string
		mods []sdk.Module
	}{
		{"platformer", []sdk.Module{combatModule{}}},
		{"physics-sandbox", nil},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			canonicalExample(t, filepath.Join("..", "..", "examples", tc.dir), tc.mods)
		})
	}
}

func canonicalExample(t *testing.T, root string, mods []sdk.Module) {
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	sameFile(t, p.FilePath(), p.Encode())
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	adapted := make([]module.Module, len(mods))
	for i, m := range mods {
		adapted[i] = sdk.Adapt(m)
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{Seed: 1}, adapted...); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, "scenes", "*.aescene"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenes in %s: %v", root, err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s, _, err := sceneio.Load(data, filepath.Base(f), types, nil, sceneio.LoadOptions{KeepIDs: true})
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		sameFile(t, f, sceneio.Save(s))
	}
}

func sameFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == string(want) {
		return
	}
	if *updateExamples {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatalf("%s is not canonical, run go test ./tests/integration -run TestExamplesCanonical -update-examples", path)
}
