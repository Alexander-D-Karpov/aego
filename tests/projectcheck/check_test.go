package projectcheck_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/projects"
	"aego/app/run"
	"aego/asset/pipeline"
	"aego/project"
)

func TestCheckIsReadOnlyAndRejectsStrayScripts(t *testing.T) {
	p, err := project.Create(filepath.Join(t.TempDir(), "game"), "game", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.AssetsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(p.AssetsDir(), "stray.aes")
	if err := os.WriteFile(stray, []byte("package game\n\nfunc Broken() int {\n\treturn nope\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := projects.OpenWorkspace(p, "", false)
	if err != nil {
		t.Fatal(err)
	}
	imps, err := run.Importers(ws)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if n := projects.CheckAssets(p, imps, pipeline.MetaReadOnly, &out); n == 0 || !strings.Contains(out.String(), "stray.aes") {
		t.Fatalf("a script outside scripts/ must be reported, got %d:\n%s", n, out.String())
	}
	if _, err := os.Stat(stray + pipeline.MetaSuffix); !os.IsNotExist(err) {
		t.Fatal("check wrote a .meta file")
	}
}
