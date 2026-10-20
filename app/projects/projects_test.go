package projects_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/edit"
	"aego/app/projects"
	"aego/app/run"
	"aego/core/log"
	"aego/project"
	"aego/render/view"
	"aego/scene/sceneio"
)

func TestTemplates(t *testing.T) {
	for _, tmpl := range projects.Templates {
		t.Run(tmpl.ID, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "game")
			p, err := projects.Create(projects.Options{Dir: dir, Template: tmpl.ID})
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := projects.Verify(p, &out); err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			if p.Module != "game" || p.MainScene.IsZero() {
				t.Fatalf("module=%q mainScene=%v", p.Module, p.MainScene)
			}
			want3D := tmpl.ID == projects.Template3D || tmpl.ID == projects.Template25D
			if (p.DefaultSpace == project.Space3D) != want3D {
				t.Fatalf("defaultSpace = %q", p.DefaultSpace)
			}
			for _, d := range project.DefaultDirs {
				if _, err := os.Stat(filepath.Join(p.Root, d)); err != nil {
					t.Fatalf("missing %s", d)
				}
			}
			a, err := run.Open(p, run.Config{Headless: true, Logger: log.Nop()})
			if err != nil {
				t.Fatal(err)
			}
			driver := a.Driver()
			for i := 0; i < 60; i++ {
				if !a.Engine.Step(driver) {
					t.Fatalf("engine stopped at frame %d: %v", i, a.Engine.Err())
				}
			}
			if st := a.Runner.Stats(); st.MissingAssets != 0 {
				t.Fatalf("missing assets = %d", st.MissingAssets)
			}
			if tmpl.ID == projects.Template25D {
				if vs := a.Runner.Views(); len(vs) != 1 || vs[0].Projection != view.Ortho3D {
					t.Fatalf("2.5d views = %+v", vs)
				}
			}
			data, ok := a.Loader.Scene(p.MainScene)
			if !ok {
				t.Fatal("main scene is not imported")
			}
			_, rep, err := sceneio.Load(data, "main.aescene", a.Types, a.Loader, sceneio.LoadOptions{KeepIDs: true})
			if err != nil || len(rep.Warnings)+len(rep.Unresolved)+len(rep.Unknown) > 0 {
				t.Fatalf("main scene: err=%v report=%+v", err, rep)
			}
			a.Close()
			s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
			if err != nil {
				t.Fatal(err)
			}
			s.Step()
			if s.App.Context().Doc() == nil {
				t.Fatal("main scene did not open in the editor")
			}
			s.Close()
		})
	}
}

func TestModuleValidation(t *testing.T) {
	for _, m := range []string{"Bad.Mod", "aego/x", "has space"} {
		dir := filepath.Join(t.TempDir(), "g")
		if _, err := projects.Create(projects.Options{Dir: dir, Module: m}); err == nil {
			t.Fatalf("module %q accepted", m)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("failed create left %s behind", dir)
		}
	}
	p, err := projects.Create(projects.Options{Dir: filepath.Join(t.TempDir(), "My Game")})
	if err != nil {
		t.Fatal(err)
	}
	if p.Module != "my-game" {
		t.Fatalf("default module = %q", p.Module)
	}
}

func TestRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o644)
	if _, err := projects.Create(projects.Options{Dir: dir}); err == nil {
		t.Fatal("created a project in a non-empty directory")
	}
}

func TestExamplePlatformer(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the native module")
	}
	root, err := projects.EngineRoot("")
	if err != nil {
		t.Skip(err)
	}
	dir := filepath.Join(t.TempDir(), "arcade")
	p, err := projects.Create(projects.Options{Dir: dir, Example: "platformer", Engine: root})
	if err != nil {
		t.Fatal(err)
	}
	sceneData, _ := os.ReadFile(filepath.Join(dir, "scenes", "main.aescene"))
	if !strings.Contains(string(sceneData), `"arcade.player.PlayerController"`) || strings.Contains(string(sceneData), "mygame") {
		t.Fatal("module was not renamed in the scene")
	}
	gomod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(gomod), "module arcade") || !strings.Contains(string(gomod), "replace aego => "+filepath.ToSlash(root)) {
		t.Fatalf("go.mod:\n%s", gomod)
	}
	var out bytes.Buffer
	if err := projects.Verify(p, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
}

func TestCheckSectionsReportsBadBlock(t *testing.T) {
	dir := t.TempDir()
	src := "project version=1 {\n  name=\"T\"\n  module=\"t\"\n  physics {\n    substeps=-3\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(dir, project.FileName), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if n := projects.CheckSections(p, &out); n != 1 || !strings.Contains(out.String(), "substeps") {
		t.Fatalf("n = %d, out = %q", n, out.String())
	}
}

func TestTemplatePreviewIsNotCopied(t *testing.T) {
	for _, tm := range projects.Templates {
		for _, f := range projects.TemplateFiles(tm.ID) {
			if f == projects.PreviewFile {
				t.Fatalf("%s lists its preview as a project file", tm.ID)
			}
		}
		p, err := projects.Create(projects.Options{Dir: filepath.Join(t.TempDir(), "p"), Template: tm.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(p.Root, projects.PreviewFile)); err == nil {
			t.Fatalf("template %s copied %s into the project", tm.ID, projects.PreviewFile)
		}
	}
}
