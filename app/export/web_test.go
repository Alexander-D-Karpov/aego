package export

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/project"
	"aego/runtime/bundle"
)

var webTarget = Target{OS: "js", Arch: "wasm"}

func TestParseWeb(t *testing.T) {
	for _, s := range []string{"web", "WEB", "js/wasm", "wasm"} {
		tg, err := ParseTarget(s)
		if err != nil || !tg.Web() || tg.String() != "web" {
			t.Fatalf("%s: %+v %v", s, tg, err)
		}
	}
}

func TestWebLayout(t *testing.T) {
	p := &project.Project{Name: "My Game", Root: t.TempDir()}
	l := layoutFor(webTarget, "/out", p)
	if l.binary() != filepath.Join("/out", WebWasm) || l.data != filepath.Join("/out", bundle.DataDir) {
		t.Fatalf("layout %+v", l)
	}
	if !strings.HasSuffix(DefaultOut(p, webTarget, bundle.Release), "web-"+bundle.Release) {
		t.Fatal(DefaultOut(p, webTarget, bundle.Release))
	}
}

func TestPackWeb(t *testing.T) {
	root, err := goRoot()
	if err != nil {
		t.Skip(err)
	}
	out := t.TempDir()
	wasm := bytes.Repeat([]byte("wasm"), 4096)
	j := &job{
		p: &project.Project{Name: `<Game & "Co">`, Module: "game"}, t: webTarget, out: out, goroot: root,
		rep:   &Report{Hash: strings.Repeat("ab", 32), BinarySize: int64(len(wasm))},
		files: []webFile{{Path: "artifacts/a.tex", Size: 3}}, manifestSize: 8,
	}
	j.l = layoutFor(webTarget, out, j.p)
	if err := os.MkdirAll(j.l.data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.l.binary(), wasm, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := j.packWeb(); err != nil {
		t.Fatal(err)
	}
	page, _ := os.ReadFile(filepath.Join(out, webIndex))
	if !strings.Contains(string(page), "&lt;Game &amp;") || strings.Contains(string(page), "<Game") {
		t.Fatal("the title is not escaped")
	}
	if !strings.Contains(string(page), `data-build="abababababab"`) {
		t.Fatal("the build key is missing")
	}
	raw, err := os.ReadFile(filepath.Join(j.l.data, WebFiles))
	if err != nil {
		t.Fatal(err)
	}
	var list webList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if list.Wasm != int64(len(wasm)) || list.Manifest.Size != 8 || len(list.Files) != 1 || list.Files[0].Path != "artifacts/a.tex" {
		t.Fatalf("files.json %+v", list)
	}
	for _, f := range []string{webExec, webLoader, WebWasm + ".gz"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("%s is missing", f)
		}
	}
	if j.rep.BinaryGzip <= 0 || j.rep.BinaryGzip >= j.rep.BinarySize {
		t.Fatalf("wasm gzip size %d of %d", j.rep.BinaryGzip, j.rep.BinarySize)
	}
}
