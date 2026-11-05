package docstool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"aego/app/projects"
	"aego/engine/docs"
)

// GoProblems checks the go blocks of the docs against sdk (D347): a fragment cannot be built and is reported, a whole file is built in a temporary module next to the engine source.
func GoProblems(reg *docs.Registry, engine string, strict bool) []docs.Problem {
	var out []docs.Problem
	root, rootErr := projects.EngineRoot(engine)
	_, goErr := exec.LookPath("go")
	for _, e := range reg.All() {
		for i, s := range docs.GoSnippets(&e) {
			at := e.ID + " go block " + strconv.Itoa(i+1)
			if !strings.HasPrefix(strings.TrimSpace(s.Code), "package ") {
				out = append(out, docs.Problem{ID: e.ID, Error: strict, Message: "DOC022 " + at + " is a fragment; write it as a whole file so it can be built against sdk"})
				continue
			}
			if rootErr != nil || goErr != nil {
				continue
			}
			if msg := buildGo(root, s.Code); msg != "" {
				out = append(out, docs.Problem{ID: e.ID, Error: strict, Message: "DOC022 " + at + " does not build: " + msg})
			}
		}
	}
	return out
}

func buildGo(root, code string) string {
	dir, err := os.MkdirTemp("", "aego-docs-go-")
	if err != nil {
		return err.Error()
	}
	defer os.RemoveAll(dir)
	mod := "module docsnippet\n\ngo 1.27\n\nrequire aego v0.0.0\n\nreplace aego => " + filepath.ToSlash(root) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		return err.Error()
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(code), 0o644); err != nil {
		return err.Error()
	}
	// Build the native example, but vet against the portable SDK to avoid transitive generated cgo GL diagnostics; native vet/lint covers the platform separately.
	for _, args := range [][]string{
		{"build", "-mod=mod", "-o", filepath.Join(dir, "compiled"), "."},
		{"vet", "-tags=aego_nonative", "-mod=mod", "."},
	} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
		if b, err := cmd.CombinedOutput(); err != nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}
