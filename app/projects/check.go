package projects

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/app/bootstrap"
	scriptimp "aego/asset/importers/script"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/log"
	"aego/engine"
	"aego/engine/problems"
	"aego/format/aetext"
	"aego/project"
	"aego/script/bind"
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
		built, err := bootstrap.BuildManifest(p, fmt.Sprintf("%s+v%d", engine.Version, manifest.Version), log.New(log.NewTextSink(os.Stderr)))
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

func CheckSections(p *project.Project, w io.Writer) int {
	types, err := assembly.NewTypes()
	if err != nil {
		fmt.Fprintln(w, "aego:", err)
		return 1
	}
	man, err := builtin.Default().Load(types, bind.New(), assembly.Options{})
	if err != nil {
		fmt.Fprintln(w, "aego:", err)
		return 1
	}
	if _, err := man.LoadSections(p.Block, project.CoreBlocks...); err != nil {
		fmt.Fprintf(w, "%s: %v\n", p.FilePath(), err)
		return 1
	}
	return 0
}

func Verify(p *project.Project, w io.Writer) error {
	n := CheckDocuments(p, w) + CheckSections(p, w)
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

type failure struct {
	path string
	msg  string
}

func CheckAssets(p *project.Project, imps *pipeline.ImporterRegistry, mode pipeline.MetaMode, w io.Writer) int {
	types, err := assembly.NewTypes()
	if err != nil {
		fmt.Fprintln(w, "aego:", err)
		return 1
	}
	man, err := builtin.Default().Load(types, bind.New(), assembly.Options{})
	if err != nil {
		fmt.Fprintln(w, "aego:", err)
		return 1
	}
	if err := types.Schema().Freeze(); err != nil {
		fmt.Fprintln(w, "aego:", err)
		return 1
	}
	pl, err := pipeline.Open(p, imps, log.Nop())
	if err != nil {
		fmt.Fprintln(w, "aego:", err)
		return 1
	}
	defer pl.Close()
	pl.SetMetaMode(mode)
	if _, _, err := pl.Scan(); err != nil {
		fmt.Fprintln(w, "aego:", err)
		return 1
	}
	batch, err := pl.ImportAll()
	if err != nil {
		fmt.Fprintln(w, "aego:", err)
		return 1
	}
	var failed []failure
	for id, ierr := range batch.Failed {
		var ce *scriptimp.CompileError
		if errors.As(ierr, &ce) {
			continue
		}
		path, ok := pl.PathOf(id)
		if !ok {
			path = id.String()
		}
		failed = append(failed, failure{path: path, msg: ierr.Error()})
	}
	slices.SortFunc(failed, func(a, b failure) int { return strings.Compare(a.path, b.path) })
	for _, f := range failed {
		fmt.Fprintf(w, "%s: error: %s\n", f.path, f.msg)
	}
	n := len(failed)
	for _, d := range man.ValidateAssets(types.Schema(), pl, bootstrap.NeedsNative(p), nil) {
		sev := "warning"
		if d.Severity == problems.Error {
			sev = "error"
			n++
		}
		fmt.Fprintf(w, "%s: %s %s: %s\n", d.Subject.File, sev, d.Code, d.Message)
	}
	return n
}
