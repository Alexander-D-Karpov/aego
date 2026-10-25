//go:build !aego_runtime

package docs_test

import (
	"maps"
	"regexp"
	"strconv"
	"testing"
	"testing/fstest"

	"aego/script/diag"
	"aego/script/workspace"
)

func TestRunBlocksCompileWithEveryModule(t *testing.T) {
	r, bm, reg := buildWith(t)
	ws, err := workspace.Open(workspace.Config{FS: fstest.MapFS{}, Module: "game", Scripts: "scripts", Bindings: reg.Table(), Docs: bm})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range r.All() {
		page := map[string][]byte{}
		for i, s := range e.Snippets {
			if !s.Run && !s.Check || s.Lang != "aes" {
				continue
			}
			n++
			files := map[string][]byte{"scripts/" + packageName(s.Code) + "/main.aes": []byte(s.Code)}
			if s.Check {
				// a check block builds with the check blocks above it on its page, a later version of a script replacing the earlier
				name := "scripts/" + packageName(s.Code) + "/main.aes"
				if m := scriptDecl.FindStringSubmatch(s.Code); m != nil {
					name = "scripts/" + packageName(s.Code) + "/" + m[1] + ".aes"
				}
				page[name] = []byte(s.Code)
				files = maps.Clone(page)
			}
			t.Run(e.ID+"/"+strconv.Itoa(i), func(t *testing.T) {
				ws.Sync(files)
				_, ds := ws.Build()
				for _, d := range ds {
					if d.Severity == diag.Error {
						t.Errorf("%s %s", d.Code, d.Message)
					}
				}
			})
		}
	}
	if n == 0 {
		t.Fatal("no run blocks")
	}
}

func packageName(src string) string {
	const kw = "package "
	for i := 0; i+len(kw) <= len(src); i++ {
		if src[i:i+len(kw)] != kw || i > 0 && src[i-1] != '\n' {
			continue
		}
		j := i + len(kw)
		k := j
		for k < len(src) && src[k] != '\n' && src[k] != ' ' && src[k] != '\r' {
			k++
		}
		return src[j:k]
	}
	return "main"
}

var scriptDecl = regexp.MustCompile(`(?m)^script (\w+)`)
