package projects

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine"
	"aego/engine/bootstrap"
	"aego/engine/builtin"
	"aego/format/aetext"
	"aego/project"
	"aego/script/bind/manifest"
	"aego/script/diag"
	"aego/script/workspace"
)

func EngineRoot(flagValue string) (string, error) {
	candidates := []string{flagValue, os.Getenv("AEGO_ROOT")}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "..", ".."))
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if isEngineRoot(abs) {
			return abs, nil
		}
	}
	return "", errors.New("cannot locate the aego source tree; pass --engine <dir> or set AEGO_ROOT")
}

func isEngineRoot(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "module aego" {
			return true
		}
	}
	return false
}

func LoadManifest(p *project.Project, path string) (*manifest.Manifest, error) {
	if path == "" {
		if !bootstrap.NeedsNative(p) {
			return builtin.Manifest()
		}
		built, err := bootstrap.BuildManifest(p, engine.Version, log.New(log.NewTextSink(os.Stderr)))
		if err != nil {
			return nil, err
		}
		path = built
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return manifest.Decode(data)
}

func OpenWorkspace(p *project.Project, manifestPath string, tests bool) (*workspace.Workspace, error) {
	man, err := LoadManifest(p, manifestPath)
	if err != nil {
		return nil, err
	}
	tab, err := man.Table()
	if err != nil {
		return nil, err
	}
	return workspace.Open(workspace.Config{
		Root:     p.Root,
		Module:   p.Module,
		Scripts:  p.ScriptsDir(),
		Bindings: tab,
		Docs:     man,
		Tests:    tests,
	})
}

func CheckDocuments(p *project.Project, w io.Writer) int {
	problems := 0
	err := filepath.WalkDir(p.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != p.Root && skipCopy[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".aescene", ".aeprefab", ".aego", ".meta":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if _, err := aetext.Parse(data, path); err != nil {
			fmt.Fprintln(w, err)
			problems++
		}
		if len(data) > 0 && data[len(data)-1] != '\n' {
			fmt.Fprintf(w, "%s: missing final newline\n", path)
			problems++
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(w, "aego:", err)
		problems++
	}
	return problems
}

func Verify(p *project.Project, w io.Writer) error {
	n := CheckDocuments(p, w)
	ws, err := OpenWorkspace(p, "", false)
	if err != nil {
		return err
	}
	snap := ws.Snapshot()
	diag.Print(w, snap.Sources, snap.Diagnostics)
	n += snap.Errors()
	if n > 0 {
		return errs.New(errs.BuildFailed, fmt.Sprintf("the new project has %d problem(s)", n), log.F("root", p.Root))
	}
	return nil
}
