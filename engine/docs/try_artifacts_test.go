//go:build !aego_runtime

package docs

import (
	"strings"
	"testing"
	"testing/fstest"

	"aego/engine/module"
)

func TestTryArtifactsRejectInvalidOrMissingPreparedData(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "malformed", true: "missing"}[missing], func(t *testing.T) {
			files := fstest.MapFS{"page.md": page("---\nid: demo\ntry: try/Example.aescene\n---\n# Demo\n"), "try/Example.aescene": page("scene version=1 {}")}
			if missing {
				files["try/Example.assets/project.aego"] = page("project version=1 {}")
			} else {
				files["try/Example.aetry"] = page("broken")
			}
			r := Assemble(Sources{Docs: []module.DocSource{{Owner: "game", FS: files}}})
			found := false
			for _, p := range r.Check(false) {
				found = found || p.Error && strings.Contains(p.Message, ".aetry")
			}
			if !found {
				t.Fatal("invalid Try artifacts did not block docs check")
			}
			if e, ok := r.Get("demo"); !ok || len(e.TryArtifacts) != 0 {
				t.Fatal("bad payload published")
			}
		})
	}
}
