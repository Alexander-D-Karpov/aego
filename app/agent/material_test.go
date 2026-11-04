package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/project"
)

func materialSession(t *testing.T) *Session {
	t.Helper()
	p := &project.Project{Root: t.TempDir(), Name: "Materials", Module: "materials", Version: 1, DefaultSpace: project.Space2D, Window: project.WindowSettings{Width: 320, Height: 240}}
	if _, err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func materialCommand(t *testing.T, s *Session, name string, args Args) *Output {
	t.Helper()
	out, err := Run(s, name, args)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

func TestMaterialCommandsUseSharedTypedTransactionsAndOptimisticRevision(t *testing.T) {
	s := materialSession(t)
	const file = "assets/flash.aemat"
	materialCommand(t, s, "material.create", Args{"file": file, "shader": "engine/shaders/flash"})
	out := materialCommand(t, s, "material.get", Args{"file": file})
	revision := out.Value.(map[string]any)["sourceHash"].(string)
	materialCommand(t, s, "material.set", Args{"file": file, "param": "Flash", "value": "0.5", "expected": revision})
	before, err := os.ReadFile(filepath.Join(s.Project.Root, file))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(s, "material.reset", Args{"file": file, "param": "Flash", "expected": revision}); err == nil {
		t.Fatal("stale expected hash was accepted")
	}
	if _, err := Run(s, "material.apply", Args{"file": file, "tx": `tx version=1 { op "material-set" { path="Flash" value=0.8 } op "material-set" { path="Flash" value=true } }`}); err == nil {
		t.Fatal("wrong-type second operation did not reject atomic batch")
	}
	after, _ := os.ReadFile(filepath.Join(s.Project.Root, file))
	if !bytes.Equal(before, after) {
		t.Fatal("failed transaction wrote a partial source")
	}
	materialCommand(t, s, "material.reset", Args{"file": file, "param": "Flash", "dry-run": true})
	after, _ = os.ReadFile(filepath.Join(s.Project.Root, file))
	if !bytes.Equal(before, after) {
		t.Fatal("dry-run wrote material source")
	}
	materialCommand(t, s, "material.reset", Args{"file": file, "param": "Flash"})
	after, _ = os.ReadFile(filepath.Join(s.Project.Root, file))
	doc, err := asset.ParseMaterial(after, file)
	if err != nil {
		t.Fatal(err)
	}
	if _, explicit := doc.Value("Flash"); explicit {
		t.Fatal("reset did not restore inheritance")
	}
}

func TestShaderCommandsCreateAndValidateCurrentSourceInsteadOfLastGood(t *testing.T) {
	s := materialSession(t)
	const file = "assets/custom.aeshader"
	out := materialCommand(t, s, "shader.create", Args{"file": file})
	if len(out.Created) != 1 {
		t.Fatal("new shader did not receive an asset identity")
	}
	validated := materialCommand(t, s, "shader.schema", Args{"shader": file}).Value.(map[string]any)
	if validated["schemaChecked"] != true || validated["gl33Linked"] != false || validated["webgl2Linked"] != false {
		t.Fatal("headless validation claimed a device link")
	}
	abs := filepath.Join(s.Project.Root, file)
	if err := os.WriteFile(abs, []byte("#pragma aego abi 99\nvoid fragment(){}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(s, "shader.validate", Args{"shader": file}); err == nil {
		t.Fatal("validation reused a stale last-good artifact")
	}
	if _, err := Run(s, "material.create", Args{"file": "assets/broken.aemat", "shader": file}); err == nil {
		t.Fatal("created material from invalid current shader")
	}
	if _, err := os.Stat(filepath.Join(s.Project.Root, "assets/broken.aemat")); !os.IsNotExist(err) {
		t.Fatal("failed create left a partial source")
	}
}
