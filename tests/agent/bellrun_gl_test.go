//go:build aego_gltest

package agent_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/agent"
	"aego/app/run"
	"aego/project"
)

func TestCaptureAuthorGameWithCurrentRenderer(t *testing.T) {
	src, out := os.Getenv("AEGO_BELLRUN_SOURCE"), os.Getenv("AEGO_BELLRUN_CAPTURES")
	if src == "" || out == "" {
		t.Skip("set AEGO_BELLRUN_SOURCE and AEGO_BELLRUN_CAPTURES to capture the generated authoring project")
	}
	dir := t.TempDir()
	if err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "captures") {
			return filepath.SkipDir
		}
		dest := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0644)
	}); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, dir: dir, p: p, opt: agent.Options{Run: run.AgentRunner()}}
	args := agent.Args{}
	if err := json.Unmarshal(e.read("playtest-args.json"), &args); err != nil {
		t.Fatal(err)
	}
	args["shot"] = "12,74,175"
	args["camera"] = "Camera"
	o := e.must("scene.run", args)
	report := o.Value.(agent.RunReport)
	if o.Errors() || len(o.Images) != 3 || len(report.Checks) != 10 {
		t.Fatal(o.Problems, report.Shots)
	}
	if err = os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, shot := range report.Shots {
		if shot.Error != "" {
			t.Fatal(shot.Error)
		}
		data := e.read(shot.Path)
		if err := os.WriteFile(filepath.Join(out, filepath.Base(shot.Path)), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "current-renderer-run.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
